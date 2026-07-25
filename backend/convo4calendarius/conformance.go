package convo4calendarius

import (
	"context"
	"testing"
)

// RunServiceConformance asserts the behaviour every Service implementation —
// real or fake — must exhibit for ResolveContacts. A fake that is kinder than
// the real thing hides the bugs it exists to catch: a fake once used in
// sneat-bots returned a ClarificationError itself, which let the real
// convoservice4calendarius implementation return a bare, unwrapped error on
// the same path while every test using the fake kept passing. Users saw
// "calendarius contact resolver returned 0 contacts for 1 participants"
// before that was fixed reactively (sneat-bots#53, #55). This suite exists so
// that class of bug fails a test the moment either side drifts from the
// documented contract, instead of shipping.
//
// newService seeds an implementation with the given contacts and returns it
// ready to use. The suite calls every method with context.Background(); an
// implementation that needs more context (for example a user-scoped one, as
// the real Calendarius service requires) must have its factory return a
// Service wrapper that supplies it internally — what a context must carry is
// not part of this contract, so the suite deliberately stays agnostic about
// it and only exercises the documented ResolveContacts behaviour.
func RunServiceConformance(t *testing.T, newService func(t *testing.T, contacts []Contact) Service) {
	t.Helper()
	for _, check := range serviceConformanceChecks {
		t.Run(check.name, func(t *testing.T) {
			t.Helper()
			check.run(t, func(contacts []Contact) Service { return newService(t, contacts) })
		})
	}
}

// conformanceT is the subset of *testing.T the checks below need.
//
// They take this interface rather than *testing.T so that the suite's OWN
// negative test — the one proving it actually REJECTS a non-conforming
// implementation — can run in-process against a recording stand-in. A suite
// that always passes is worse than no suite, and the only way to know it does
// not is to run it against something deliberately broken. Driving that through
// a subprocess instead would leave every assertion branch here unexecuted as
// far as coverage is concerned, which is exactly the blind spot this package
// exists to close.
//
// There is no Fatalf: a check reports with Errorf and returns, so control flow
// never depends on runtime.Goexit and a stand-in needs no special machinery.
type conformanceT interface {
	Helper()
	Errorf(format string, args ...any)
}

// serviceConformanceCheck is one named contract obligation.
type serviceConformanceCheck struct {
	name string
	run  func(t conformanceT, newService func(contacts []Contact) Service)
}

var serviceConformanceChecks = []serviceConformanceCheck{
	{"UnambiguousNameResolves", checkUnambiguousNameResolves},
	{"AmbiguousNameIsOmittedWithNilError", checkAmbiguousNameIsOmitted},
	{"UnknownNameIsOmittedWithNilError", checkUnknownNameIsOmitted},
	{"CountMismatchIsHowACallerDetectsAProblem", checkCountMismatchIsDetectable},
	{"MatchingIsCaseInsensitiveSubstring", checkMatchingIsCaseInsensitiveSubstring},
}

func checkUnambiguousNameResolves(t conformanceT, newService func(contacts []Contact) Service) {
	t.Helper()
	service := newService([]Contact{
		{ID: "c1", DisplayName: "Sarah Connor"},
		{ID: "c2", DisplayName: "Bob Marley"},
	})

	resolved, err := service.ResolveContacts(context.Background(), "space1", []string{"Bob"})
	if err != nil {
		t.Errorf(`ResolveContacts("Bob") error = %v, want nil`, err)
		return
	}
	if len(resolved) != 1 || resolved[0].ID != "c2" {
		t.Errorf(`ResolveContacts("Bob") = %+v, want exactly [Bob Marley]`, resolved)
	}
}

// An ambiguous name is the CALLER's problem to solve, never the service's to
// guess at. Picking one of several matches would silently invite the wrong
// person to a meeting — a mistake nobody would notice until the event. The
// documented contract is: omit it, and return a nil error, not a clarification
// raised by the service itself.
func checkAmbiguousNameIsOmitted(t conformanceT, newService func(contacts []Contact) Service) {
	t.Helper()
	service := newService([]Contact{
		{ID: "c1", DisplayName: "Sarah Connor"},
		{ID: "c2", DisplayName: "Sarah Miles"},
	})

	resolved, err := service.ResolveContacts(context.Background(), "space1", []string{"Sarah"})
	if err != nil {
		t.Errorf(`ResolveContacts("Sarah") error = %v, want nil — an ambiguous name is the caller's decision, not the service's`, err)
		return
	}
	if len(resolved) != 0 {
		t.Errorf(`ResolveContacts("Sarah") = %+v, want none — the name matches two contacts`, resolved)
	}
}

func checkUnknownNameIsOmitted(t conformanceT, newService func(contacts []Contact) Service) {
	t.Helper()
	service := newService([]Contact{{ID: "c1", DisplayName: "Sarah Connor"}})

	resolved, err := service.ResolveContacts(context.Background(), "space1", []string{"Nobody"})
	if err != nil {
		t.Errorf(`ResolveContacts("Nobody") error = %v, want nil`, err)
		return
	}
	if len(resolved) != 0 {
		t.Errorf(`ResolveContacts("Nobody") = %+v, want none`, resolved)
	}
}

// The service never reports WHICH names failed to resolve — that is the point
// of omission over an error. A caller detects the problem only by noticing the
// count shrank, so resolved names must keep the request's order for that
// comparison to mean anything.
func checkCountMismatchIsDetectable(t conformanceT, newService func(contacts []Contact) Service) {
	t.Helper()
	service := newService([]Contact{
		{ID: "c1", DisplayName: "Sarah Connor"},
		{ID: "c2", DisplayName: "Sarah Miles"},
		{ID: "c3", DisplayName: "Bob Marley"},
		{ID: "c4", DisplayName: "Alice Cooper"},
	})

	// "Sarah" is ambiguous, "Nobody" is unknown; only Bob and Alice uniquely
	// resolve.
	names := []string{"Bob", "Sarah", "Alice", "Nobody"}
	resolved, err := service.ResolveContacts(context.Background(), "space1", names)
	if err != nil {
		t.Errorf("ResolveContacts(%v) error = %v, want nil", names, err)
		return
	}
	if len(resolved) == len(names) {
		t.Errorf("ResolveContacts(%v) resolved all %d names, want fewer — two of them do not uniquely resolve", names, len(names))
		return
	}
	if len(resolved) != 2 || resolved[0].ID != "c3" || resolved[1].ID != "c4" {
		t.Errorf("ResolveContacts(%v) = %+v, want exactly [Bob Marley, Alice Cooper] in request order", names, resolved)
	}
}

// convoservice4calendarius documents matching as a case-insensitive substring
// of the contact's display name — not an exact or prefix match. Assert exactly
// that rule, no stricter.
func checkMatchingIsCaseInsensitiveSubstring(t conformanceT, newService func(contacts []Contact) Service) {
	t.Helper()
	service := newService([]Contact{{ID: "c1", DisplayName: "Bob Marley"}})

	for _, name := range []string{"bob", "MARLEY", "b Mar"} {
		resolved, err := service.ResolveContacts(context.Background(), "space1", []string{name})
		if err != nil || len(resolved) != 1 || resolved[0].ID != "c1" {
			t.Errorf(`ResolveContacts(%q) = %+v, %v, want [Bob Marley], nil — matching is a case-insensitive substring of the display name`,
				name, resolved, err)
		}
	}
}

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

	t.Run("UnambiguousNameResolves", func(t *testing.T) {
		contacts := []Contact{
			{ID: "c1", DisplayName: "Sarah Connor"},
			{ID: "c2", DisplayName: "Bob Marley"},
		}
		service := newService(t, contacts)

		resolved, err := service.ResolveContacts(context.Background(), "space1", []string{"Bob"})
		if err != nil {
			t.Fatalf(`ResolveContacts("Bob") error = %v, want nil`, err)
		}
		if len(resolved) != 1 || resolved[0].ID != "c2" {
			t.Fatalf(`ResolveContacts("Bob") = %+v, want exactly [Bob Marley]`, resolved)
		}
	})

	// An ambiguous name is the CALLER's problem to solve, never the service's
	// to guess at. Picking one of several matches would silently invite the
	// wrong person to a meeting — a mistake nobody would notice until the
	// event. The documented contract is: omit it, and return a nil error, not
	// a clarification raised by the service itself.
	t.Run("AmbiguousNameIsOmittedWithNilError", func(t *testing.T) {
		contacts := []Contact{
			{ID: "c1", DisplayName: "Sarah Connor"},
			{ID: "c2", DisplayName: "Sarah Miles"},
		}
		service := newService(t, contacts)

		resolved, err := service.ResolveContacts(context.Background(), "space1", []string{"Sarah"})
		if err != nil {
			t.Fatalf(`ResolveContacts("Sarah") error = %v, want nil — an ambiguous name is the caller's decision, not the service's`, err)
		}
		if len(resolved) != 0 {
			t.Fatalf(`ResolveContacts("Sarah") = %+v, want none — the name matches two contacts`, resolved)
		}
	})

	t.Run("UnknownNameIsOmittedWithNilError", func(t *testing.T) {
		contacts := []Contact{{ID: "c1", DisplayName: "Sarah Connor"}}
		service := newService(t, contacts)

		resolved, err := service.ResolveContacts(context.Background(), "space1", []string{"Nobody"})
		if err != nil {
			t.Fatalf(`ResolveContacts("Nobody") error = %v, want nil`, err)
		}
		if len(resolved) != 0 {
			t.Fatalf(`ResolveContacts("Nobody") = %+v, want none`, resolved)
		}
	})

	// The service never reports WHICH names failed to resolve — that is the
	// point of omission over an error. A caller detects the problem only by
	// noticing the count shrank, so resolved names must keep the request's
	// order for that comparison to mean anything.
	t.Run("CountMismatchIsHowACallerDetectsAProblem", func(t *testing.T) {
		contacts := []Contact{
			{ID: "c1", DisplayName: "Sarah Connor"},
			{ID: "c2", DisplayName: "Sarah Miles"},
			{ID: "c3", DisplayName: "Bob Marley"},
			{ID: "c4", DisplayName: "Alice Cooper"},
		}
		service := newService(t, contacts)

		// "Sarah" is ambiguous, "Nobody" is unknown; only Bob and Alice
		// uniquely resolve.
		names := []string{"Bob", "Sarah", "Alice", "Nobody"}
		resolved, err := service.ResolveContacts(context.Background(), "space1", names)
		if err != nil {
			t.Fatalf("ResolveContacts(%v) error = %v, want nil", names, err)
		}
		if len(resolved) == len(names) {
			t.Fatalf("ResolveContacts(%v) resolved all %d names, want fewer — two of them do not uniquely resolve", names, len(names))
		}
		if len(resolved) != 2 || resolved[0].ID != "c3" || resolved[1].ID != "c4" {
			t.Fatalf("ResolveContacts(%v) = %+v, want exactly [Bob Marley, Alice Cooper] in request order", names, resolved)
		}
	})

	// convoservice4calendarius documents matching as a case-insensitive
	// substring of the contact's display name — not an exact or
	// prefix match. Assert exactly that rule, no stricter.
	t.Run("MatchingIsCaseInsensitiveSubstring", func(t *testing.T) {
		contacts := []Contact{{ID: "c1", DisplayName: "Bob Marley"}}
		service := newService(t, contacts)

		lower, err := service.ResolveContacts(context.Background(), "space1", []string{"bob"})
		if err != nil || len(lower) != 1 || lower[0].ID != "c1" {
			t.Fatalf(`ResolveContacts("bob") = %+v, %v, want [Bob Marley], nil — matching must be case-insensitive`, lower, err)
		}

		upper, err := service.ResolveContacts(context.Background(), "space1", []string{"MARLEY"})
		if err != nil || len(upper) != 1 || upper[0].ID != "c1" {
			t.Fatalf(`ResolveContacts("MARLEY") = %+v, %v, want [Bob Marley], nil — matching must be case-insensitive`, upper, err)
		}

		substring, err := service.ResolveContacts(context.Background(), "space1", []string{"b Mar"})
		if err != nil || len(substring) != 1 || substring[0].ID != "c1" {
			t.Fatalf(`ResolveContacts("b Mar") = %+v, %v, want [Bob Marley], nil — the name only has to be a substring of the display name`, substring, err)
		}
	})
}

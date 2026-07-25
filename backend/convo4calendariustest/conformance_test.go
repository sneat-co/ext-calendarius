package convo4calendariustest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/sneat-co/ext-calendarius/backend/convo4calendarius"
)

// referenceService is a minimal, deliberately correct implementation of
// ResolveContacts used only to exercise RunServiceConformance itself. It
// mirrors convoservice4calendarius's documented matching rule (case-
// insensitive substring, ambiguous/unknown names omitted with a nil error)
// so the suite's own assertions run against a known-good subject here, not
// just against downstream repos where this module can't run its tests.
type referenceService struct {
	contacts []convo4calendarius.Contact
}

func (s referenceService) ResolveContacts(_ context.Context, _ string, names []string) ([]convo4calendarius.Contact, error) {
	resolved := make([]convo4calendarius.Contact, 0, len(names))
	for _, name := range names {
		var matches []convo4calendarius.Contact
		for _, contact := range s.contacts {
			if contact.DisplayName == "" || !strings.Contains(strings.ToLower(contact.DisplayName), strings.ToLower(name)) {
				continue
			}
			matches = append(matches, contact)
		}
		if len(matches) == 1 {
			resolved = append(resolved, matches[0])
		}
	}
	return resolved, nil
}

func (s referenceService) CreateEvent(context.Context, convo4calendarius.CreateEventRequest) (convo4calendarius.Event, error) {
	return convo4calendarius.Event{}, nil
}

func (s referenceService) ListEvents(context.Context, string) ([]convo4calendarius.Event, error) {
	return nil, nil
}

func (s referenceService) DeleteEvent(context.Context, string, string, string) error {
	return nil
}

func TestRunServiceConformanceAgainstReferenceImplementation(t *testing.T) {
	RunServiceConformance(t, func(t *testing.T, contacts []convo4calendarius.Contact) convo4calendarius.Service {
		t.Helper()
		return referenceService{contacts: contacts}
	})
}

// brokenService is deliberately WRONG: on an ambiguous name it guesses the
// first match instead of omitting it. Its only purpose is to prove
// RunServiceConformance actually rejects an implementation that violates the
// documented contract — a suite that always passes, no matter what it is
// run against, catches nothing. That is exactly the failure mode this whole
// suite exists to prevent: a fake (or, here, an implementation) that looks
// fine because nothing ever exercises its wrong path.
type brokenService struct {
	contacts []convo4calendarius.Contact
}

func (s brokenService) ResolveContacts(_ context.Context, _ string, names []string) ([]convo4calendarius.Contact, error) {
	resolved := make([]convo4calendarius.Contact, 0, len(names))
	for _, name := range names {
		for _, contact := range s.contacts {
			if contact.DisplayName == "" || !strings.Contains(strings.ToLower(contact.DisplayName), strings.ToLower(name)) {
				continue
			}
			resolved = append(resolved, contact) // wrong: never checks for ambiguity
			break
		}
	}
	return resolved, nil
}

func (s brokenService) CreateEvent(context.Context, convo4calendarius.CreateEventRequest) (convo4calendarius.Event, error) {
	return convo4calendarius.Event{}, nil
}

func (s brokenService) ListEvents(context.Context, string) ([]convo4calendarius.Event, error) {
	return nil, nil
}

func (s brokenService) DeleteEvent(context.Context, string, string, string) error {
	return nil
}

// recordingT captures what a check reports instead of failing this test, so
// the suite can be run against a deliberately broken implementation IN-PROCESS.
type recordingT struct{ failures []string }

func (r *recordingT) Helper() {}

func (r *recordingT) Errorf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

// TestRunServiceConformanceRejectsABrokenImplementation proves the suite has
// TEETH: run against an implementation that guesses at ambiguous names, the
// ambiguity check must report a failure. A suite that passes no matter what it
// is run against catches nothing — which is precisely the failure mode this
// package exists to prevent.
//
// Run in-process against a recording stand-in rather than by re-executing the
// test binary, so every assertion branch in conformance.go is actually
// exercised and measured rather than sitting at zero coverage in a child
// process the profiler never sees.
func TestRunServiceConformanceRejectsABrokenImplementation(t *testing.T) {
	var ambiguity serviceConformanceCheck
	for _, check := range serviceConformanceChecks {
		if check.name == "AmbiguousNameIsOmittedWithNilError" {
			ambiguity = check
		}
	}
	if ambiguity.run == nil {
		t.Fatal("the ambiguity check is missing from the suite")
	}

	recorder := &recordingT{}
	ambiguity.run(recorder, func(contacts []convo4calendarius.Contact) convo4calendarius.Service {
		return brokenService{contacts: contacts}
	})
	if len(recorder.failures) == 0 {
		t.Fatal("the suite passed an implementation that guesses at ambiguous names; it must reject it")
	}
	// The broken service GUESSES rather than erroring, so it trips the
	// "want none" branch — naming which contact it wrongly picked.
	if !strings.Contains(recorder.failures[0], "want none") {
		t.Errorf("failure = %q, want it to report that an ambiguous name must resolve to nothing", recorder.failures[0])
	}

	// Every other check must still PASS against the broken service, so a
	// failure here is attributable to the ambiguity rule rather than to the
	// implementation being broken in some incidental way.
	for _, check := range serviceConformanceChecks {
		if check.name == ambiguity.name || check.name == "CountMismatchIsHowACallerDetectsAProblem" {
			continue // both depend on the ambiguity rule
		}
		other := &recordingT{}
		check.run(other, func(contacts []convo4calendarius.Contact) convo4calendarius.Service {
			return brokenService{contacts: contacts}
		})
		if len(other.failures) != 0 {
			t.Errorf("check %s failed for an unrelated reason: %v", check.name, other.failures)
		}
	}
}

// Every check must pass against a correct implementation — otherwise a green
// downstream run would prove nothing.
func TestEveryCheckPassesAgainstTheReferenceImplementation(t *testing.T) {
	for _, check := range serviceConformanceChecks {
		recorder := &recordingT{}
		check.run(recorder, func(contacts []convo4calendarius.Contact) convo4calendarius.Service {
			return referenceService{contacts: contacts}
		})
		if len(recorder.failures) != 0 {
			t.Errorf("check %s failed against the reference implementation: %v", check.name, recorder.failures)
		}
	}
}

// funcService lets a test express one deliberately-broken ResolveContacts
// inline.
type funcService struct {
	resolve func(names []string) ([]convo4calendarius.Contact, error)
}

func (s funcService) ResolveContacts(_ context.Context, _ string, names []string) ([]convo4calendarius.Contact, error) {
	return s.resolve(names)
}

func (s funcService) CreateEvent(context.Context, convo4calendarius.CreateEventRequest) (convo4calendarius.Event, error) {
	return convo4calendarius.Event{}, nil
}
func (s funcService) ListEvents(context.Context, string) ([]convo4calendarius.Event, error) {
	return nil, nil
}
func (s funcService) DeleteEvent(context.Context, string, string, string) error {
	return nil
}

// Every check must REJECT an implementation that breaks the specific rule it
// owns. Without this, a check could assert nothing and the suite would still
// look green — the same "always passes" trap that let a kinder fake hide a real
// bug in the first place. Each case also pins WHICH check is supposed to catch
// which violation, so a rule cannot quietly migrate between checks.
func TestEveryCheckRejectsAViolationOfItsOwnRule(t *testing.T) {
	all := func(contacts []convo4calendarius.Contact) []convo4calendarius.Contact { return contacts }

	for _, tt := range []struct {
		check   string
		broken  func(contacts []convo4calendarius.Contact) convo4calendarius.Service
		because string
	}{
		{"UnambiguousNameResolves",
			func(contacts []convo4calendarius.Contact) convo4calendarius.Service {
				return funcService{resolve: func([]string) ([]convo4calendarius.Contact, error) { return nil, errBroken }}
			},
			"a resolvable name must not error"},
		{"UnambiguousNameResolves",
			func(contacts []convo4calendarius.Contact) convo4calendarius.Service {
				return funcService{resolve: func([]string) ([]convo4calendarius.Contact, error) { return all(contacts), nil }}
			},
			"resolving to the wrong contact must be caught"},
		{"AmbiguousNameIsOmittedWithNilError",
			func(contacts []convo4calendarius.Contact) convo4calendarius.Service {
				return funcService{resolve: func([]string) ([]convo4calendarius.Contact, error) { return nil, errBroken }}
			},
			"an ambiguous name must be omitted, not turned into an error"},
		{"UnknownNameIsOmittedWithNilError",
			func(contacts []convo4calendarius.Contact) convo4calendarius.Service {
				return funcService{resolve: func([]string) ([]convo4calendarius.Contact, error) { return nil, errBroken }}
			},
			"an unknown name must be omitted, not turned into an error"},
		{"UnknownNameIsOmittedWithNilError",
			func(contacts []convo4calendarius.Contact) convo4calendarius.Service {
				return funcService{resolve: func([]string) ([]convo4calendarius.Contact, error) { return all(contacts), nil }}
			},
			"an unknown name must resolve to nothing"},
		{"CountMismatchIsHowACallerDetectsAProblem",
			func(contacts []convo4calendarius.Contact) convo4calendarius.Service {
				return funcService{resolve: func([]string) ([]convo4calendarius.Contact, error) { return nil, errBroken }}
			},
			"a partly-resolvable request must not error"},
		{"CountMismatchIsHowACallerDetectsAProblem",
			func(contacts []convo4calendarius.Contact) convo4calendarius.Service {
				return funcService{resolve: func(names []string) ([]convo4calendarius.Contact, error) {
					resolved := make([]convo4calendarius.Contact, 0, len(names))
					for range names {
						resolved = append(resolved, contacts[0])
					}
					return resolved, nil
				}}
			},
			"resolving every name when two cannot resolve must be caught"},
		{"CountMismatchIsHowACallerDetectsAProblem",
			func(contacts []convo4calendarius.Contact) convo4calendarius.Service {
				return funcService{resolve: func([]string) ([]convo4calendarius.Contact, error) { return contacts[2:3], nil }}
			},
			"resolving too few must be caught"},
		// The RIGHT contacts in the WRONG order: the count matches, so only the
		// identity/order comparison can catch this. Without a case like it, that
		// comparison could be deleted and every test here would still pass — which
		// is precisely the "looks green, asserts nothing" trap this suite exists
		// to prevent.
		{"CountMismatchIsHowACallerDetectsAProblem",
			func(contacts []convo4calendarius.Contact) convo4calendarius.Service {
				return funcService{resolve: func([]string) ([]convo4calendarius.Contact, error) {
					return []convo4calendarius.Contact{contacts[3], contacts[2]}, nil
				}}
			},
			"returning the right contacts in the wrong order must be caught"},
		// Right COUNT, wrong CONTACT — reachable only by the identity comparison.
		{"UnambiguousNameResolves",
			func(contacts []convo4calendarius.Contact) convo4calendarius.Service {
				return funcService{resolve: func([]string) ([]convo4calendarius.Contact, error) {
					return []convo4calendarius.Contact{contacts[0]}, nil // Sarah Connor, not Bob Marley
				}}
			},
			"resolving to one wrong contact must be caught"},
		{"MatchingIsCaseInsensitiveSubstring",
			func(contacts []convo4calendarius.Contact) convo4calendarius.Service {
				return funcService{resolve: func(names []string) ([]convo4calendarius.Contact, error) {
					// Exact-match only: rejects "bob", "MARLEY" and "b Mar".
					for _, contact := range contacts {
						if len(names) == 1 && contact.DisplayName == names[0] {
							return []convo4calendarius.Contact{contact}, nil
						}
					}
					return nil, nil
				}}
			},
			"exact-only matching must be caught"},
	} {
		var check serviceConformanceCheck
		for _, candidate := range serviceConformanceChecks {
			if candidate.name == tt.check {
				check = candidate
			}
		}
		if check.run == nil {
			t.Fatalf("no check named %q", tt.check)
		}
		recorder := &recordingT{}
		check.run(recorder, tt.broken)
		if len(recorder.failures) == 0 {
			t.Errorf("%s passed a broken implementation — %s", tt.check, tt.because)
		}
	}
}

var errBroken = errors.New("broken implementation")

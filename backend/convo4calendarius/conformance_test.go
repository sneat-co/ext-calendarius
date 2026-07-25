package convo4calendarius

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// referenceService is a minimal, deliberately correct implementation of
// ResolveContacts used only to exercise RunServiceConformance itself. It
// mirrors convoservice4calendarius's documented matching rule (case-
// insensitive substring, ambiguous/unknown names omitted with a nil error)
// so the suite's own assertions run against a known-good subject here, not
// just against downstream repos where this module can't run its tests.
type referenceService struct {
	contacts []Contact
}

func (s referenceService) ResolveContacts(_ context.Context, _ string, names []string) ([]Contact, error) {
	resolved := make([]Contact, 0, len(names))
	for _, name := range names {
		var matches []Contact
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

func (s referenceService) CreateEvent(context.Context, CreateEventRequest) (Event, error) {
	return Event{}, nil
}

func (s referenceService) ListEvents(context.Context, string) ([]Event, error) {
	return nil, nil
}

func (s referenceService) DeleteEvent(context.Context, string, string, string) error {
	return nil
}

func TestRunServiceConformanceAgainstReferenceImplementation(t *testing.T) {
	RunServiceConformance(t, func(t *testing.T, contacts []Contact) Service {
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
	contacts []Contact
}

func (s brokenService) ResolveContacts(_ context.Context, _ string, names []string) ([]Contact, error) {
	resolved := make([]Contact, 0, len(names))
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

func (s brokenService) CreateEvent(context.Context, CreateEventRequest) (Event, error) {
	return Event{}, nil
}

func (s brokenService) ListEvents(context.Context, string) ([]Event, error) {
	return nil, nil
}

func (s brokenService) DeleteEvent(context.Context, string, string, string) error {
	return nil
}

// TestRunServiceConformanceRejectsABrokenImplementation runs the suite, in a
// child process, against brokenService. It must FAIL — that is what proves
// the suite has teeth. The assertion runs out-of-process specifically so
// that expected failure does not fail this module's own `go test ./...`.
func TestRunServiceConformanceRejectsABrokenImplementation(t *testing.T) {
	if os.Getenv("CONFORMANCE_SELFTEST_RUN_BROKEN") == "1" {
		RunServiceConformance(t, func(t *testing.T, contacts []Contact) Service {
			t.Helper()
			return brokenService{contacts: contacts}
		})
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestRunServiceConformanceRejectsABrokenImplementation") //nolint:gosec // fixed argv, test-only re-exec of the current test binary
	cmd.Env = append(os.Environ(), "CONFORMANCE_SELFTEST_RUN_BROKEN=1")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("RunServiceConformance passed against an implementation that guesses at ambiguous names; it should have failed:\n%s", output)
	}
	if !strings.Contains(string(output), "AmbiguousNameIsOmittedWithNilError") {
		t.Fatalf("expected the ambiguous-name subtest to be the one that failed, got:\n%s", output)
	}
}

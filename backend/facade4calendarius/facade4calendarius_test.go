package facade4calendarius

import (
	"errors"
	"testing"
)

func TestErrorSentinels(t *testing.T) {
	if ErrRequestIDConflict == nil {
		t.Fatal("ErrRequestIDConflict must not be nil")
	}
	if ErrEventHappeningClosed == nil {
		t.Fatal("ErrEventHappeningClosed must not be nil")
	}
	if ErrRequestIDConflict == ErrEventHappeningClosed {
		t.Fatal("error sentinels must be distinct")
	}
	if ErrEventHappeningVersionConflict == ErrRequestIDConflict || ErrInvalidEventHappening == ErrRequestIDConflict {
		t.Fatal("error sentinels must be distinct")
	}
	// Confirm errors.Is works for direct equality comparisons.
	if !errors.Is(ErrRequestIDConflict, ErrRequestIDConflict) {
		t.Fatal("ErrRequestIDConflict must match itself")
	}
	if !errors.Is(ErrEventHappeningClosed, ErrEventHappeningClosed) {
		t.Fatal("ErrEventHappeningClosed must match itself")
	}
}

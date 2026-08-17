package facade4calendarius

import (
	"errors"
	"testing"
)

func TestErrorSentinels(t *testing.T) {
	sentinels := []error{
		ErrRequestIDConflict,
		ErrEventHappeningClosed,
		ErrEventHappeningVersionConflict,
		ErrInvalidEventHappening,
		ErrEventHappeningUnauthorized,
		ErrEventHappeningNotFound,
		ErrEventHappeningCorrupt,
		ErrEventHappeningListLimitExceeded,
		ErrEventHappeningHierarchyConflict,
		ErrEventHappeningHierarchyCorrupt,
	}
	for i, sentinel := range sentinels {
		if sentinel == nil || !errors.Is(sentinel, sentinel) {
			t.Fatalf("sentinel[%d] must be non-nil and match itself", i)
		}
		for j := i + 1; j < len(sentinels); j++ {
			if sentinel == sentinels[j] {
				t.Fatalf("sentinels[%d] and [%d] must be distinct", i, j)
			}
		}
	}
}

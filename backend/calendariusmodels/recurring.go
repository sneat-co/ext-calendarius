package calendariusmodels

import (
	"encoding/json"
	"time"
)

// RecurringHappening is the public projection of a recurring happening needed
// by extensions that attach their own overlay data. Slots, adjustments and the
// persistence schema remain private to Calendarius.
type RecurringHappening struct {
	ID         string
	Active     bool
	Extensions map[string]json.RawMessage
}

// Occurrence is one expanded recurring slot in an absolute time range.
type Occurrence struct {
	Start time.Time
	End   time.Time
}

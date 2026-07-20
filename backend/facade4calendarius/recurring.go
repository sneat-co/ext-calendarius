package facade4calendarius

import (
	"context"
	"encoding/json"
	"time"

	"github.com/sneat-co/ext-calendarius/backend/calendariusmodels"
)

// RecurringHappeningsFacade exposes recurring calendar behavior without
// leaking Calendarius DBOs, slot schemas or recurrence implementation types.
type RecurringHappeningsFacade interface {
	ListActiveRecurring(ctx context.Context, kind string) ([]calendariusmodels.RecurringHappening, error)
	GetRecurring(ctx context.Context, spaceID, happeningID string) (calendariusmodels.RecurringHappening, error)
	SetExtensionData(ctx context.Context, spaceID, happeningID, extensionID string, data json.RawMessage) error
	ExpandOccurrences(ctx context.Context, spaceID, happeningID, timezone string, from, to time.Time) ([]calendariusmodels.Occurrence, error)
}

// Package facade4calendarius declares the calendar facade interface consumers
// depend on. The host backend (sneat-go) implements it once over the
// calendarius implementation facade and injects it into consuming extensions;
// tests inject fakes. Consumers must not import calendarius/backend directly.
package facade4calendarius

import (
	"context"

	"github.com/sneat-co/ext-calendarius/backend/calendariusmodels"
)

// HappeningsFacade is the cross-extension surface of calendarius happenings.
// It starts with the single operation today's consumers need (the eventius
// adapter pattern); extend it only when a consumer needs more, keeping each
// method implementable by the host in one place.
type HappeningsFacade interface {
	// CreateHappening creates the single happening that backs the consumer's
	// record and returns its id. userID acts within spaceID; the implementation
	// enforces membership and validation.
	CreateHappening(ctx context.Context, userID, spaceID string, spec calendariusmodels.HappeningSpec) (happeningID string, err error)
}

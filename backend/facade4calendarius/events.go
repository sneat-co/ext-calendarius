package facade4calendarius

import (
	"context"
	"errors"

	"github.com/sneat-co/ext-calendarius/backend/calendariusmodels"
)

var (
	// ErrRequestIDConflict is returned when a caller reuses a request ID with a
	// different operation, target or payload.
	ErrRequestIDConflict = errors.New("event happening request ID conflict")

	// ErrEventHappeningClosed is returned when a mutation targets a Happening
	// that is no longer active.
	ErrEventHappeningClosed = errors.New("event happening is closed")

	// ErrEventHappeningVersionConflict is returned when an update is based on a
	// stale EventHappening.Version. The caller must re-read before retrying with
	// a new request ID.
	ErrEventHappeningVersionConflict = errors.New("event happening version conflict")

	// ErrInvalidEventHappening is returned for a contract-validation failure.
	// Providers may wrap the precise field error; callers can use errors.Is.
	ErrInvalidEventHappening = errors.New("invalid event happening")
)

// EventHappeningsFacade exposes Eventius's canonical event lifecycle without
// leaking Calendarius DBOs or slot schemas. EventID is the returned happening
// ID; there is no parallel Event entity.
type EventHappeningsFacade interface {
	CreateEventHappening(
		ctx context.Context,
		userID, spaceID string,
		request calendariusmodels.CreateEventHappeningRequest,
	) (calendariusmodels.EventHappeningMutation, error)

	GetEventHappening(
		ctx context.Context,
		userID, spaceID, happeningID string,
	) (calendariusmodels.EventHappening, error)

	// UpdateEventHappening applies a transactional patch, preserving fields the
	// caller did not provide.
	UpdateEventHappening(
		ctx context.Context,
		userID, spaceID, happeningID string,
		request calendariusmodels.UpdateEventHappeningRequest,
	) (calendariusmodels.EventHappeningMutation, error)

	// ListEventHappenings returns active Event happenings in deterministic
	// order. Scheduled events are ordered chronologically before unscheduled
	// plans.
	ListEventHappenings(
		ctx context.Context,
		userID, spaceID string,
	) ([]calendariusmodels.EventHappening, error)
}

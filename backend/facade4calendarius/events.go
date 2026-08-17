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

	// ErrEventHappeningUnauthorized is returned before any mutation, receipt, or
	// audit write when the principal lacks the Space authority required by the
	// operation.
	ErrEventHappeningUnauthorized = errors.New("event happening operation unauthorized")

	// ErrEventHappeningNotFound also covers an ID whose stored row is neither a
	// supported Event Happening type nor kind=event; the facade never projects
	// another Happening kind.
	ErrEventHappeningNotFound = errors.New("event happening not found")

	// ErrEventHappeningCorrupt is returned when a canonical row exists but cannot
	// produce a validated EventHappening projection. Providers must not return a
	// partially populated or silently redacted projection.
	ErrEventHappeningCorrupt = errors.New("event happening record is corrupt")

	// ErrEventHappeningListLimitExceeded is returned instead of silently
	// truncating when a Space has more than EventHappeningListMax matching rows.
	ErrEventHappeningListLimitExceeded = errors.New("event happening list limit exceeded")

	// ErrEventHappeningHierarchyConflict is returned when a requested immutable
	// parent attachment cannot preserve the finite one-parent, acyclic Linkage
	// hierarchy. A stale ExpectedParentVersion remains the ordinary typed
	// ErrEventHappeningVersionConflict.
	ErrEventHappeningHierarchyConflict = errors.New("event happening hierarchy conflict")

	// ErrEventHappeningHierarchyCorrupt is returned when standard Sneat Linkage
	// cannot be derived as a reciprocal, one-parent, acyclic Happening tree.
	ErrEventHappeningHierarchyCorrupt = errors.New("event happening hierarchy is corrupt")
)

// EventHappeningsFacade exposes the canonical Calendarius Event-Happening
// lifecycle without leaking DBOs or slot schemas. EventID is the returned
// Happening ID; there is no parallel Event entity.
//
// Every method first validates finite principal/Space/reference IDs, then
// authorizes userID against spaceID before revealing or mutating state. Create
// and Update use one durable receipt namespace keyed by
// (userID, spaceID, RequestID). The receipt stores operation, target, the
// normative canonical full-payload fingerprint, the original mutation
// projection, and one audit fact. Receipt creation and the Happening mutation
// are one transaction. An identical replay returns the originally recorded
// Event projection with disposition=reused even if the Event has since changed;
// it creates no Happening, receipt, or audit duplicate. Reusing that scope with
// another payload, target, or operation returns ErrRequestIDConflict. The same
// RequestID under another principal or Space is independent.
//
// Create may atomically persist initial canonical Happening-owned prices, and
// returned EventHappening projections include them. This facade does not create
// a parallel Event price authority: callers select distinct prices by their
// stable HappeningPrice item IDs and use the existing generic Happening pricing
// command surface for later price edits.
//
// A recurring yearly `kind=event` Happening is an annual Series/Cup root;
// single nodes are its editions, Tournaments and games. Recurrence remains
// owned by the existing RecurringHappeningsFacade, never this Event facade.
//
// ParentHappeningID is an immutable first-release attachment convenience. It
// never creates a second hierarchy authority: providers derive projections
// from and atomically write reciprocal standard Sneat Linkage parent/child
// roles plus RelatedIDs. A child create validates the same-Space canonical
// parent and its expected version, rejects a corrupt/cyclic ancestor chain, and
// commits both Linkage endpoints with the child, receipt, prices, and audit.
// Attaching a child increments the canonical parent's Version in that same
// transaction. Parent attachment is immutable here; this contract has no Move
// operation.
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
	// caller did not provide. ExpectedVersion is checked and the complete
	// resulting EventHappening, including type-specific recurrence and scheduling
	// invariants, is validated in the same transaction before the patch and
	// durable idempotency receipt commit.
	UpdateEventHappening(
		ctx context.Context,
		userID, spaceID, happeningID string,
		request calendariusmodels.UpdateEventHappeningRequest,
	) (calendariusmodels.EventHappeningMutation, error)

	// ListEventHappenings returns a flat, non-recursive list containing only
	// active type=single or type=recurring, kind=event Happenings. Each row carries its derived
	// direct Linkage hierarchy projection. Scheduled events are ordered by
	// resolved start instant, then ID. Planning events follow, ordered by
	// CreatedAt then ID. The result is bounded by EventHappeningListMax and
	// returns ErrEventHappeningListLimitExceeded rather than silently truncating.
	ListEventHappenings(
		ctx context.Context,
		userID, spaceID string,
	) ([]calendariusmodels.EventHappening, error)
}

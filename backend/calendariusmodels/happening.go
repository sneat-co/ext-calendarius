// Package calendariusmodels is the shared calendar vocabulary other extensions
// may depend on. It intentionally contains no storage (dbo4*) types and has no
// dependencies beyond the standard library: implementation details stay in
// calendarius/backend; this package only carries what crosses extension
// boundaries.
package calendariusmodels

import (
	"fmt"
	"strings"
	"time"
)

const (
	// EventHappeningTitleMaxLen is shared with Calendarius's persisted
	// Happening title limit.
	EventHappeningTitleMaxLen       = 100
	EventHappeningLocationMaxLen    = 200
	EventHappeningDescriptionMaxLen = 5000
	EventHappeningRequestIDMaxLen   = 200
	// One week is the largest finite duration an Event Happening accepts. Longer
	// multi-day activities need an explicit end instead of an unbounded duration.
	EventHappeningDurationMaxMinutes = 7 * 24 * 60
)

// HappeningSpec is the minimal timing/place a consumer supplies to create the
// single calendarius happening that backs its own record (an eventius event, a
// bookius booking, a school-portal lesson, ...). Generalized from the
// eventius port of the same name; grow fields only when a consumer needs them.
type HappeningSpec struct {
	Title    string
	Start    time.Time
	Location string

	// DurationMinutes is the happening's length in minutes. When zero, the
	// implementation applies its default (60m). Consumers are not required to
	// know an end time — only a start.
	DurationMinutes int
}

// HappeningBrief is the compact read model of an existing happening that may be
// embedded in or returned to other extensions. It mirrors HappeningSpec plus
// identity and cancellation state; it is not the storage schema.
type HappeningBrief struct {
	ID       string
	Title    string
	Start    time.Time
	Location string

	// DurationMinutes is 0 when the happening uses the implementation default.
	DurationMinutes int

	Canceled bool
}

// EventHappeningSpec is the mutable, platform-neutral set of fields used to
// create or replace a single Event happening. Date, time and location are
// intentionally independent: an event can exist while its participants are
// still deciding any of them.
//
// DurationMinutes is meaningful only when Time is populated. A zero duration
// asks Calendarius to apply its default.
type EventHappeningSpec struct {
	Title           string
	Date            string
	Time            string
	EndDate         string
	EndTime         string
	Location        string
	Description     string
	DurationMinutes int
}

// Validate rejects malformed or ambiguous planning input. A date and a time
// may each be known independently, but an explicit end or a duration only
// makes sense once both start components are known. An omitted end is derived
// by Calendarius from DurationMinutes (or its documented default).
func (v EventHappeningSpec) Validate() error {
	if err := validateEventHappeningText("title", v.Title, EventHappeningTitleMaxLen, true); err != nil {
		return err
	}
	if err := validateEventHappeningDate("date", v.Date); err != nil {
		return err
	}
	if err := validateEventHappeningTime("time", v.Time); err != nil {
		return err
	}
	if err := validateEventHappeningDate("endDate", v.EndDate); err != nil {
		return err
	}
	if err := validateEventHappeningTime("endTime", v.EndTime); err != nil {
		return err
	}
	if err := validateEventHappeningText("location", v.Location, EventHappeningLocationMaxLen, false); err != nil {
		return err
	}
	if err := validateEventHappeningText("description", v.Description, EventHappeningDescriptionMaxLen, false); err != nil {
		return err
	}
	if v.DurationMinutes < 0 || v.DurationMinutes > EventHappeningDurationMaxMinutes {
		return fmt.Errorf("durationMinutes must be between 0 and %d", EventHappeningDurationMaxMinutes)
	}
	if (v.EndDate != "" || v.EndTime != "" || v.DurationMinutes != 0) && (v.Date == "" || v.Time == "") {
		return fmt.Errorf("endDate, endTime, and durationMinutes require both date and time")
	}
	if v.EndDate != "" && v.EndTime == "" {
		return fmt.Errorf("endDate requires endTime")
	}
	if v.EndTime != "" && v.DurationMinutes != 0 {
		return fmt.Errorf("endTime and durationMinutes are mutually exclusive")
	}
	return nil
}

// IsScheduled derives whether the spec has enough information to place the
// event on a calendar. Scheduledness is deliberately not persisted as a
// separate state that could drift from the planning fields.
func (v EventHappeningSpec) IsScheduled() bool {
	return v.Date != "" && v.Time != ""
}

// EventHappening is the public projection of a Calendarius happening whose
// semantic kind is "event". Its ID is the canonical event identity; consumers
// must not create a second event record with another ID.
type EventHappening struct {
	ID              string
	Version         int64
	Title           string
	Date            string
	Time            string
	EndDate         string
	EndTime         string
	Location        string
	Description     string
	DurationMinutes int
	Status          EventHappeningStatus
	CreatedBy       string
	CreatedAt       time.Time
}

// EventHappeningStatus is the canonical Calendarius lifecycle projected
// without exposing persistence constants.
type EventHappeningStatus string

const (
	EventHappeningStatusActive    EventHappeningStatus = "active"
	EventHappeningStatusArchived  EventHappeningStatus = "archived"
	EventHappeningStatusCancelled EventHappeningStatus = "cancelled"
	EventHappeningStatusDeleted   EventHappeningStatus = "deleted"
)

// IsScheduled derives whether the event can be placed on a calendar.
func (v EventHappening) IsScheduled() bool {
	return v.Date != "" && v.Time != ""
}

// EventHappeningMutationDisposition describes what a mutation actually did.
// It is returned to callers for deterministic rendering and observability.
type EventHappeningMutationDisposition string

const (
	EventHappeningCreated   EventHappeningMutationDisposition = "created"
	EventHappeningChanged   EventHappeningMutationDisposition = "changed"
	EventHappeningUnchanged EventHappeningMutationDisposition = "unchanged"
	EventHappeningReused    EventHappeningMutationDisposition = "reused"
)

// EventHappeningMutation is the result of an idempotent create or update.
type EventHappeningMutation struct {
	Event       EventHappening
	Disposition EventHappeningMutationDisposition
}

// CreateEventHappeningRequest carries a caller-stable request ID and the full
// initial event plan. Retrying the same request with the same payload reuses
// the original happening. Reusing it with another payload fails closed.
type CreateEventHappeningRequest struct {
	RequestID string
	Spec      EventHappeningSpec
}

// Validate confirms request identity and the initial event plan before a
// provider starts an idempotent operation.
func (v CreateEventHappeningRequest) Validate() error {
	if err := validateEventHappeningRequestID(v.RequestID); err != nil {
		return err
	}
	return v.Spec.Validate()
}

// UpdateEventHappeningRequest is a transactional patch. Nil means "leave the
// field unchanged"; a non-nil empty string clears an optional planning field.
// Title may not be cleared. RequestID has the same idempotency semantics as on
// create.
type UpdateEventHappeningRequest struct {
	RequestID string
	// ExpectedVersion is mandatory optimistic-concurrency protection. Providers
	// return facade4calendarius.ErrEventHappeningVersionConflict when it is stale.
	ExpectedVersion int64
	Title           *string
	Date            *string
	Time            *string
	EndDate         *string
	EndTime         *string
	Location        *string
	Description     *string
	DurationMinutes *int
}

// Validate checks a patch in isolation. Providers must merge it with the
// current EventHappening and call EventHappeningSpec.Validate before writing.
func (v UpdateEventHappeningRequest) Validate() error {
	if err := validateEventHappeningRequestID(v.RequestID); err != nil {
		return err
	}
	if v.ExpectedVersion < 1 {
		return fmt.Errorf("expectedVersion must be positive")
	}
	if v.Title != nil {
		if err := validateEventHappeningText("title", *v.Title, EventHappeningTitleMaxLen, true); err != nil {
			return err
		}
	}
	if v.Date != nil {
		if err := validateEventHappeningDate("date", *v.Date); err != nil {
			return err
		}
	}
	if v.Time != nil {
		if err := validateEventHappeningTime("time", *v.Time); err != nil {
			return err
		}
	}
	if v.EndDate != nil {
		if err := validateEventHappeningDate("endDate", *v.EndDate); err != nil {
			return err
		}
	}
	if v.EndTime != nil {
		if err := validateEventHappeningTime("endTime", *v.EndTime); err != nil {
			return err
		}
	}
	if v.Location != nil {
		if err := validateEventHappeningText("location", *v.Location, EventHappeningLocationMaxLen, false); err != nil {
			return err
		}
	}
	if v.Description != nil {
		if err := validateEventHappeningText("description", *v.Description, EventHappeningDescriptionMaxLen, false); err != nil {
			return err
		}
	}
	if v.DurationMinutes != nil && (*v.DurationMinutes < 0 || *v.DurationMinutes > EventHappeningDurationMaxMinutes) {
		return fmt.Errorf("durationMinutes must be between 0 and %d", EventHappeningDurationMaxMinutes)
	}
	return nil
}

func validateEventHappeningRequestID(value string) error {
	return validateEventHappeningText("requestID", value, EventHappeningRequestIDMaxLen, true)
}

func validateEventHappeningText(field, value string, maxLen int, required bool) error {
	if value == "" {
		if required {
			return fmt.Errorf("%s is required", field)
		}
		return nil
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not have leading or trailing whitespace", field)
	}
	if len(value) > maxLen {
		return fmt.Errorf("%s exceeds maximum length %d", field, maxLen)
	}
	return nil
}

func validateEventHappeningDate(field, value string) error {
	if value == "" {
		return nil
	}
	if _, err := time.Parse(time.DateOnly, value); err != nil {
		return fmt.Errorf("%s must be ISO date: %w", field, err)
	}
	return nil
}

func validateEventHappeningTime(field, value string) error {
	if value == "" {
		return nil
	}
	if _, err := time.Parse("15:04", value); err != nil {
		return fmt.Errorf("%s must be 24-hour HH:MM time: %w", field, err)
	}
	return nil
}

// Package calendariusmodels is the shared calendar vocabulary other extensions
// may depend on. It intentionally contains no storage (dbo4*) types:
// implementation details stay in calendarius/backend. Shared value objects,
// including the existing crediterra/money amount used by Happening prices, are
// reused rather than redefined here.
package calendariusmodels

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// All public string limits are UTF-8 byte limits, not rune counts. This
	// matches persistence and wire-size enforcement across Go and TypeScript.
	EventHappeningIDMaxBytes          = 200
	EventHappeningPrincipalMaxBytes   = 200
	EventHappeningSpaceIDMaxBytes     = 200
	EventHappeningTitleMaxBytes       = 100
	EventHappeningLocationMaxBytes    = 200
	EventHappeningDescriptionMaxBytes = 5000
	EventHappeningRequestIDMaxBytes   = 200
	EventHappeningTimeZoneMaxBytes    = 255
	EventHappeningListMax             = 100
	EventHappeningChildrenMax         = EventHappeningListMax
	// EventHappeningMaxSafeInteger keeps Go integer fields lossless when mirrored
	// by the TypeScript/JSON number contract.
	EventHappeningMaxSafeInteger int64 = 1<<53 - 1
	// One week is the largest finite duration an Event Happening accepts. Longer
	// multi-day events must carry an explicit end instant.
	EventHappeningDurationMaxMinutes = 7 * 24 * 60
)

// Deprecated compatibility aliases. New callers should use the explicitly
// byte-scoped constants above.
const (
	EventHappeningTitleMaxLen       = EventHappeningTitleMaxBytes
	EventHappeningLocationMaxLen    = EventHappeningLocationMaxBytes
	EventHappeningDescriptionMaxLen = EventHappeningDescriptionMaxBytes
	EventHappeningRequestIDMaxLen   = EventHappeningRequestIDMaxBytes
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

type EventHappeningType string

const (
	EventHappeningTypeSingle    EventHappeningType = "single"
	EventHappeningTypeRecurring EventHappeningType = "recurring"
)

// EventHappeningRecurrence deliberately reuses Calendarius's existing slot
// cadence vocabulary. This Event facade does not expand occurrences or own a
// second recurrence engine: the real provider maps this value to its normal
// recurring Happening/slot representation and delegates expansion to
// RecurringHappeningsFacade. The first public hierarchy use case is an annual
// Series/Cup root, hence yearly is the only accepted cadence here.
type EventHappeningRecurrence struct {
	Repeats string
}

func (v EventHappeningRecurrence) Validate() error {
	if v.Repeats != "yearly" {
		return fmt.Errorf("recurrence.repeats must be Calendarius yearly")
	}
	return nil
}

type EventHappeningKind string

const EventHappeningKindEvent EventHappeningKind = "event"

// EventHappeningHierarchy is a finite, non-recursive projection derived from
// standard Sneat Linkage parent/child roles. It is never a separate persisted
// hierarchy authority: providers read and transactionally write reciprocal
// WithRelated plus RelatedIDs records.
type EventHappeningHierarchy struct {
	ParentHappeningID string
	ChildHappeningIDs []string
}

// Validate checks the derived projection. Child IDs are sorted so repeated
// reads are deterministic. Whole-graph cycle and reciprocity checks belong to
// the provider transaction because one projection cannot observe ancestors.
func (v EventHappeningHierarchy) Validate(happeningID string) error {
	if v.ParentHappeningID != "" {
		if err := validateEventHappeningLinkID("parentHappeningID", v.ParentHappeningID); err != nil {
			return err
		}
		if v.ParentHappeningID == happeningID {
			return fmt.Errorf("parentHappeningID must not reference the Happening itself")
		}
	}
	if len(v.ChildHappeningIDs) > EventHappeningChildrenMax {
		return fmt.Errorf("childHappeningIDs exceeds maximum item count %d", EventHappeningChildrenMax)
	}
	for i, childID := range v.ChildHappeningIDs {
		if err := validateEventHappeningLinkID(fmt.Sprintf("childHappeningIDs[%d]", i), childID); err != nil {
			return err
		}
		if childID == happeningID {
			return fmt.Errorf("childHappeningIDs[%d] references the Happening itself", i)
		}
		if i > 0 && v.ChildHappeningIDs[i-1] >= childID {
			return fmt.Errorf("childHappeningIDs must be sorted and unique")
		}
	}
	return nil
}

// EventHappeningSpec is the mutable, platform-neutral plan for one canonical
// Calendarius Happening with type=single and kind=event.
//
// Date and Time are independently optional during planning. Once both exist,
// TimeZone (an IANA TZDB name) and UTCOffset (±HH:MM) are mandatory. The
// explicit offset selects the intended instant during a DST fold and is checked
// against the named zone; a nonexistent DST-gap local time is rejected.
//
// EndTime is an explicit local wall time in the same IANA zone. If EndDate is
// omitted, it means the same local calendar date as Date. EndUTCOffset is
// mandatory with EndTime, and the resulting end instant must be after start.
// DurationMinutes and an explicit end are mutually exclusive. A zero duration
// with no explicit end asks Calendarius to apply its documented default.
type EventHappeningSpec struct {
	Title        string
	Date         string
	Time         string
	TimeZone     string
	UTCOffset    string
	EndDate      string
	EndTime      string
	EndUTCOffset string
	// Location is finite physical-place text. Virtual meeting metadata belongs
	// to a separate extension payload and is not inferred from this field.
	Location        string
	Description     string
	DurationMinutes int
}

// Validate rejects malformed, ambiguous, nonexistent, unbounded, or
// incorrectly ordered planning input.
func (v EventHappeningSpec) Validate() error {
	if err := validateEventHappeningText("title", v.Title, EventHappeningTitleMaxBytes, true); err != nil {
		return err
	}
	if err := validateEventHappeningDate("date", v.Date); err != nil {
		return err
	}
	if err := validateEventHappeningTime("time", v.Time); err != nil {
		return err
	}
	if err := validateEventHappeningTimeZone(v.TimeZone); err != nil {
		return err
	}
	if err := validateEventHappeningOffset("utcOffset", v.UTCOffset); err != nil {
		return err
	}
	if err := validateEventHappeningDate("endDate", v.EndDate); err != nil {
		return err
	}
	if err := validateEventHappeningTime("endTime", v.EndTime); err != nil {
		return err
	}
	if err := validateEventHappeningOffset("endUTCOffset", v.EndUTCOffset); err != nil {
		return err
	}
	if err := validateEventHappeningText("location", v.Location, EventHappeningLocationMaxBytes, false); err != nil {
		return err
	}
	if err := validateEventHappeningText("description", v.Description, EventHappeningDescriptionMaxBytes, false); err != nil {
		return err
	}
	if v.DurationMinutes < 0 || v.DurationMinutes > EventHappeningDurationMaxMinutes {
		return fmt.Errorf("durationMinutes must be between 0 and %d", EventHappeningDurationMaxMinutes)
	}

	completeStart := v.Date != "" && v.Time != ""
	if completeStart {
		if v.TimeZone == "" {
			return fmt.Errorf("timeZone is required when date and time are set")
		}
		if v.UTCOffset == "" {
			return fmt.Errorf("utcOffset is required when date and time are set")
		}
	} else if v.UTCOffset != "" {
		return fmt.Errorf("utcOffset requires both date and time")
	}
	if v.EndDate != "" && v.EndTime == "" {
		return fmt.Errorf("endDate requires endTime")
	}
	if v.EndUTCOffset != "" && v.EndTime == "" {
		return fmt.Errorf("endUTCOffset requires endTime")
	}
	if v.EndTime != "" {
		if !completeStart {
			return fmt.Errorf("endTime requires both date and time")
		}
		if v.EndUTCOffset == "" {
			return fmt.Errorf("endUTCOffset is required with endTime")
		}
	}
	if v.DurationMinutes != 0 && !completeStart {
		return fmt.Errorf("durationMinutes requires both date and time")
	}
	if v.EndTime != "" && v.DurationMinutes != 0 {
		return fmt.Errorf("endTime and durationMinutes are mutually exclusive")
	}

	if completeStart {
		start, err := eventHappeningInstant(v.Date, v.Time, v.TimeZone, v.UTCOffset)
		if err != nil {
			return fmt.Errorf("start local time is invalid: %w", err)
		}
		if v.EndTime != "" {
			endDate := v.EndDate
			if endDate == "" {
				endDate = v.Date
			}
			end, err := eventHappeningInstant(endDate, v.EndTime, v.TimeZone, v.EndUTCOffset)
			if err != nil {
				return fmt.Errorf("end local time is invalid: %w", err)
			}
			if !end.After(start) {
				return fmt.Errorf("end instant must be after start instant")
			}
		}
	}
	return nil
}

// IsScheduled reports whether enough local-time data exists to identify a
// single unambiguous start instant.
func (v EventHappeningSpec) IsScheduled() bool {
	return v.Date != "" && v.Time != "" && v.TimeZone != "" && v.UTCOffset != ""
}

// EventHappening is the validated public projection of one canonical
// Calendarius Happening. Type and Kind are explicit so consumers and provider
// conformance tests can fail closed on accidental non-event rows. Prices reuse
// the generic Happening price authority: this event projection does not define
// a second pricing model or mutate prices.
type EventHappening struct {
	WithHappeningPrices
	Hierarchy       EventHappeningHierarchy
	ID              string
	Type            EventHappeningType
	Recurrence      *EventHappeningRecurrence
	Kind            EventHappeningKind
	Version         int64
	Title           string
	Date            string
	Time            string
	TimeZone        string
	UTCOffset       string
	EndDate         string
	EndTime         string
	EndUTCOffset    string
	Location        string
	Description     string
	DurationMinutes int
	Status          EventHappeningStatus
	CreatedBy       string
	CreatedAt       time.Time
}

// Spec returns the event-facing mutable plan carried by the projection.
func (v EventHappening) Spec() EventHappeningSpec {
	return EventHappeningSpec{
		Title: v.Title, Date: v.Date, Time: v.Time, TimeZone: v.TimeZone, UTCOffset: v.UTCOffset,
		EndDate: v.EndDate, EndTime: v.EndTime, EndUTCOffset: v.EndUTCOffset,
		Location: v.Location, Description: v.Description, DurationMinutes: v.DurationMinutes,
	}
}

// Validate ensures a projection is safe to return across the facade boundary.
func (v EventHappening) Validate() error {
	if err := validateEventHappeningLinkID("id", v.ID); err != nil {
		return err
	}
	switch v.Type {
	case EventHappeningTypeSingle:
		if v.Recurrence != nil {
			return fmt.Errorf("single Happening must not have recurrence")
		}
	case EventHappeningTypeRecurring:
		if v.Recurrence == nil {
			return fmt.Errorf("recurring Happening requires recurrence")
		}
		if err := v.Recurrence.Validate(); err != nil {
			return err
		}
		if v.Date != "" || v.Time != "" || v.UTCOffset != "" || v.EndDate != "" || v.EndTime != "" || v.EndUTCOffset != "" || v.DurationMinutes != 0 {
			return fmt.Errorf("recurring Happening must use Calendarius recurrence instead of a concrete single-event schedule")
		}
	default:
		return fmt.Errorf("unknown happening type %q", v.Type)
	}
	if v.Kind != EventHappeningKindEvent {
		return fmt.Errorf("kind must be %q, got %q", EventHappeningKindEvent, v.Kind)
	}
	if v.Version < 1 || v.Version > EventHappeningMaxSafeInteger {
		return fmt.Errorf("version must be between 1 and %d", EventHappeningMaxSafeInteger)
	}
	if !v.Status.IsValid() {
		return fmt.Errorf("unknown status %q", v.Status)
	}
	if err := validateEventHappeningText("createdBy", v.CreatedBy, EventHappeningPrincipalMaxBytes, true); err != nil {
		return err
	}
	if v.CreatedAt.IsZero() {
		return fmt.Errorf("createdAt is required")
	}
	if v.CreatedAt.Location() != time.UTC {
		return fmt.Errorf("createdAt must use UTC")
	}
	if err := v.WithHappeningPrices.ValidateProjection(); err != nil {
		return err
	}
	if err := v.Hierarchy.Validate(v.ID); err != nil {
		return fmt.Errorf("hierarchy: %w", err)
	}
	return v.Spec().Validate()
}

// EventHappeningStatus is the canonical Calendarius lifecycle projected
// without exposing persistence DBOs.
type EventHappeningStatus string

const (
	EventHappeningStatusActive   EventHappeningStatus = "active"
	EventHappeningStatusArchived EventHappeningStatus = "archived"
	EventHappeningStatusCanceled EventHappeningStatus = "canceled"
	EventHappeningStatusDeleted  EventHappeningStatus = "deleted"
	// Deprecated: use EventHappeningStatusCanceled, matching persisted Calendarius vocabulary.
	EventHappeningStatusCancelled = EventHappeningStatusCanceled
)

func (v EventHappeningStatus) IsValid() bool {
	switch v {
	case EventHappeningStatusActive, EventHappeningStatusArchived, EventHappeningStatusCanceled, EventHappeningStatusDeleted:
		return true
	default:
		return false
	}
}

// IsScheduled derives whether the event identifies an unambiguous start instant.
func (v EventHappening) IsScheduled() bool { return v.Spec().IsScheduled() }

type EventHappeningOperation string

const (
	EventHappeningOperationCreate EventHappeningOperation = "create"
	EventHappeningOperationUpdate EventHappeningOperation = "update"
)

// EventHappeningMutationDisposition describes what a mutation actually did.
type EventHappeningMutationDisposition string

const (
	EventHappeningCreated   EventHappeningMutationDisposition = "created"
	EventHappeningChanged   EventHappeningMutationDisposition = "changed"
	EventHappeningUnchanged EventHappeningMutationDisposition = "unchanged"
	EventHappeningReused    EventHappeningMutationDisposition = "reused"
)

func (v EventHappeningMutationDisposition) IsValid() bool {
	switch v {
	case EventHappeningCreated, EventHappeningChanged, EventHappeningUnchanged, EventHappeningReused:
		return true
	default:
		return false
	}
}

// EventHappeningMutation is the result of an idempotent create or update.
type EventHappeningMutation struct {
	Event       EventHappening
	Disposition EventHappeningMutationDisposition
}

func (v EventHappeningMutation) Validate() error {
	if !v.Disposition.IsValid() {
		return fmt.Errorf("unknown disposition %q", v.Disposition)
	}
	return v.Event.Validate()
}

// EventHappeningRequestScope is the durable idempotency namespace. Providers
// key one receipt by (PrincipalID, SpaceID, RequestID). Operation, target and
// canonical full payload are stored in that receipt and fingerprinted, not
// added to the key: this is what makes cross-operation reuse conflict while the
// same RequestID used by another principal or Space remains independent.
type EventHappeningRequestScope struct {
	PrincipalID string
	SpaceID     string
	RequestID   string
}

func (v EventHappeningRequestScope) Validate() error {
	if err := (EventHappeningAccessScope{PrincipalID: v.PrincipalID, SpaceID: v.SpaceID}).Validate(); err != nil {
		return err
	}
	return validateEventHappeningRequestID(v.RequestID)
}

// EventHappeningAccessScope is the validated principal/Space boundary shared
// by reads and mutations. Providers validate it before authorization or DAL
// access; validation reveals no resource state.
type EventHappeningAccessScope struct {
	PrincipalID string
	SpaceID     string
}

func (v EventHappeningAccessScope) Validate() error {
	if err := validateEventHappeningText("principalID", v.PrincipalID, EventHappeningPrincipalMaxBytes, true); err != nil {
		return err
	}
	return validateEventHappeningText("spaceID", v.SpaceID, EventHappeningSpaceIDMaxBytes, true)
}

// ValidateEventHappeningID validates a same-Space canonical Happening ID at a
// public facade boundary.
func ValidateEventHappeningID(value string) error {
	return validateEventHappeningLinkID("happeningID", value)
}

// CreateEventHappeningRequest carries a caller-stable request ID, the full
// initial event plan, and optional canonical Happening-owned prices. Price IDs
// must already be assigned, matching the existing atomic generic Happening
// create path. Later price edits use that generic pricing command surface.
type CreateEventHappeningRequest struct {
	WithHappeningPrices
	RequestID string
	// Type is explicit for new callers. Empty remains a deprecated compatibility
	// spelling of single while provider fingerprints always use EffectiveType().
	Type                  EventHappeningType
	Recurrence            *EventHappeningRecurrence
	ParentHappeningID     string
	ExpectedParentVersion int64
	Spec                  EventHappeningSpec
}

func (v CreateEventHappeningRequest) Validate() error {
	if err := validateEventHappeningRequestID(v.RequestID); err != nil {
		return err
	}
	if err := v.Spec.Validate(); err != nil {
		return err
	}
	switch v.EffectiveType() {
	case EventHappeningTypeSingle:
		if v.Recurrence != nil {
			return fmt.Errorf("single Happening must not have recurrence")
		}
	case EventHappeningTypeRecurring:
		if v.Recurrence == nil {
			return fmt.Errorf("recurring Happening requires recurrence")
		}
		if err := v.Recurrence.Validate(); err != nil {
			return err
		}
		if v.Spec.Date != "" || v.Spec.Time != "" || v.Spec.UTCOffset != "" || v.Spec.EndDate != "" || v.Spec.EndTime != "" || v.Spec.EndUTCOffset != "" || v.Spec.DurationMinutes != 0 {
			return fmt.Errorf("recurring Happening must use Calendarius recurrence instead of a concrete single-event schedule")
		}
	default:
		return fmt.Errorf("unknown happening type %q", v.Type)
	}
	if v.ParentHappeningID == "" {
		if v.ExpectedParentVersion != 0 {
			return fmt.Errorf("expectedParentVersion requires parentHappeningID")
		}
	} else {
		if err := validateEventHappeningLinkID("parentHappeningID", v.ParentHappeningID); err != nil {
			return err
		}
		if v.ExpectedParentVersion < 1 || v.ExpectedParentVersion > EventHappeningMaxSafeInteger {
			return fmt.Errorf("expectedParentVersion must be between 1 and %d with parentHappeningID", EventHappeningMaxSafeInteger)
		}
	}
	return v.WithHappeningPrices.ValidateProjection()
}

func (v CreateEventHappeningRequest) EffectiveType() EventHappeningType {
	if v.Type == "" {
		return EventHappeningTypeSingle
	}
	return v.Type
}

// Fingerprint returns the normative SHA-256 fingerprint of the operation and
// canonical full payload. Providers persist it with the request receipt.
func (v CreateEventHappeningRequest) Fingerprint() (string, error) {
	if err := validateEventHappeningRequestID(v.RequestID); err != nil {
		return "", err
	}
	w := newEventHappeningFingerprintWriter(EventHappeningOperationCreate)
	w.writeString(v.RequestID)
	w.writeString(string(v.EffectiveType()))
	if v.Recurrence == nil {
		w.writeString("")
	} else {
		w.writeString(v.Recurrence.Repeats)
	}
	w.writeString(v.ParentHappeningID)
	w.writeInt64(v.ExpectedParentVersion)
	w.writeSpec(v.Spec)
	w.writePrices(v.Prices)
	return w.sum(), nil
}

// UpdateEventHappeningRequest is a transactional patch. Nil means leave the
// field unchanged; a non-nil empty string clears an optional planning field.
type UpdateEventHappeningRequest struct {
	RequestID       string
	ExpectedVersion int64
	Title           *string
	Date            *string
	Time            *string
	TimeZone        *string
	UTCOffset       *string
	EndDate         *string
	EndTime         *string
	EndUTCOffset    *string
	Location        *string
	Description     *string
	DurationMinutes *int
}

// Validate checks a patch in isolation. Providers merge it with the current
// projection and validate the resulting EventHappeningSpec atomically.
func (v UpdateEventHappeningRequest) Validate() error {
	if err := validateEventHappeningRequestID(v.RequestID); err != nil {
		return err
	}
	if v.ExpectedVersion < 1 || v.ExpectedVersion > EventHappeningMaxSafeInteger {
		return fmt.Errorf("expectedVersion must be between 1 and %d", EventHappeningMaxSafeInteger)
	}
	if v.Title != nil {
		if err := validateEventHappeningText("title", *v.Title, EventHappeningTitleMaxBytes, true); err != nil {
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
	if v.TimeZone != nil {
		if err := validateEventHappeningTimeZone(*v.TimeZone); err != nil {
			return err
		}
	}
	if v.UTCOffset != nil {
		if err := validateEventHappeningOffset("utcOffset", *v.UTCOffset); err != nil {
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
	if v.EndUTCOffset != nil {
		if err := validateEventHappeningOffset("endUTCOffset", *v.EndUTCOffset); err != nil {
			return err
		}
	}
	if v.Location != nil {
		if err := validateEventHappeningText("location", *v.Location, EventHappeningLocationMaxBytes, false); err != nil {
			return err
		}
	}
	if v.Description != nil {
		if err := validateEventHappeningText("description", *v.Description, EventHappeningDescriptionMaxBytes, false); err != nil {
			return err
		}
	}
	if v.DurationMinutes != nil && (*v.DurationMinutes < 0 || *v.DurationMinutes > EventHappeningDurationMaxMinutes) {
		return fmt.Errorf("durationMinutes must be between 0 and %d", EventHappeningDurationMaxMinutes)
	}
	return nil
}

// Fingerprint returns the normative SHA-256 fingerprint of operation, target,
// expected version, and every patch field including nil-vs-clear distinctions.
func (v UpdateEventHappeningRequest) Fingerprint(happeningID string) (string, error) {
	if err := ValidateEventHappeningID(happeningID); err != nil {
		return "", err
	}
	if err := validateEventHappeningRequestID(v.RequestID); err != nil {
		return "", err
	}
	w := newEventHappeningFingerprintWriter(EventHappeningOperationUpdate)
	w.writeString(happeningID)
	w.writeString(v.RequestID)
	w.writeInt64(v.ExpectedVersion)
	w.writeOptionalString(v.Title)
	w.writeOptionalString(v.Date)
	w.writeOptionalString(v.Time)
	w.writeOptionalString(v.TimeZone)
	w.writeOptionalString(v.UTCOffset)
	w.writeOptionalString(v.EndDate)
	w.writeOptionalString(v.EndTime)
	w.writeOptionalString(v.EndUTCOffset)
	w.writeOptionalString(v.Location)
	w.writeOptionalString(v.Description)
	w.writeOptionalInt(v.DurationMinutes)
	return w.sum(), nil
}

// eventHappeningFingerprintWriter defines the normative canonical payload:
// versioned, fixed field order, length-prefixed raw string bytes, explicit
// pointer presence markers, and big-endian signed integers. Raw bytes are
// retained even for malformed UTF-8, so an invalid changed payload cannot
// collide with the replacement-rune encoding of a prior valid payload.
type eventHappeningFingerprintWriter struct{ hash hash.Hash }

func newEventHappeningFingerprintWriter(operation EventHappeningOperation) *eventHappeningFingerprintWriter {
	w := &eventHappeningFingerprintWriter{hash: sha256.New()}
	w.writeString("calendarius/event-happening-request/v1")
	w.writeString(string(operation))
	return w
}

func (w *eventHappeningFingerprintWriter) writeSpec(v EventHappeningSpec) {
	w.writeString(v.Title)
	w.writeString(v.Date)
	w.writeString(v.Time)
	w.writeString(v.TimeZone)
	w.writeString(v.UTCOffset)
	w.writeString(v.EndDate)
	w.writeString(v.EndTime)
	w.writeString(v.EndUTCOffset)
	w.writeString(v.Location)
	w.writeString(v.Description)
	w.writeInt64(int64(v.DurationMinutes))
}

func (w *eventHappeningFingerprintWriter) writePrices(prices []*HappeningPrice) {
	w.writeInt64(int64(len(prices)))
	for _, price := range prices {
		if price == nil {
			_, _ = w.hash.Write([]byte{0})
			continue
		}
		_, _ = w.hash.Write([]byte{1})
		w.writeString(price.ID)
		w.writeString(string(price.Term.Unit))
		w.writeInt64(int64(price.Term.Length))
		w.writeString(string(price.Amount.Currency))
		w.writeInt64(int64(price.Amount.Value))
		w.writeInt64(int64(price.ExpenseQuantity))
	}
}

func (w *eventHappeningFingerprintWriter) writeString(value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = w.hash.Write(size[:])
	_, _ = w.hash.Write([]byte(value))
}

func (w *eventHappeningFingerprintWriter) writeInt64(value int64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], uint64(value))
	_, _ = w.hash.Write(encoded[:])
}

func (w *eventHappeningFingerprintWriter) writeOptionalString(value *string) {
	if value == nil {
		_, _ = w.hash.Write([]byte{0})
		return
	}
	_, _ = w.hash.Write([]byte{1})
	w.writeString(*value)
}

func (w *eventHappeningFingerprintWriter) writeOptionalInt(value *int) {
	if value == nil {
		_, _ = w.hash.Write([]byte{0})
		return
	}
	_, _ = w.hash.Write([]byte{1})
	w.writeInt64(int64(*value))
}

func (w *eventHappeningFingerprintWriter) sum() string {
	return hex.EncodeToString(w.hash.Sum(nil))
}

func validateEventHappeningRequestID(value string) error {
	return validateEventHappeningText("requestID", value, EventHappeningRequestIDMaxBytes, true)
}

func validateEventHappeningLinkID(field, value string) error {
	if err := validateEventHappeningText(field, value, EventHappeningIDMaxBytes, true); err != nil {
		return err
	}
	if strings.Contains(value, "@") {
		return fmt.Errorf("%s must be a same-Space bare Happening ID", field)
	}
	return nil
}

func validateEventHappeningText(field, value string, maxBytes int, required bool) error {
	if value == "" {
		if required {
			return fmt.Errorf("%s is required", field)
		}
		return nil
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s must be valid UTF-8", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not have leading or trailing whitespace", field)
	}
	if len(value) > maxBytes {
		return fmt.Errorf("%s exceeds maximum UTF-8 byte length %d", field, maxBytes)
	}
	return nil
}

func validateEventHappeningDate(field, value string) error {
	if value == "" {
		return nil
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s must be valid UTF-8", field)
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
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s must be valid UTF-8", field)
	}
	if _, err := time.Parse("15:04", value); err != nil {
		return fmt.Errorf("%s must be 24-hour HH:MM time: %w", field, err)
	}
	return nil
}

func validateEventHappeningTimeZone(value string) error {
	if value == "" {
		return nil
	}
	if err := validateEventHappeningText("timeZone", value, EventHappeningTimeZoneMaxBytes, false); err != nil {
		return err
	}
	if value == "Local" {
		return fmt.Errorf("timeZone must be an explicit IANA TZDB name, not Local")
	}
	if _, err := time.LoadLocation(value); err != nil {
		return fmt.Errorf("timeZone must be an IANA TZDB name: %w", err)
	}
	return nil
}

func validateEventHappeningOffset(field, value string) error {
	if value == "" {
		return nil
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s must be valid UTF-8", field)
	}
	if _, err := eventHappeningOffsetSeconds(value); err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	return nil
}

func eventHappeningOffsetSeconds(value string) (int, error) {
	if len(value) != 6 || (value[0] != '+' && value[0] != '-') || value[3] != ':' {
		return 0, fmt.Errorf("must use ±HH:MM")
	}
	hour := int(value[1]-'0')*10 + int(value[2]-'0')
	minute := int(value[4]-'0')*10 + int(value[5]-'0')
	if value[1] < '0' || value[1] > '9' || value[2] < '0' || value[2] > '9' ||
		value[4] < '0' || value[4] > '9' || value[5] < '0' || value[5] > '9' ||
		minute > 59 || hour > 14 || (hour == 14 && minute != 0) {
		return 0, fmt.Errorf("must be a real UTC offset between -14:00 and +14:00")
	}
	seconds := hour*60*60 + minute*60
	if value[0] == '-' {
		seconds = -seconds
	}
	return seconds, nil
}

// eventHappeningInstant resolves a local wall time using both a named IANA zone
// and an explicit offset. Converting the candidate instant back through the
// named zone rejects DST gaps and offsets that do not select either side of a
// DST fold.
func eventHappeningInstant(date, clock, zoneName, offsetText string) (time.Time, error) {
	dateValue, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return time.Time{}, err
	}
	timeValue, err := time.Parse("15:04", clock)
	if err != nil {
		return time.Time{}, err
	}
	offsetSeconds, err := eventHappeningOffsetSeconds(offsetText)
	if err != nil {
		return time.Time{}, err
	}
	location, err := time.LoadLocation(zoneName)
	if err != nil {
		return time.Time{}, err
	}
	fixed := time.FixedZone("event-offset", offsetSeconds)
	candidate := time.Date(dateValue.Year(), dateValue.Month(), dateValue.Day(), timeValue.Hour(), timeValue.Minute(), 0, 0, fixed).UTC()
	local := candidate.In(location)
	_, actualOffset := local.Zone()
	if local.Year() != dateValue.Year() || local.Month() != dateValue.Month() || local.Day() != dateValue.Day() ||
		local.Hour() != timeValue.Hour() || local.Minute() != timeValue.Minute() || actualOffset != offsetSeconds {
		return time.Time{}, fmt.Errorf("local time/offset does not exist in IANA zone %q", zoneName)
	}
	return candidate, nil
}

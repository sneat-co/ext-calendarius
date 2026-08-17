package calendariusmodels

import (
	"strings"
	"testing"
	"time"
)

func scheduledEventSpec() EventHappeningSpec {
	return EventHappeningSpec{
		Title: "Picnic", Date: "2026-08-01", Time: "12:30",
		TimeZone: "Europe/Dublin", UTCOffset: "+01:00",
		EndTime: "14:00", EndUTCOffset: "+01:00", Location: "Phoenix Park",
	}
}

func TestEventHappeningSchedulednessRequiresUnambiguousInstant(t *testing.T) {
	for _, tt := range []struct {
		name      string
		date      string
		clock     string
		zone      string
		offset    string
		scheduled bool
	}{
		{name: "title only"},
		{name: "date only", date: "2026-08-01"},
		{name: "time only", clock: "18:30"},
		{name: "local fields without zone", date: "2026-08-01", clock: "18:30"},
		{name: "unambiguous instant", date: "2026-08-01", clock: "18:30", zone: "Europe/Dublin", offset: "+01:00", scheduled: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			spec := EventHappeningSpec{Title: "Picnic", Date: tt.date, Time: tt.clock, TimeZone: tt.zone, UTCOffset: tt.offset}
			if got := spec.IsScheduled(); got != tt.scheduled {
				t.Fatalf("EventHappeningSpec.IsScheduled() = %v, want %v", got, tt.scheduled)
			}
			event := EventHappening{Title: spec.Title, Date: spec.Date, Time: spec.Time, TimeZone: spec.TimeZone, UTCOffset: spec.UTCOffset}
			if got := event.IsScheduled(); got != tt.scheduled {
				t.Fatalf("EventHappening.IsScheduled() = %v, want %v", got, tt.scheduled)
			}
		})
	}
}

func TestEventHappeningSpecValidate(t *testing.T) {
	valid := scheduledEventSpec()
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid scheduled event: %v", err)
	}
	sameDay := valid
	sameDay.EndDate = ""
	if err := sameDay.Validate(); err != nil {
		t.Fatalf("omitted end date means same local date: %v", err)
	}

	for _, tt := range []struct {
		name string
		spec EventHappeningSpec
	}{
		{name: "missing title", spec: EventHappeningSpec{}},
		{name: "malformed date", spec: EventHappeningSpec{Title: "Picnic", Date: "not-a-date"}},
		{name: "malformed time", spec: EventHappeningSpec{Title: "Picnic", Time: "noon"}},
		{name: "scheduled missing zone", spec: EventHappeningSpec{Title: "Picnic", Date: "2026-08-01", Time: "12:30", UTCOffset: "+01:00"}},
		{name: "scheduled missing offset", spec: EventHappeningSpec{Title: "Picnic", Date: "2026-08-01", Time: "12:30", TimeZone: "Europe/Dublin"}},
		{name: "offset does not match zone", spec: EventHappeningSpec{Title: "Picnic", Date: "2026-08-01", Time: "12:30", TimeZone: "Europe/Dublin", UTCOffset: "+00:00"}},
		{name: "DST gap", spec: EventHappeningSpec{Title: "Picnic", Date: "2026-03-29", Time: "01:30", TimeZone: "Europe/Dublin", UTCOffset: "+00:00"}},
		{name: "unknown timezone", spec: EventHappeningSpec{Title: "Picnic", Date: "2026-08-01", Time: "12:30", TimeZone: "Mars/Olympus", UTCOffset: "+00:00"}},
		{name: "duration before schedule", spec: EventHappeningSpec{Title: "Picnic", DurationMinutes: 60}},
		{name: "end date without end time", spec: EventHappeningSpec{Title: "Picnic", EndDate: "2026-08-01"}},
		{name: "end and duration conflict", spec: func() EventHappeningSpec { v := valid; v.DurationMinutes = 60; return v }()},
		{name: "end before start", spec: func() EventHappeningSpec { v := valid; v.EndTime = "11:00"; return v }()},
		{name: "end equals start", spec: func() EventHappeningSpec { v := valid; v.EndTime = v.Time; return v }()},
		{name: "non finite duration", spec: EventHappeningSpec{Title: "Picnic", Date: "2026-08-01", Time: "12:30", TimeZone: "Europe/Dublin", UTCOffset: "+01:00", DurationMinutes: EventHappeningDurationMaxMinutes + 1}},
		{name: "invalid UTF-8", spec: EventHappeningSpec{Title: string([]byte{0xff})}},
		{name: "multibyte byte bound", spec: EventHappeningSpec{Title: strings.Repeat("é", EventHappeningTitleMaxBytes/2+1)}},
		{name: "invalid UTF-8 location", spec: EventHappeningSpec{Title: "Picnic", Location: string([]byte{0xff})}},
		{name: "location byte bound", spec: EventHappeningSpec{Title: "Picnic", Location: strings.Repeat("é", EventHappeningLocationMaxBytes/2+1)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.spec.Validate(); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}

func TestEventHappeningDSTFoldRequiresAndHonorsOffset(t *testing.T) {
	first := EventHappeningSpec{Title: "Fold", Date: "2026-10-25", Time: "01:30", TimeZone: "Europe/Dublin", UTCOffset: "+01:00", DurationMinutes: 30}
	second := first
	second.UTCOffset = "+00:00"
	if err := first.Validate(); err != nil {
		t.Fatalf("first fold occurrence: %v", err)
	}
	if err := second.Validate(); err != nil {
		t.Fatalf("second fold occurrence: %v", err)
	}
	firstInstant, _ := eventHappeningInstant(first.Date, first.Time, first.TimeZone, first.UTCOffset)
	secondInstant, _ := eventHappeningInstant(second.Date, second.Time, second.TimeZone, second.UTCOffset)
	if !secondInstant.After(firstInstant) {
		t.Fatalf("explicit offsets did not disambiguate fold: first=%v second=%v", firstInstant, secondInstant)
	}
}

func TestEventHappeningProjectionValidate(t *testing.T) {
	spec := scheduledEventSpec()
	event := EventHappening{
		ID: "event1", Type: EventHappeningTypeSingle, Kind: EventHappeningKindEvent,
		Version: 1, Title: spec.Title, Date: spec.Date, Time: spec.Time, TimeZone: spec.TimeZone,
		UTCOffset: spec.UTCOffset, EndTime: spec.EndTime, EndUTCOffset: spec.EndUTCOffset,
		Location: spec.Location, Status: EventHappeningStatusActive,
		CreatedBy: "user1", CreatedAt: time.Unix(1, 0).UTC(),
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("valid projection: %v", err)
	}
	for _, mutate := range []func(*EventHappening){
		func(v *EventHappening) { v.ID = "" },
		func(v *EventHappening) { v.Type = "recurring" },
		func(v *EventHappening) { v.Kind = "appointment" },
		func(v *EventHappening) { v.Version = 0 },
		func(v *EventHappening) { v.Version = EventHappeningMaxSafeInteger + 1 },
		func(v *EventHappening) { v.Status = "unknown" },
		func(v *EventHappening) { v.CreatedBy = "" },
		func(v *EventHappening) { v.CreatedAt = time.Time{} },
		func(v *EventHappening) { v.CreatedAt = time.Unix(1, 0).In(time.FixedZone("zero", 0)) },
	} {
		copy := event
		mutate(&copy)
		if err := copy.Validate(); err == nil {
			t.Fatalf("invalid projection accepted: %+v", copy)
		}
	}
}

func TestEventHappeningHierarchyValidate(t *testing.T) {
	valid := EventHappeningHierarchy{ParentHappeningID: "parent", ChildHappeningIDs: []string{"child-a", "child-b"}}
	if err := valid.Validate("event1"); err != nil {
		t.Fatalf("valid hierarchy: %v", err)
	}
	for _, hierarchy := range []EventHappeningHierarchy{
		{ParentHappeningID: "event1"},
		{ParentHappeningID: "parent@other-space"},
		{ChildHappeningIDs: []string{"event1"}},
		{ChildHappeningIDs: []string{"child-b", "child-a"}},
		{ChildHappeningIDs: []string{"child-a", "child-a"}},
		{ChildHappeningIDs: make([]string, EventHappeningChildrenMax+1)},
	} {
		if err := hierarchy.Validate("event1"); err == nil {
			t.Fatalf("invalid hierarchy accepted: %+v", hierarchy)
		}
	}
}

func TestEventHappeningRequestFingerprints(t *testing.T) {
	create := CreateEventHappeningRequest{
		RequestID: "request1", Spec: EventHappeningSpec{Title: "Picnic"}, WithHappeningPrices: validHappeningPrices(),
	}
	first, err := create.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	again, _ := create.Fingerprint()
	if first != again || len(first) != sha256HexLen {
		t.Fatalf("fingerprint is not stable SHA-256: %q %q", first, again)
	}
	changed := create
	changed.Spec.Title = "Changed"
	changedFingerprint, _ := changed.Fingerprint()
	if changedFingerprint == first {
		t.Fatal("changed payload retained fingerprint")
	}
	changedPrice := create
	changedPrice.WithHappeningPrices = validHappeningPrices()
	changedPrice.Prices[0].Amount.Value++
	changedPriceFingerprint, _ := changedPrice.Fingerprint()
	if changedPriceFingerprint == first {
		t.Fatal("changed canonical Happening price retained fingerprint")
	}
	childCreate := create
	childCreate.ParentHappeningID = "parent1"
	childCreate.ExpectedParentVersion = 2
	childFingerprint, _ := childCreate.Fingerprint()
	if childFingerprint == first {
		t.Fatal("parent attachment retained root-create fingerprint")
	}
	title := "Changed"
	update := UpdateEventHappeningRequest{RequestID: create.RequestID, ExpectedVersion: 1, Title: &title}
	updateFingerprint, err := update.Fingerprint("event1")
	if err != nil {
		t.Fatal(err)
	}
	if updateFingerprint == first {
		t.Fatal("cross-operation payload retained fingerprint")
	}
	invalidUTF8 := CreateEventHappeningRequest{RequestID: "request1", Spec: EventHappeningSpec{Title: string([]byte{0xff})}}
	replacementRune := CreateEventHappeningRequest{RequestID: "request1", Spec: EventHappeningSpec{Title: "�"}}
	invalidFingerprint, _ := invalidUTF8.Fingerprint()
	replacementFingerprint, _ := replacementRune.Fingerprint()
	if invalidFingerprint == replacementFingerprint {
		t.Fatal("canonical fingerprint replaced malformed UTF-8 and collided with a valid payload")
	}
}

const sha256HexLen = 64

func TestUpdateEventHappeningRequestValidate(t *testing.T) {
	title := "Picnic"
	if err := (UpdateEventHappeningRequest{RequestID: "update-1", ExpectedVersion: 1, Title: &title}).Validate(); err != nil {
		t.Fatalf("valid patch: %v", err)
	}
	if err := (UpdateEventHappeningRequest{RequestID: "update-1"}).Validate(); err == nil {
		t.Fatal("missing expected version was accepted")
	}
	if err := (UpdateEventHappeningRequest{RequestID: "update-1", ExpectedVersion: EventHappeningMaxSafeInteger + 1}).Validate(); err == nil {
		t.Fatal("unsafe expected version was accepted")
	}
}

func TestCreateEventHappeningRequestHierarchyValidate(t *testing.T) {
	valid := CreateEventHappeningRequest{
		RequestID: "child-create", ParentHappeningID: "parent1", ExpectedParentVersion: 1,
		Spec: EventHappeningSpec{Title: "Child"},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid child create: %v", err)
	}
	for _, request := range []CreateEventHappeningRequest{
		{RequestID: "bad1", ExpectedParentVersion: 1, Spec: EventHappeningSpec{Title: "Child"}},
		{RequestID: "bad2", ParentHappeningID: "parent1", Spec: EventHappeningSpec{Title: "Child"}},
		{RequestID: "bad3", ParentHappeningID: "parent@space2", ExpectedParentVersion: 1, Spec: EventHappeningSpec{Title: "Child"}},
		{RequestID: "bad4", ParentHappeningID: "parent1", ExpectedParentVersion: EventHappeningMaxSafeInteger + 1, Spec: EventHappeningSpec{Title: "Child"}},
	} {
		if err := request.Validate(); err == nil {
			t.Fatalf("invalid child create accepted: %+v", request)
		}
	}
}

func TestRecurringEventHappeningIsYearlyAndChangesCreateFingerprint(t *testing.T) {
	series := CreateEventHappeningRequest{
		RequestID: "annual-cup", Type: EventHappeningTypeRecurring,
		Recurrence: &EventHappeningRecurrence{Repeats: "yearly"},
		Spec:       EventHappeningSpec{Title: "Annual cup"},
	}
	if err := series.Validate(); err != nil {
		t.Fatalf("valid yearly series: %v", err)
	}
	single := series
	single.Type, single.Recurrence = EventHappeningTypeSingle, nil
	seriesFingerprint, _ := series.Fingerprint()
	singleFingerprint, _ := single.Fingerprint()
	if seriesFingerprint == singleFingerprint {
		t.Fatal("type/recurrence did not affect create fingerprint")
	}
	series.Spec.Date, series.Spec.Time, series.Spec.TimeZone, series.Spec.UTCOffset = "2026-01-01", "12:00", "Europe/Dublin", "+00:00"
	if err := series.Validate(); err == nil {
		t.Fatal("recurring series accepted a concrete single-event schedule")
	}
}

func TestEventHappeningScopesValidateUTF8ByteBounds(t *testing.T) {
	if err := (EventHappeningRequestScope{PrincipalID: "user1", SpaceID: "space1", RequestID: "request1"}).Validate(); err != nil {
		t.Fatalf("valid request scope: %v", err)
	}
	for _, scope := range []EventHappeningRequestScope{
		{SpaceID: "space1", RequestID: "request1"},
		{PrincipalID: string([]byte{0xff}), SpaceID: "space1", RequestID: "request1"},
		{PrincipalID: strings.Repeat("é", EventHappeningPrincipalMaxBytes/2+1), SpaceID: "space1", RequestID: "request1"},
		{PrincipalID: "user1", SpaceID: string([]byte{0xff}), RequestID: "request1"},
		{PrincipalID: "user1", SpaceID: strings.Repeat("é", EventHappeningSpaceIDMaxBytes/2+1), RequestID: "request1"},
	} {
		if err := scope.Validate(); err == nil {
			t.Fatalf("invalid scope accepted: %+v", scope)
		}
	}
}

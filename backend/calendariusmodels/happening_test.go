package calendariusmodels

import "testing"

func TestEventHappeningSchedulednessIsDerived(t *testing.T) {
	for _, tt := range []struct {
		name      string
		date      string
		time      string
		scheduled bool
	}{
		{name: "title only"},
		{name: "date only", date: "2026-08-01"},
		{name: "time only", time: "18:30"},
		{name: "date and time", date: "2026-08-01", time: "18:30", scheduled: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			spec := EventHappeningSpec{Title: "Picnic", Date: tt.date, Time: tt.time}
			if got := spec.IsScheduled(); got != tt.scheduled {
				t.Fatalf("EventHappeningSpec.IsScheduled() = %v, want %v", got, tt.scheduled)
			}
			event := EventHappening{Title: spec.Title, Date: spec.Date, Time: spec.Time}
			if got := event.IsScheduled(); got != tt.scheduled {
				t.Fatalf("EventHappening.IsScheduled() = %v, want %v", got, tt.scheduled)
			}
		})
	}
}

func TestEventHappeningSpecValidate(t *testing.T) {
	valid := EventHappeningSpec{
		Title: "Picnic", Date: "2026-08-01", Time: "12:30", EndTime: "14:00", Location: "Phoenix Park",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid planned event: %v", err)
	}
	for _, tt := range []struct {
		name string
		spec EventHappeningSpec
	}{
		{name: "missing title", spec: EventHappeningSpec{}},
		{name: "malformed date", spec: EventHappeningSpec{Title: "Picnic", Date: "not-a-date"}},
		{name: "malformed time", spec: EventHappeningSpec{Title: "Picnic", Time: "noon"}},
		{name: "duration before schedule", spec: EventHappeningSpec{Title: "Picnic", DurationMinutes: 60}},
		{name: "end date without end time", spec: EventHappeningSpec{Title: "Picnic", Date: "2026-08-01", Time: "12:30", EndDate: "2026-08-01"}},
		{name: "end and duration conflict", spec: EventHappeningSpec{Title: "Picnic", Date: "2026-08-01", Time: "12:30", EndTime: "14:00", DurationMinutes: 60}},
		{name: "non finite duration", spec: EventHappeningSpec{Title: "Picnic", Date: "2026-08-01", Time: "12:30", DurationMinutes: EventHappeningDurationMaxMinutes + 1}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.spec.Validate(); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}

func TestUpdateEventHappeningRequestValidate(t *testing.T) {
	title := "Picnic"
	if err := (UpdateEventHappeningRequest{RequestID: "update-1", ExpectedVersion: 1, Title: &title}).Validate(); err != nil {
		t.Fatalf("valid patch: %v", err)
	}
	if err := (UpdateEventHappeningRequest{RequestID: "update-1"}).Validate(); err == nil {
		t.Fatal("missing expected version was accepted")
	}
}

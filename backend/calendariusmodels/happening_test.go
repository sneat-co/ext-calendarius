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

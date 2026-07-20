package convo4calendarius

import "testing"

func TestCreateEventRequestValid(t *testing.T) {
	t.Parallel()
	if !((CreateEventRequest{SpaceID: "space", Title: "Planning", Date: "2026-07-20", Time: "10:00", DurationMinutes: 30}).Valid()) {
		t.Fatal("complete request must be valid")
	}
	if (CreateEventRequest{SpaceID: "space", Title: "Planning", Date: "2026-07-20", Time: "10:00"}).Valid() {
		t.Fatal("request without duration must be invalid")
	}
}

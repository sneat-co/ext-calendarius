// Package facade4calendariustest contains reusable conformance checks for
// public Calendarius facades. It is separate from production contract packages
// so consumers do not inherit a dependency on testing.
package facade4calendariustest

import (
	"context"
	"errors"
	"testing"

	"github.com/sneat-co/ext-calendarius/backend/calendariusmodels"
	"github.com/sneat-co/ext-calendarius/backend/facade4calendarius"
)

const (
	conformanceUserID  = "user1"
	conformanceSpaceID = "space1"
)

// RunEventHappeningsFacadeConformance verifies the planning lifecycle that all
// real and fake EventHappeningsFacade implementations must share.
//
// newFacade must return a fresh implementation for each subtest.
func RunEventHappeningsFacadeConformance(
	t *testing.T,
	newFacade func(t *testing.T) facade4calendarius.EventHappeningsFacade,
) {
	t.Helper()

	t.Run("TitleOnlyEventIsCanonicalAndUnscheduled", func(t *testing.T) {
		facade := newFacade(t)
		created, err := facade.CreateEventHappening(
			context.Background(),
			conformanceUserID,
			conformanceSpaceID,
			calendariusmodels.CreateEventHappeningRequest{
				RequestID: "create-title-only",
				Spec:      calendariusmodels.EventHappeningSpec{Title: "Summer picnic"},
			},
		)
		if err != nil {
			t.Fatalf("CreateEventHappening() error = %v", err)
		}
		if created.Disposition != calendariusmodels.EventHappeningCreated {
			t.Fatalf("CreateEventHappening() disposition = %q, want created", created.Disposition)
		}
		if created.Event.ID == "" {
			t.Fatal("CreateEventHappening() returned an empty canonical ID")
		}
		if created.Event.CreatedBy != conformanceUserID {
			t.Fatalf("CreateEventHappening() CreatedBy = %q, want %q", created.Event.CreatedBy, conformanceUserID)
		}
		if created.Event.CreatedAt.IsZero() {
			t.Fatal("CreateEventHappening() returned no creation timestamp")
		}
		if created.Event.IsScheduled() {
			t.Fatalf("title-only event is scheduled: %+v", created)
		}

		got, err := facade.GetEventHappening(
			context.Background(), conformanceUserID, conformanceSpaceID, created.Event.ID,
		)
		if err != nil {
			t.Fatalf("GetEventHappening() error = %v", err)
		}
		if got.ID != created.Event.ID || got.Title != created.Event.Title {
			t.Fatalf("GetEventHappening() = %+v, want canonical event %+v", got, created)
		}
	})

	t.Run("PartialPlanningDoesNotClaimScheduled", func(t *testing.T) {
		facade := newFacade(t)
		created, err := facade.CreateEventHappening(
			context.Background(),
			conformanceUserID,
			conformanceSpaceID,
			calendariusmodels.CreateEventHappeningRequest{
				RequestID: "create-partial",
				Spec: calendariusmodels.EventHappeningSpec{
					Title:    "Summer picnic",
					Date:     "2026-08-01",
					Location: "Phoenix Park",
				},
			},
		)
		if err != nil {
			t.Fatalf("CreateEventHappening() error = %v", err)
		}
		if created.Event.IsScheduled() {
			t.Fatalf("date-and-location-only event is scheduled: %+v", created)
		}
		if created.Event.Date != "2026-08-01" || created.Event.Location != "Phoenix Park" {
			t.Fatalf("partial planning fields were not preserved: %+v", created)
		}
	})

	t.Run("UpdateConvergesOnScheduleWithoutChangingIdentity", func(t *testing.T) {
		facade := newFacade(t)
		created, err := facade.CreateEventHappening(
			context.Background(),
			conformanceUserID,
			conformanceSpaceID,
			calendariusmodels.CreateEventHappeningRequest{
				RequestID: "create-before-update",
				Spec:      calendariusmodels.EventHappeningSpec{Title: "Summer picnic"},
			},
		)
		if err != nil {
			t.Fatalf("CreateEventHappening() error = %v", err)
		}
		updated, err := facade.UpdateEventHappening(
			context.Background(),
			conformanceUserID,
			conformanceSpaceID,
			created.Event.ID,
			calendariusmodels.UpdateEventHappeningRequest{
				RequestID:       "schedule-event",
				Date:            ptr("2026-08-01"),
				Time:            ptr("12:30"),
				Location:        ptr("Phoenix Park"),
				Description:     ptr("Bring lunch"),
				DurationMinutes: ptr(90),
			},
		)
		if err != nil {
			t.Fatalf("UpdateEventHappening() error = %v", err)
		}
		if updated.Disposition != calendariusmodels.EventHappeningChanged {
			t.Fatalf("UpdateEventHappening() disposition = %q, want changed", updated.Disposition)
		}
		if updated.Event.ID != created.Event.ID {
			t.Fatalf("UpdateEventHappening() changed identity from %q to %q", created.Event.ID, updated.Event.ID)
		}
		if !updated.Event.IsScheduled() {
			t.Fatalf("date-and-time event is not scheduled: %+v", updated)
		}
	})

	t.Run("ListIncludesPlannedAndScheduledEvents", func(t *testing.T) {
		facade := newFacade(t)
		planned, err := facade.CreateEventHappening(
			context.Background(),
			conformanceUserID,
			conformanceSpaceID,
			calendariusmodels.CreateEventHappeningRequest{
				RequestID: "create-planned",
				Spec:      calendariusmodels.EventHappeningSpec{Title: "Plan later"},
			},
		)
		if err != nil {
			t.Fatalf("create planned event: %v", err)
		}
		scheduled, err := facade.CreateEventHappening(
			context.Background(),
			conformanceUserID,
			conformanceSpaceID,
			calendariusmodels.CreateEventHappeningRequest{
				RequestID: "create-scheduled",
				Spec: calendariusmodels.EventHappeningSpec{
					Title: "Scheduled", Date: "2026-08-01", Time: "12:30",
				},
			},
		)
		if err != nil {
			t.Fatalf("create scheduled event: %v", err)
		}
		events, err := facade.ListEventHappenings(
			context.Background(), conformanceUserID, conformanceSpaceID,
		)
		if err != nil {
			t.Fatalf("ListEventHappenings() error = %v", err)
		}
		if len(events) != 2 {
			t.Fatalf("ListEventHappenings() returned %d events, want 2: %+v", len(events), events)
		}
		if events[0].ID != scheduled.Event.ID || events[1].ID != planned.Event.ID {
			t.Fatalf("ListEventHappenings() order = [%q, %q], want scheduled %q then planned %q",
				events[0].ID, events[1].ID, scheduled.Event.ID, planned.Event.ID)
		}
	})

	t.Run("CreateRetryIsReusedAndConflictingReuseFails", func(t *testing.T) {
		facade := newFacade(t)
		request := calendariusmodels.CreateEventHappeningRequest{
			RequestID: "stable-create",
			Spec:      calendariusmodels.EventHappeningSpec{Title: "Summer picnic"},
		}
		first, err := facade.CreateEventHappening(
			context.Background(), conformanceUserID, conformanceSpaceID, request,
		)
		if err != nil {
			t.Fatalf("first create: %v", err)
		}
		retry, err := facade.CreateEventHappening(
			context.Background(), conformanceUserID, conformanceSpaceID, request,
		)
		if err != nil {
			t.Fatalf("retry create: %v", err)
		}
		if retry.Disposition != calendariusmodels.EventHappeningReused ||
			retry.Event.ID != first.Event.ID {
			t.Fatalf("retry = %+v, want reused ID %q", retry, first.Event.ID)
		}
		request.Spec.Title = "Different"
		if _, err := facade.CreateEventHappening(
			context.Background(), conformanceUserID, conformanceSpaceID, request,
		); !errors.Is(err, facade4calendarius.ErrRequestIDConflict) {
			t.Fatalf("conflicting create error = %v, want ErrRequestIDConflict", err)
		}
	})

	t.Run("UpdatePatchIsIdempotentAndPreservesOmittedFields", func(t *testing.T) {
		facade := newFacade(t)
		created, err := facade.CreateEventHappening(
			context.Background(),
			conformanceUserID,
			conformanceSpaceID,
			calendariusmodels.CreateEventHappeningRequest{
				RequestID: "create-for-patch",
				Spec: calendariusmodels.EventHappeningSpec{
					Title: "Picnic", Date: "2026-08-01", Location: "Phoenix Park",
				},
			},
		)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		request := calendariusmodels.UpdateEventHappeningRequest{
			RequestID: "patch-time",
			Time:      ptr("12:30"),
		}
		first, err := facade.UpdateEventHappening(
			context.Background(), conformanceUserID, conformanceSpaceID, created.Event.ID, request,
		)
		if err != nil {
			t.Fatalf("first update: %v", err)
		}
		if first.Event.Date != "2026-08-01" || first.Event.Location != "Phoenix Park" {
			t.Fatalf("patch lost omitted fields: %+v", first.Event)
		}
		retry, err := facade.UpdateEventHappening(
			context.Background(), conformanceUserID, conformanceSpaceID, created.Event.ID, request,
		)
		if err != nil {
			t.Fatalf("retry update: %v", err)
		}
		if retry.Disposition != calendariusmodels.EventHappeningReused {
			t.Fatalf("retry disposition = %q, want reused", retry.Disposition)
		}
		request.Time = ptr("13:00")
		if _, err := facade.UpdateEventHappening(
			context.Background(), conformanceUserID, conformanceSpaceID, created.Event.ID, request,
		); !errors.Is(err, facade4calendarius.ErrRequestIDConflict) {
			t.Fatalf("conflicting update error = %v, want ErrRequestIDConflict", err)
		}
	})
}

func ptr[T any](value T) *T { return &value }

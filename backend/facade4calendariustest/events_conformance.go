// Package facade4calendariustest contains reusable conformance checks for
// public Calendarius facades. It is separate from production contract packages
// so consumers do not inherit a dependency on testing.
package facade4calendariustest

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/crediterra/money"
	"github.com/sneat-co/ext-calendarius/backend/calendariusmodels"
	"github.com/sneat-co/ext-calendarius/backend/facade4calendarius"
)

const (
	conformanceUserID             = "user1"
	conformanceSecondUserID       = "user2"
	conformanceUnauthorizedUserID = "outsider"
	conformanceSpaceID            = "space1"
	conformanceSecondSpaceID      = "space2"
	conformanceTimeZone           = "Europe/Dublin"
	conformanceUTCOffset          = "+01:00"
)

// RunEventHappeningsFacadeConformance verifies portable facade behavior only.
// Passing it is not a production-provider conformance claim because this
// callback cannot observe raw kind/type rows, durable receipts, audit counts,
// or authorization fixtures. Real DAL providers must also pass
// RunEventHappeningsProviderConformance with its storage-observing harness.
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
		if err := created.Validate(); err != nil {
			t.Fatalf("CreateEventHappening() returned invalid mutation: %v", err)
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

	t.Run("CreateProjectsInitialHappeningOwnedPrices", func(t *testing.T) {
		facade := newFacade(t)
		prices := conformancePrices()
		created, err := facade.CreateEventHappening(
			context.Background(), conformanceUserID, conformanceSpaceID,
			calendariusmodels.CreateEventHappeningRequest{
				RequestID: "create-priced",
				Spec:      calendariusmodels.EventHappeningSpec{Title: "Priced game night"},
				WithHappeningPrices: calendariusmodels.WithHappeningPrices{
					Prices: prices,
				},
			},
		)
		if err != nil {
			t.Fatalf("create priced event: %v", err)
		}
		if len(created.Event.Prices) != len(prices) || created.Event.GetPriceByID("single1-team") == nil ||
			created.Event.GetPriceByID("quarter1") == nil {
			t.Fatalf("created prices = %+v, want %+v", created.Event.Prices, prices)
		}
		got, err := facade.GetEventHappening(
			context.Background(), conformanceUserID, conformanceSpaceID, created.Event.ID,
		)
		if err != nil || got.GetPriceByID("single1") == nil || got.GetPriceByID("single1-team") == nil {
			t.Fatalf("get priced event = %+v, %v", got, err)
		}
	})

	t.Run("CreateAttachesImmutableParentThroughDerivedHierarchy", func(t *testing.T) {
		facade := newFacade(t)
		parent, err := facade.CreateEventHappening(
			context.Background(), conformanceUserID, conformanceSpaceID,
			calendariusmodels.CreateEventHappeningRequest{
				RequestID: "create-hierarchy-parent", Spec: calendariusmodels.EventHappeningSpec{Title: "Annual cup"},
				WithHappeningPrices: calendariusmodels.WithHappeningPrices{Prices: conformancePrices()},
			},
		)
		if err != nil {
			t.Fatalf("create parent: %v", err)
		}
		child, err := facade.CreateEventHappening(
			context.Background(), conformanceUserID, conformanceSpaceID,
			calendariusmodels.CreateEventHappeningRequest{
				RequestID: "create-hierarchy-child", ParentHappeningID: parent.Event.ID,
				ExpectedParentVersion: parent.Event.Version,
				Spec:                  calendariusmodels.EventHappeningSpec{Title: "2026 cup"},
				WithHappeningPrices:   calendariusmodels.WithHappeningPrices{Prices: conformancePrices()},
			},
		)
		if err != nil {
			t.Fatalf("create child: %v", err)
		}
		if child.Event.Hierarchy.ParentHappeningID != parent.Event.ID || len(child.Event.Hierarchy.ChildHappeningIDs) != 0 {
			t.Fatalf("child hierarchy = %+v", child.Event.Hierarchy)
		}
		parentAfter, err := facade.GetEventHappening(
			context.Background(), conformanceUserID, conformanceSpaceID, parent.Event.ID,
		)
		if err != nil {
			t.Fatalf("get parent after attach: %v", err)
		}
		if parentAfter.Version != parent.Event.Version+1 ||
			len(parentAfter.Hierarchy.ChildHappeningIDs) != 1 ||
			parentAfter.Hierarchy.ChildHappeningIDs[0] != child.Event.ID {
			t.Fatalf("parent after child attach = %+v", parentAfter)
		}
		if parentAfter.GetPriceByID("single1") == nil || child.Event.GetPriceByID("single1-team") == nil {
			t.Fatalf("node-local prices lost: parent=%+v child=%+v", parentAfter.Prices, child.Event.Prices)
		}
		replay, err := facade.CreateEventHappening(
			context.Background(), conformanceUserID, conformanceSpaceID,
			calendariusmodels.CreateEventHappeningRequest{
				RequestID: "create-hierarchy-child", ParentHappeningID: parent.Event.ID,
				ExpectedParentVersion: parent.Event.Version,
				Spec:                  calendariusmodels.EventHappeningSpec{Title: "2026 cup"},
				WithHappeningPrices:   calendariusmodels.WithHappeningPrices{Prices: conformancePrices()},
			},
		)
		if err != nil || replay.Disposition != calendariusmodels.EventHappeningReused || replay.Event.ID != child.Event.ID {
			t.Fatalf("child replay = %+v, %v", replay, err)
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
				ExpectedVersion: created.Event.Version,
				Date:            ptr("2026-08-01"),
				Time:            ptr("12:30"),
				TimeZone:        ptr(conformanceTimeZone),
				UTCOffset:       ptr(conformanceUTCOffset),
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

	t.Run("RecurringRootUpdateValidatesCompleteResultAtomically", func(t *testing.T) {
		facade := newFacade(t)
		created, err := facade.CreateEventHappening(
			context.Background(), conformanceUserID, conformanceSpaceID,
			calendariusmodels.CreateEventHappeningRequest{
				RequestID: "create-recurring-update-root",
				Type:      calendariusmodels.EventHappeningTypeRecurring,
				Recurrence: &calendariusmodels.EventHappeningRecurrence{
					Repeats: "yearly",
				},
				Spec: calendariusmodels.EventHappeningSpec{Title: "Annual cup"},
			},
		)
		if err != nil {
			t.Fatalf("create recurring root: %v", err)
		}
		newTitle := "Annual creators cup"
		updated, err := facade.UpdateEventHappening(
			context.Background(), conformanceUserID, conformanceSpaceID, created.Event.ID,
			calendariusmodels.UpdateEventHappeningRequest{
				RequestID: "rename-recurring-update-root", ExpectedVersion: created.Event.Version, Title: &newTitle,
			},
		)
		if err != nil {
			t.Fatalf("rename recurring root: %v", err)
		}
		if updated.Disposition != calendariusmodels.EventHappeningChanged ||
			updated.Event.Version != created.Event.Version+1 || updated.Event.Title != newTitle ||
			updated.Event.Type != calendariusmodels.EventHappeningTypeRecurring ||
			updated.Event.Recurrence == nil || updated.Event.Recurrence.Repeats != "yearly" {
			t.Fatalf("renamed recurring root = %+v", updated)
		}

		_, err = facade.UpdateEventHappening(
			context.Background(), conformanceUserID, conformanceSpaceID, created.Event.ID,
			calendariusmodels.UpdateEventHappeningRequest{
				RequestID:       "schedule-recurring-update-root",
				ExpectedVersion: updated.Event.Version,
				Date:            ptr("2027-06-01"),
				Time:            ptr("10:00"),
				TimeZone:        ptr(conformanceTimeZone),
				UTCOffset:       ptr(conformanceUTCOffset),
				DurationMinutes: ptr(60),
			},
		)
		if !errors.Is(err, facade4calendarius.ErrInvalidEventHappening) {
			t.Fatalf("schedule recurring root error = %v, want ErrInvalidEventHappening", err)
		}
		afterFailure, err := facade.GetEventHappening(
			context.Background(), conformanceUserID, conformanceSpaceID, created.Event.ID,
		)
		if err != nil {
			t.Fatalf("get recurring root after rejected patch: %v", err)
		}
		if !reflect.DeepEqual(afterFailure, updated.Event) {
			t.Fatalf("rejected recurring schedule patch changed projection: before=%+v after=%+v", updated.Event, afterFailure)
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
					TimeZone: conformanceTimeZone, UTCOffset: conformanceUTCOffset,
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
			RequestID:       "patch-time",
			ExpectedVersion: created.Event.Version,
			Time:            ptr("12:30"),
			TimeZone:        ptr(conformanceTimeZone),
			UTCOffset:       ptr(conformanceUTCOffset),
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

	t.Run("GetNonExistentEventReturnsError", func(t *testing.T) {
		facade := newFacade(t)
		_, err := facade.GetEventHappening(
			context.Background(), conformanceUserID, conformanceSpaceID, "nonexistent-id",
		)
		if err == nil {
			t.Fatal("GetEventHappening() expected error for non-existent ID, got nil")
		}
	})

	t.Run("UpdateNonExistentEventReturnsError", func(t *testing.T) {
		facade := newFacade(t)
		_, err := facade.UpdateEventHappening(
			context.Background(), conformanceUserID, conformanceSpaceID, "nonexistent-id",
			calendariusmodels.UpdateEventHappeningRequest{RequestID: "update-nonexistent", ExpectedVersion: 1},
		)
		if err == nil {
			t.Fatal("UpdateEventHappening() expected error for non-existent ID, got nil")
		}
	})

	t.Run("UpdateWithAllOptionalFieldsAndUnchangedDisposition", func(t *testing.T) {
		facade := newFacade(t)
		created, err := facade.CreateEventHappening(
			context.Background(), conformanceUserID, conformanceSpaceID,
			calendariusmodels.CreateEventHappeningRequest{
				RequestID: "create-for-full-update",
				Spec: calendariusmodels.EventHappeningSpec{
					Title:           "Full update event",
					Date:            "2026-09-01",
					Time:            "10:00",
					TimeZone:        conformanceTimeZone,
					UTCOffset:       conformanceUTCOffset,
					Location:        "City Hall",
					Description:     "A description",
					DurationMinutes: 60,
				},
			},
		)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		// Update all optional fields.
		newTitle := "Renamed event"
		newLocation := "New location"
		newDesc := "Updated description"
		newDuration := 90
		updated, err := facade.UpdateEventHappening(
			context.Background(), conformanceUserID, conformanceSpaceID, created.Event.ID,
			calendariusmodels.UpdateEventHappeningRequest{
				RequestID:       "update-all-fields",
				ExpectedVersion: created.Event.Version,
				Title:           &newTitle,
				Date:            ptr("2026-09-15"),
				Time:            ptr("14:00"),
				Location:        &newLocation,
				Description:     &newDesc,
				DurationMinutes: &newDuration,
			},
		)
		if err != nil {
			t.Fatalf("update all fields: %v", err)
		}
		if updated.Disposition != calendariusmodels.EventHappeningChanged {
			t.Fatalf("disposition = %q, want changed", updated.Disposition)
		}
		// Re-apply the same patch — must report unchanged.
		sameUpdate, err := facade.UpdateEventHappening(
			context.Background(), conformanceUserID, conformanceSpaceID, created.Event.ID,
			calendariusmodels.UpdateEventHappeningRequest{
				RequestID:       "update-same-again",
				ExpectedVersion: updated.Event.Version,
				Title:           &newTitle,
				Date:            ptr("2026-09-15"),
				Time:            ptr("14:00"),
				Location:        &newLocation,
				Description:     &newDesc,
				DurationMinutes: &newDuration,
			},
		)
		if err != nil {
			t.Fatalf("same update: %v", err)
		}
		if sameUpdate.Disposition != calendariusmodels.EventHappeningUnchanged {
			t.Fatalf("same update disposition = %q, want unchanged", sameUpdate.Disposition)
		}
	})

	t.Run("StaleExpectedVersionFailsWithoutChangingEvent", func(t *testing.T) {
		facade := newFacade(t)
		created, err := facade.CreateEventHappening(context.Background(), conformanceUserID, conformanceSpaceID,
			calendariusmodels.CreateEventHappeningRequest{RequestID: "create-versioned", Spec: calendariusmodels.EventHappeningSpec{Title: "Picnic"}})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		changedTitle := "Changed"
		if _, err = facade.UpdateEventHappening(context.Background(), conformanceUserID, conformanceSpaceID, created.Event.ID,
			calendariusmodels.UpdateEventHappeningRequest{RequestID: "update-current", ExpectedVersion: created.Event.Version, Title: &changedTitle}); err != nil {
			t.Fatalf("current update: %v", err)
		}
		staleTitle := "Stale"
		if _, err = facade.UpdateEventHappening(context.Background(), conformanceUserID, conformanceSpaceID, created.Event.ID,
			calendariusmodels.UpdateEventHappeningRequest{RequestID: "update-stale", ExpectedVersion: created.Event.Version, Title: &staleTitle}); !errors.Is(err, facade4calendarius.ErrEventHappeningVersionConflict) {
			t.Fatalf("stale update error = %v, want ErrEventHappeningVersionConflict", err)
		}
	})

	t.Run("ConcurrentCreateReplaysOneCanonicalEvent", func(t *testing.T) {
		facade := newFacade(t)
		request := calendariusmodels.CreateEventHappeningRequest{
			RequestID: "concurrent-create",
			Spec:      calendariusmodels.EventHappeningSpec{Title: "Concurrent picnic"},
		}
		const callers = 12
		results := make(chan calendariusmodels.EventHappeningMutation, callers)
		errorsCh := make(chan error, callers)
		var start sync.WaitGroup
		start.Add(1)
		var done sync.WaitGroup
		for range callers {
			done.Add(1)
			go func() {
				defer done.Done()
				start.Wait()
				result, err := facade.CreateEventHappening(context.Background(), conformanceUserID, conformanceSpaceID, request)
				if err != nil {
					errorsCh <- err
					return
				}
				results <- result
			}()
		}
		start.Done()
		done.Wait()
		close(results)
		close(errorsCh)
		for err := range errorsCh {
			t.Fatalf("concurrent create: %v", err)
		}
		var id string
		created := 0
		for result := range results {
			if id == "" {
				id = result.Event.ID
			}
			if result.Event.ID != id {
				t.Fatalf("concurrent create returned IDs %q and %q", id, result.Event.ID)
			}
			if result.Disposition == calendariusmodels.EventHappeningCreated {
				created++
			} else if result.Disposition != calendariusmodels.EventHappeningReused {
				t.Fatalf("concurrent create disposition = %q, want created or reused", result.Disposition)
			}
		}
		if created != 1 {
			t.Fatalf("concurrent create count = %d, want 1", created)
		}
	})
}

func conformancePrices() []*calendariusmodels.HappeningPrice {
	return []*calendariusmodels.HappeningPrice{
		{
			ID: "single1", Term: calendariusmodels.Term{Unit: calendariusmodels.TermUnitSingle, Length: 1},
			Amount: money.Amount{Currency: "EUR", Value: 2500}, ExpenseQuantity: 1,
		},
		{
			ID: "single1-team", Term: calendariusmodels.Term{Unit: calendariusmodels.TermUnitSingle, Length: 1},
			Amount: money.Amount{Currency: "EUR", Value: 5000}, ExpenseQuantity: 2,
		},
		{
			ID: "quarter1", Term: calendariusmodels.Term{Unit: calendariusmodels.TermUnitQuarter, Length: 1},
			Amount: money.Amount{Currency: "EUR", Value: 12000},
		},
	}
}

func ptr[T any](value T) *T { return &value }

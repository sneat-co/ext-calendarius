package facade4calendariustest

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/sneat-co/ext-calendarius/backend/calendariusmodels"
	"github.com/sneat-co/ext-calendarius/backend/facade4calendarius"
)

type referenceEventFacade struct {
	next   int
	events map[string]calendariusmodels.EventHappening
	ops    map[string]referenceOperation
}

type referenceOperation struct {
	fingerprint string
	eventID     string
}

func newReferenceEventFacade() *referenceEventFacade {
	return &referenceEventFacade{
		events: make(map[string]calendariusmodels.EventHappening),
		ops:    make(map[string]referenceOperation),
	}
}

func (f *referenceEventFacade) CreateEventHappening(
	_ context.Context,
	userID, _ string,
	request calendariusmodels.CreateEventHappeningRequest,
) (calendariusmodels.EventHappeningMutation, error) {
	fingerprint := "create:" + jsonFingerprint(request.Spec)
	if operation, ok := f.ops[request.RequestID]; ok {
		if operation.fingerprint != fingerprint {
			return calendariusmodels.EventHappeningMutation{}, facade4calendarius.ErrRequestIDConflict
		}
		return calendariusmodels.EventHappeningMutation{
			Event:       f.events[operation.eventID],
			Disposition: calendariusmodels.EventHappeningReused,
		}, nil
	}
	f.next++
	id := strconv.Itoa(f.next)
	event := eventFromSpec(id, userID, request.Spec)
	f.events[id] = event
	f.ops[request.RequestID] = referenceOperation{fingerprint: fingerprint, eventID: id}
	return calendariusmodels.EventHappeningMutation{
		Event:       event,
		Disposition: calendariusmodels.EventHappeningCreated,
	}, nil
}

func (f *referenceEventFacade) GetEventHappening(
	_ context.Context,
	_, _, happeningID string,
) (calendariusmodels.EventHappening, error) {
	event, ok := f.events[happeningID]
	if !ok {
		return calendariusmodels.EventHappening{}, fmt.Errorf("event not found")
	}
	return event, nil
}

func (f *referenceEventFacade) UpdateEventHappening(
	_ context.Context,
	_, _, happeningID string,
	request calendariusmodels.UpdateEventHappeningRequest,
) (calendariusmodels.EventHappeningMutation, error) {
	fingerprint := "update:" + happeningID + ":" + jsonFingerprint(request)
	if operation, ok := f.ops[request.RequestID]; ok {
		if operation.fingerprint != fingerprint {
			return calendariusmodels.EventHappeningMutation{}, facade4calendarius.ErrRequestIDConflict
		}
		return calendariusmodels.EventHappeningMutation{
			Event:       f.events[operation.eventID],
			Disposition: calendariusmodels.EventHappeningReused,
		}, nil
	}
	event, ok := f.events[happeningID]
	if !ok {
		return calendariusmodels.EventHappeningMutation{}, fmt.Errorf("event not found")
	}
	before := event
	if request.Title != nil {
		event.Title = *request.Title
	}
	if request.Date != nil {
		event.Date = *request.Date
	}
	if request.Time != nil {
		event.Time = *request.Time
	}
	if request.Location != nil {
		event.Location = *request.Location
	}
	if request.Description != nil {
		event.Description = *request.Description
	}
	if request.DurationMinutes != nil {
		event.DurationMinutes = *request.DurationMinutes
	}
	f.events[happeningID] = event
	f.ops[request.RequestID] = referenceOperation{fingerprint: fingerprint, eventID: happeningID}
	disposition := calendariusmodels.EventHappeningChanged
	if event == before {
		disposition = calendariusmodels.EventHappeningUnchanged
	}
	return calendariusmodels.EventHappeningMutation{Event: event, Disposition: disposition}, nil
}

func (f *referenceEventFacade) ListEventHappenings(
	_ context.Context,
	_, _ string,
) ([]calendariusmodels.EventHappening, error) {
	events := make([]calendariusmodels.EventHappening, 0, len(f.events))
	for _, event := range f.events {
		events = append(events, event)
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].IsScheduled() != events[j].IsScheduled() {
			return events[i].IsScheduled()
		}
		return events[i].ID < events[j].ID
	})
	return events, nil
}

func eventFromSpec(id, createdBy string, spec calendariusmodels.EventHappeningSpec) calendariusmodels.EventHappening {
	return calendariusmodels.EventHappening{
		ID:              id,
		Title:           spec.Title,
		Date:            spec.Date,
		Time:            spec.Time,
		Location:        spec.Location,
		Description:     spec.Description,
		DurationMinutes: spec.DurationMinutes,
		Status:          calendariusmodels.EventHappeningStatusActive,
		CreatedBy:       createdBy,
		CreatedAt:       time.Unix(1, 0).UTC(),
	}
}

func jsonFingerprint(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}

var _ facade4calendarius.EventHappeningsFacade = (*referenceEventFacade)(nil)

func TestEventHappeningsFacadeConformanceSuite(t *testing.T) {
	RunEventHappeningsFacadeConformance(t, func(t *testing.T) facade4calendarius.EventHappeningsFacade {
		t.Helper()
		return newReferenceEventFacade()
	})
}

package facade4calendariustest

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sneat-co/ext-calendarius/backend/calendariusmodels"
	"github.com/sneat-co/ext-calendarius/backend/facade4calendarius"
)

type referenceEventFacade struct {
	mu     sync.Mutex
	next   int
	events map[string]map[string]calendariusmodels.EventHappening
	links  map[string]map[string]referenceHappeningLinkage
	ops    map[referenceRequestScope]referenceOperation
}

type referenceHappeningLinkage struct {
	parentHappeningIDs []string
	childHappeningIDs  []string
	relatedIDs         []string
}

type referenceRequestScope struct {
	principalID string
	spaceID     string
	requestID   string
}

type referenceOperation struct {
	operation   calendariusmodels.EventHappeningOperation
	fingerprint string
	mutation    calendariusmodels.EventHappeningMutation
	auditCount  int
}

func newReferenceEventFacade() *referenceEventFacade {
	return &referenceEventFacade{
		events: make(map[string]map[string]calendariusmodels.EventHappening),
		links:  make(map[string]map[string]referenceHappeningLinkage),
		ops:    make(map[referenceRequestScope]referenceOperation),
	}
}

func (f *referenceEventFacade) authorize(userID, spaceID string) error {
	if (userID == conformanceUserID || userID == conformanceSecondUserID) &&
		(spaceID == conformanceSpaceID || spaceID == conformanceSecondSpaceID) {
		return nil
	}
	return facade4calendarius.ErrEventHappeningUnauthorized
}

func validateReferenceAccess(userID, spaceID string) error {
	if err := (calendariusmodels.EventHappeningAccessScope{PrincipalID: userID, SpaceID: spaceID}).Validate(); err != nil {
		return fmt.Errorf("%w: %v", facade4calendarius.ErrInvalidEventHappening, err)
	}
	return nil
}

func validateReferenceRequestScope(userID, spaceID, requestID string) error {
	if err := (calendariusmodels.EventHappeningRequestScope{
		PrincipalID: userID, SpaceID: spaceID, RequestID: requestID,
	}).Validate(); err != nil {
		return fmt.Errorf("%w: %v", facade4calendarius.ErrInvalidEventHappening, err)
	}
	return nil
}

func requestScope(userID, spaceID, requestID string) referenceRequestScope {
	return referenceRequestScope{principalID: userID, spaceID: spaceID, requestID: requestID}
}

func (f *referenceEventFacade) replayOrConflict(
	scope referenceRequestScope,
	operation calendariusmodels.EventHappeningOperation,
	fingerprint string,
) (calendariusmodels.EventHappeningMutation, bool, error) {
	receipt, ok := f.ops[scope]
	if !ok {
		return calendariusmodels.EventHappeningMutation{}, false, nil
	}
	if receipt.operation != operation || receipt.fingerprint != fingerprint {
		return calendariusmodels.EventHappeningMutation{}, true, facade4calendarius.ErrRequestIDConflict
	}
	replayed := receipt.mutation
	replayed.Disposition = calendariusmodels.EventHappeningReused
	return replayed, true, nil
}

func (f *referenceEventFacade) CreateEventHappening(
	_ context.Context,
	userID, spaceID string,
	request calendariusmodels.CreateEventHappeningRequest,
) (calendariusmodels.EventHappeningMutation, error) {
	if err := validateReferenceRequestScope(userID, spaceID, request.RequestID); err != nil {
		return calendariusmodels.EventHappeningMutation{}, err
	}
	if err := f.authorize(userID, spaceID); err != nil {
		return calendariusmodels.EventHappeningMutation{}, err
	}
	fingerprint, err := request.Fingerprint()
	if err != nil {
		return calendariusmodels.EventHappeningMutation{}, fmt.Errorf("%w: %v", facade4calendarius.ErrInvalidEventHappening, err)
	}
	scope := requestScope(userID, spaceID, request.RequestID)
	f.mu.Lock()
	defer f.mu.Unlock()
	if replay, found, err := f.replayOrConflict(scope, calendariusmodels.EventHappeningOperationCreate, fingerprint); found {
		return replay, err
	}
	if err := request.Validate(); err != nil {
		return calendariusmodels.EventHappeningMutation{}, fmt.Errorf("%w: %v", facade4calendarius.ErrInvalidEventHappening, err)
	}
	if request.ParentHappeningID != "" {
		parent, ok := f.events[spaceID][request.ParentHappeningID]
		if !ok || parent.Type != calendariusmodels.EventHappeningTypeSingle || parent.Kind != calendariusmodels.EventHappeningKindEvent {
			return calendariusmodels.EventHappeningMutation{}, facade4calendarius.ErrEventHappeningNotFound
		}
		projectedParent, err := f.projectEventLocked(spaceID, request.ParentHappeningID, parent)
		if err != nil {
			return calendariusmodels.EventHappeningMutation{}, err
		}
		if projectedParent.Status != calendariusmodels.EventHappeningStatusActive {
			return calendariusmodels.EventHappeningMutation{}, facade4calendarius.ErrEventHappeningClosed
		}
		if projectedParent.Version != request.ExpectedParentVersion {
			return calendariusmodels.EventHappeningMutation{}, facade4calendarius.ErrEventHappeningVersionConflict
		}
	}
	f.next++
	id := strconv.Itoa(f.next)
	event := eventFromSpec(id, userID, request.Spec, time.Unix(int64(f.next), 0).UTC())
	event.Prices = cloneHappeningPrices(request.Prices)
	if f.events[spaceID] == nil {
		f.events[spaceID] = make(map[string]calendariusmodels.EventHappening)
	}
	f.events[spaceID][id] = event
	f.ensureReferenceLinkage(spaceID, id)
	if request.ParentHappeningID != "" {
		childLink := f.links[spaceID][id]
		childLink.parentHappeningIDs = []string{request.ParentHappeningID}
		childLink.relatedIDs = referenceRelatedIDs(spaceID, referenceLinkIDs(childLink))
		f.links[spaceID][id] = childLink

		parentLink := f.links[spaceID][request.ParentHappeningID]
		parentLink.childHappeningIDs = append(parentLink.childHappeningIDs, id)
		sort.Strings(parentLink.childHappeningIDs)
		parentLink.relatedIDs = referenceRelatedIDs(spaceID, referenceLinkIDs(parentLink))
		f.links[spaceID][request.ParentHappeningID] = parentLink
		parent := f.events[spaceID][request.ParentHappeningID]
		parent.Version++
		f.events[spaceID][request.ParentHappeningID] = parent
	}
	projected, err := f.projectEventLocked(spaceID, id, event)
	if err != nil {
		return calendariusmodels.EventHappeningMutation{}, err
	}
	mutation := calendariusmodels.EventHappeningMutation{Event: projected, Disposition: calendariusmodels.EventHappeningCreated}
	f.ops[scope] = referenceOperation{
		operation: calendariusmodels.EventHappeningOperationCreate, fingerprint: fingerprint, mutation: mutation, auditCount: 1,
	}
	return mutation, nil
}

func (f *referenceEventFacade) GetEventHappening(
	_ context.Context,
	userID, spaceID, happeningID string,
) (calendariusmodels.EventHappening, error) {
	if err := validateReferenceAccess(userID, spaceID); err != nil {
		return calendariusmodels.EventHappening{}, err
	}
	if err := calendariusmodels.ValidateEventHappeningID(happeningID); err != nil {
		return calendariusmodels.EventHappening{}, fmt.Errorf("%w: %v", facade4calendarius.ErrInvalidEventHappening, err)
	}
	if err := f.authorize(userID, spaceID); err != nil {
		return calendariusmodels.EventHappening{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	event, ok := f.events[spaceID][happeningID]
	if !ok || event.Type != calendariusmodels.EventHappeningTypeSingle || event.Kind != calendariusmodels.EventHappeningKindEvent {
		return calendariusmodels.EventHappening{}, facade4calendarius.ErrEventHappeningNotFound
	}
	return f.projectEventLocked(spaceID, happeningID, event)
}

func (f *referenceEventFacade) UpdateEventHappening(
	_ context.Context,
	userID, spaceID, happeningID string,
	request calendariusmodels.UpdateEventHappeningRequest,
) (calendariusmodels.EventHappeningMutation, error) {
	if err := validateReferenceRequestScope(userID, spaceID, request.RequestID); err != nil {
		return calendariusmodels.EventHappeningMutation{}, err
	}
	if err := calendariusmodels.ValidateEventHappeningID(happeningID); err != nil {
		return calendariusmodels.EventHappeningMutation{}, fmt.Errorf("%w: %v", facade4calendarius.ErrInvalidEventHappening, err)
	}
	if err := f.authorize(userID, spaceID); err != nil {
		return calendariusmodels.EventHappeningMutation{}, err
	}
	fingerprint, err := request.Fingerprint(happeningID)
	if err != nil {
		return calendariusmodels.EventHappeningMutation{}, fmt.Errorf("%w: %v", facade4calendarius.ErrInvalidEventHappening, err)
	}
	scope := requestScope(userID, spaceID, request.RequestID)
	f.mu.Lock()
	defer f.mu.Unlock()
	if replay, found, err := f.replayOrConflict(scope, calendariusmodels.EventHappeningOperationUpdate, fingerprint); found {
		return replay, err
	}
	if err := request.Validate(); err != nil {
		return calendariusmodels.EventHappeningMutation{}, fmt.Errorf("%w: %v", facade4calendarius.ErrInvalidEventHappening, err)
	}
	stored, ok := f.events[spaceID][happeningID]
	if !ok || stored.Type != calendariusmodels.EventHappeningTypeSingle || stored.Kind != calendariusmodels.EventHappeningKindEvent {
		return calendariusmodels.EventHappeningMutation{}, facade4calendarius.ErrEventHappeningNotFound
	}
	event, err := f.projectEventLocked(spaceID, happeningID, stored)
	if err != nil {
		return calendariusmodels.EventHappeningMutation{}, err
	}
	if event.Status != calendariusmodels.EventHappeningStatusActive {
		return calendariusmodels.EventHappeningMutation{}, facade4calendarius.ErrEventHappeningClosed
	}
	if request.ExpectedVersion != event.Version {
		return calendariusmodels.EventHappeningMutation{}, facade4calendarius.ErrEventHappeningVersionConflict
	}
	before := event
	applyEventPatch(&event, request)
	if err := event.Spec().Validate(); err != nil {
		return calendariusmodels.EventHappeningMutation{}, fmt.Errorf("%w: %v", facade4calendarius.ErrInvalidEventHappening, err)
	}
	disposition := calendariusmodels.EventHappeningUnchanged
	if !reflect.DeepEqual(event, before) {
		event.Version++
		disposition = calendariusmodels.EventHappeningChanged
	}
	stored = event
	stored.Hierarchy = calendariusmodels.EventHappeningHierarchy{}
	f.events[spaceID][happeningID] = stored
	mutation := calendariusmodels.EventHappeningMutation{Event: event, Disposition: disposition}
	f.ops[scope] = referenceOperation{
		operation: calendariusmodels.EventHappeningOperationUpdate, fingerprint: fingerprint, mutation: mutation, auditCount: 1,
	}
	return mutation, nil
}

func applyEventPatch(event *calendariusmodels.EventHappening, request calendariusmodels.UpdateEventHappeningRequest) {
	if request.Title != nil {
		event.Title = *request.Title
	}
	if request.Date != nil {
		event.Date = *request.Date
	}
	if request.Time != nil {
		event.Time = *request.Time
	}
	if request.TimeZone != nil {
		event.TimeZone = *request.TimeZone
	}
	if request.UTCOffset != nil {
		event.UTCOffset = *request.UTCOffset
	}
	if request.EndDate != nil {
		event.EndDate = *request.EndDate
	}
	if request.EndTime != nil {
		event.EndTime = *request.EndTime
	}
	if request.EndUTCOffset != nil {
		event.EndUTCOffset = *request.EndUTCOffset
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
}

func (f *referenceEventFacade) ListEventHappenings(
	_ context.Context,
	userID, spaceID string,
) ([]calendariusmodels.EventHappening, error) {
	if err := validateReferenceAccess(userID, spaceID); err != nil {
		return nil, err
	}
	if err := f.authorize(userID, spaceID); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	events := make([]calendariusmodels.EventHappening, 0, len(f.events[spaceID]))
	for id, stored := range f.events[spaceID] {
		event := stored
		if event.Type != calendariusmodels.EventHappeningTypeSingle || event.Kind != calendariusmodels.EventHappeningKindEvent ||
			event.Status != calendariusmodels.EventHappeningStatusActive {
			continue
		}
		var err error
		if event, err = f.projectEventLocked(spaceID, id, stored); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if len(events) > calendariusmodels.EventHappeningListMax {
		return nil, facade4calendarius.ErrEventHappeningListLimitExceeded
	}
	sort.Slice(events, func(i, j int) bool { return eventHappeningLess(events[i], events[j]) })
	return events, nil
}

func (f *referenceEventFacade) ensureReferenceLinkage(spaceID, happeningID string) {
	if f.links[spaceID] == nil {
		f.links[spaceID] = make(map[string]referenceHappeningLinkage)
	}
	if _, ok := f.links[spaceID][happeningID]; !ok {
		f.links[spaceID][happeningID] = referenceHappeningLinkage{relatedIDs: []string{"-"}}
	}
}

func referenceRelatedIDs(spaceID string, happeningIDs []string) []string {
	if len(happeningIDs) == 0 {
		return []string{"-"}
	}
	result := []string{"*"}
	for _, id := range happeningIDs {
		result = append(result, fmt.Sprintf("m=calendarius&c=happenings&s=%s&i=%s", spaceID, id))
	}
	sort.Strings(result)
	return result
}

func referenceLinkIDs(link referenceHappeningLinkage) []string {
	ids := append(append([]string(nil), link.parentHappeningIDs...), link.childHappeningIDs...)
	sort.Strings(ids)
	return slices.Compact(ids)
}

func (f *referenceEventFacade) projectEventLocked(
	spaceID, happeningID string,
	event calendariusmodels.EventHappening,
) (calendariusmodels.EventHappening, error) {
	f.ensureReferenceLinkage(spaceID, happeningID)
	link := f.links[spaceID][happeningID]
	if len(link.parentHappeningIDs) > 1 {
		return calendariusmodels.EventHappening{}, facade4calendarius.ErrEventHappeningHierarchyCorrupt
	}
	parentID := ""
	if len(link.parentHappeningIDs) == 1 {
		parentID = link.parentHappeningIDs[0]
	}
	children := append([]string(nil), link.childHappeningIDs...)
	sort.Strings(children)
	event.Hierarchy = calendariusmodels.EventHappeningHierarchy{
		ParentHappeningID: parentID, ChildHappeningIDs: children,
	}
	if err := f.validateReferenceHierarchyLocked(spaceID, happeningID); err != nil {
		return calendariusmodels.EventHappening{}, err
	}
	if err := event.Validate(); err != nil {
		return calendariusmodels.EventHappening{}, fmt.Errorf("%w: %v", facade4calendarius.ErrEventHappeningCorrupt, err)
	}
	return event, nil
}

func (f *referenceEventFacade) validateReferenceHierarchyLocked(spaceID, happeningID string) error {
	seen := make(map[string]struct{})
	currentID := happeningID
	for currentID != "" {
		if _, exists := seen[currentID]; exists {
			return facade4calendarius.ErrEventHappeningHierarchyCorrupt
		}
		seen[currentID] = struct{}{}
		current, ok := f.events[spaceID][currentID]
		if !ok || current.Type != calendariusmodels.EventHappeningTypeSingle || current.Kind != calendariusmodels.EventHappeningKindEvent {
			return facade4calendarius.ErrEventHappeningHierarchyCorrupt
		}
		f.ensureReferenceLinkage(spaceID, currentID)
		link := f.links[spaceID][currentID]
		if len(link.parentHappeningIDs) > 1 {
			return facade4calendarius.ErrEventHappeningHierarchyCorrupt
		}
		for _, childID := range link.childHappeningIDs {
			child, exists := f.links[spaceID][childID]
			if !exists || !slices.Contains(child.parentHappeningIDs, currentID) || !referenceRelatedIDsContain(link.relatedIDs, childID) {
				return facade4calendarius.ErrEventHappeningHierarchyCorrupt
			}
		}
		if len(link.parentHappeningIDs) == 0 {
			currentID = ""
			continue
		}
		parentID := link.parentHappeningIDs[0]
		parent, exists := f.links[spaceID][parentID]
		if !exists || !slices.Contains(parent.childHappeningIDs, currentID) || !referenceRelatedIDsContain(link.relatedIDs, parentID) {
			return facade4calendarius.ErrEventHappeningHierarchyCorrupt
		}
		currentID = parentID
	}
	return nil
}

func referenceRelatedIDsContain(relatedIDs []string, happeningID string) bool {
	for _, relatedID := range relatedIDs {
		if strings.Contains(relatedID, "&i="+happeningID) {
			return true
		}
	}
	return false
}

func eventHappeningLess(a, b calendariusmodels.EventHappening) bool {
	if a.IsScheduled() != b.IsScheduled() {
		return a.IsScheduled()
	}
	if a.IsScheduled() {
		aStart, _ := time.Parse(time.RFC3339, a.Date+"T"+a.Time+":00"+a.UTCOffset)
		bStart, _ := time.Parse(time.RFC3339, b.Date+"T"+b.Time+":00"+b.UTCOffset)
		if !aStart.Equal(bStart) {
			return aStart.Before(bStart)
		}
	} else if !a.CreatedAt.Equal(b.CreatedAt) {
		return a.CreatedAt.Before(b.CreatedAt)
	}
	return a.ID < b.ID
}

func eventFromSpec(id, createdBy string, spec calendariusmodels.EventHappeningSpec, createdAt time.Time) calendariusmodels.EventHappening {
	return calendariusmodels.EventHappening{
		ID: id, Type: calendariusmodels.EventHappeningTypeSingle, Kind: calendariusmodels.EventHappeningKindEvent,
		Version: 1, Title: spec.Title, Date: spec.Date, Time: spec.Time, TimeZone: spec.TimeZone, UTCOffset: spec.UTCOffset,
		EndDate: spec.EndDate, EndTime: spec.EndTime, EndUTCOffset: spec.EndUTCOffset,
		Location: spec.Location, Description: spec.Description, DurationMinutes: spec.DurationMinutes,
		Status: calendariusmodels.EventHappeningStatusActive, CreatedBy: createdBy, CreatedAt: createdAt,
	}
}

func cloneHappeningPrices(prices []*calendariusmodels.HappeningPrice) []*calendariusmodels.HappeningPrice {
	if prices == nil {
		return nil
	}
	cloned := make([]*calendariusmodels.HappeningPrice, len(prices))
	for i, price := range prices {
		if price != nil {
			value := *price
			cloned[i] = &value
		}
	}
	return cloned
}

var _ facade4calendarius.EventHappeningsFacade = (*referenceEventFacade)(nil)

// This exercises the portable behavior checks against a contract reference.
// It is deliberately not evidence that a production DAL provider conforms.
func TestReferenceFacadeBehaviorChecksDoNotClaimProductionProvider(t *testing.T) {
	RunEventHappeningsFacadeConformance(t, func(t *testing.T) facade4calendarius.EventHappeningsFacade {
		t.Helper()
		return newReferenceEventFacade()
	})
}

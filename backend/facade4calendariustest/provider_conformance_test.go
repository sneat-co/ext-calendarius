package facade4calendariustest

import (
	"sort"
	"strconv"
	"testing"

	"github.com/sneat-co/ext-calendarius/backend/calendariusmodels"
	"github.com/sneat-co/ext-calendarius/backend/facade4calendarius"
)

type referenceProviderHarness struct{ facade *referenceEventFacade }

func newReferenceProviderHarness() *referenceProviderHarness {
	return &referenceProviderHarness{facade: newReferenceEventFacade()}
}

func (h *referenceProviderHarness) Facade() facade4calendarius.EventHappeningsFacade {
	return h.facade
}
func (*referenceProviderHarness) PrimaryUserID() string             { return conformanceUserID }
func (*referenceProviderHarness) SecondaryAuthorizedUserID() string { return conformanceSecondUserID }
func (*referenceProviderHarness) UnauthorizedUserID() string        { return conformanceUnauthorizedUserID }
func (*referenceProviderHarness) PrimarySpaceID() string            { return conformanceSpaceID }
func (*referenceProviderHarness) SecondarySpaceID() string          { return conformanceSecondSpaceID }

func (h *referenceProviderHarness) SeedHappening(t *testing.T, seed StoredHappeningSeed) string {
	t.Helper()
	h.facade.mu.Lock()
	defer h.facade.mu.Unlock()
	h.facade.next++
	id := seed.ID
	if id == "" {
		id = "seed-" + strconv.Itoa(h.facade.next)
	}
	if h.facade.events[seed.SpaceID] == nil {
		h.facade.events[seed.SpaceID] = make(map[string]calendariusmodels.EventHappening)
	}
	event := eventFromSpec(id, seed.CreatedBy, seed.Spec, calendariusmodels.EventHappeningType(seed.Type), seed.Recurrence, seed.CreatedAt)
	event.Prices = seed.Prices
	event.Type = calendariusmodels.EventHappeningType(seed.Type)
	event.Kind = calendariusmodels.EventHappeningKind(seed.Kind)
	event.Status = calendariusmodels.EventHappeningStatus(seed.Status)
	event.Version = seed.Version
	h.facade.events[seed.SpaceID][id] = event
	h.facade.ensureReferenceLinkage(seed.SpaceID, id)
	link := h.facade.links[seed.SpaceID][id]
	link.parentHappeningIDs = append([]string(nil), seed.LinkageParentHappeningIDs...)
	link.childHappeningIDs = append([]string(nil), seed.LinkageChildHappeningIDs...)
	if seed.RelatedIDs != nil {
		link.relatedIDs = append([]string(nil), seed.RelatedIDs...)
	} else {
		link.relatedIDs = referenceRelatedIDs(seed.SpaceID, referenceLinkIDs(link))
	}
	h.facade.links[seed.SpaceID][id] = link
	return id
}

func (h *referenceProviderHarness) Observe(t *testing.T, spaceID string) EventHappeningsProviderObservation {
	t.Helper()
	h.facade.mu.Lock()
	defer h.facade.mu.Unlock()
	observation := EventHappeningsProviderObservation{}
	for _, event := range h.facade.events[spaceID] {
		link := h.facade.links[spaceID][event.ID]
		linkage := make([]StoredHappeningLinkageObservation, 0, len(link.parentHappeningIDs)+len(link.childHappeningIDs))
		for _, parentID := range link.parentHappeningIDs {
			linkage = append(linkage, StoredHappeningLinkageObservation{
				ModuleID: "calendarius", CollectionID: "happenings", SpaceID: spaceID, ItemID: parentID,
				RolesOfItem: []string{"parent"}, RolesToItem: []string{"child"},
			})
		}
		for _, childID := range link.childHappeningIDs {
			linkage = append(linkage, StoredHappeningLinkageObservation{
				ModuleID: "calendarius", CollectionID: "happenings", SpaceID: spaceID, ItemID: childID,
				RolesOfItem: []string{"child"}, RolesToItem: []string{"parent"},
			})
		}
		observation.Happenings = append(observation.Happenings, StoredHappeningObservation{
			SpaceID: spaceID, ID: event.ID, Type: string(event.Type), Kind: string(event.Kind), Status: string(event.Status),
			Prices: event.Prices, Linkage: linkage, RelatedIDs: append([]string(nil), link.relatedIDs...),
		})
	}
	for scope, receipt := range h.facade.ops {
		if scope.spaceID != spaceID {
			continue
		}
		observation.Receipts = append(observation.Receipts, EventHappeningReceiptObservation{
			Scope: calendariusmodels.EventHappeningRequestScope{
				PrincipalID: scope.principalID, SpaceID: scope.spaceID, RequestID: scope.requestID,
			},
			Operation: receipt.operation, TargetHappeningID: receipt.mutation.Event.ID,
			Fingerprint: receipt.fingerprint, OriginalMutation: receipt.mutation, AuditCount: receipt.auditCount,
		})
	}
	sort.Slice(observation.Happenings, func(i, j int) bool { return observation.Happenings[i].ID < observation.Happenings[j].ID })
	sort.Slice(observation.Receipts, func(i, j int) bool {
		a, b := observation.Receipts[i].Scope, observation.Receipts[j].Scope
		if a.PrincipalID != b.PrincipalID {
			return a.PrincipalID < b.PrincipalID
		}
		return a.RequestID < b.RequestID
	})
	return observation
}

var _ EventHappeningsProviderHarness = (*referenceProviderHarness)(nil)

// This proves the storage-observing suite is executable. It does not claim
// conformance for a production Calendarius DAL; that test belongs downstream.
func TestReferenceHarnessExercisesProviderSuiteWithoutProductionClaim(t *testing.T) {
	RunEventHappeningsProviderConformance(t, func(t *testing.T) EventHappeningsProviderHarness {
		t.Helper()
		return newReferenceProviderHarness()
	})
}

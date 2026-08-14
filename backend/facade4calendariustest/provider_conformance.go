package facade4calendariustest

import (
	"context"
	"errors"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sneat-co/ext-calendarius/backend/calendariusmodels"
	"github.com/sneat-co/ext-calendarius/backend/facade4calendarius"
	"github.com/strongo/decimal"
)

// EventHappeningsProviderHarness exposes the observations a real persistence
// provider must make available to claim conformance. Facade-only tests cannot
// prove row kind/type, atomic receipt persistence, audit cardinality, or
// filtering of legacy/non-event rows.
type EventHappeningsProviderHarness interface {
	Facade() facade4calendarius.EventHappeningsFacade
	PrimaryUserID() string
	SecondaryAuthorizedUserID() string
	UnauthorizedUserID() string
	PrimarySpaceID() string
	SecondarySpaceID() string
	SeedHappening(t *testing.T, seed StoredHappeningSeed) string
	Observe(t *testing.T, spaceID string) EventHappeningsProviderObservation
}

// StoredHappeningSeed intentionally uses raw Type/Kind/Status strings so the
// harness can insert non-event and corrupt legacy rows that the public facade
// must exclude or reject.
type StoredHappeningSeed struct {
	SpaceID    string
	ID         string
	Type       string
	Kind       string
	Status     string
	Version    int64
	Spec       calendariusmodels.EventHappeningSpec
	Recurrence *calendariusmodels.EventHappeningRecurrence
	Prices     []*calendariusmodels.HappeningPrice
	// These fields represent raw standard Sneat Linkage roles and search
	// indexes, not persisted EventHappening hierarchy convenience fields.
	LinkageParentHappeningIDs []string
	LinkageChildHappeningIDs  []string
	RelatedIDs                []string
	CreatedBy                 string
	CreatedAt                 time.Time
}

type StoredHappeningObservation struct {
	SpaceID    string
	ID         string
	Type       string
	Kind       string
	Status     string
	Prices     []*calendariusmodels.HappeningPrice
	Linkage    []StoredHappeningLinkageObservation
	RelatedIDs []string
}

// StoredHappeningLinkageObservation is the raw standard Sneat Linkage entry
// needed to prove hierarchy is not sourced from a parallel parent/child field.
// SpaceID is the effective resolved Space (same-Space entries are stored with a
// bare ItemID); RolesOfItem and RolesToItem preserve both Linkage directions.
type StoredHappeningLinkageObservation struct {
	ModuleID     string
	CollectionID string
	SpaceID      string
	ItemID       string
	RolesOfItem  []string
	RolesToItem  []string
}

type EventHappeningReceiptObservation struct {
	Scope             calendariusmodels.EventHappeningRequestScope
	Operation         calendariusmodels.EventHappeningOperation
	TargetHappeningID string
	Fingerprint       string
	OriginalMutation  calendariusmodels.EventHappeningMutation
	AuditCount        int
}

type EventHappeningsProviderObservation struct {
	Happenings []StoredHappeningObservation
	Receipts   []EventHappeningReceiptObservation
}

// RunEventHappeningsProviderConformance is the suite a real Calendarius DAL
// implementation must run before making a provider-conformance claim. The
// ext-calendarius repository itself currently supplies only the contract and a
// reference harness; no production-provider claim is made here.
func RunEventHappeningsProviderConformance(
	t *testing.T,
	newHarness func(t *testing.T) EventHappeningsProviderHarness,
) {
	t.Helper()

	t.Run("CanonicalRowReceiptReplayAndAuditAreAtomic", func(t *testing.T) {
		h := newHarness(t)
		facade := h.Facade()
		request := calendariusmodels.CreateEventHappeningRequest{
			RequestID: "provider-canonical-create",
			Spec:      calendariusmodels.EventHappeningSpec{Title: "Planning"},
			WithHappeningPrices: calendariusmodels.WithHappeningPrices{
				Prices: conformancePrices(),
			},
		}
		created, err := facade.CreateEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(), request)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := created.Validate(); err != nil {
			t.Fatalf("invalid mutation projection: %v", err)
		}
		createFingerprint, err := request.Fingerprint()
		if err != nil {
			t.Fatalf("create fingerprint: %v", err)
		}
		observation := h.Observe(t, h.PrimarySpaceID())
		if len(observation.Happenings) != 1 {
			t.Fatalf("stored happenings = %d, want 1: %+v", len(observation.Happenings), observation.Happenings)
		}
		row := observation.Happenings[0]
		if row.ID != created.Event.ID || row.Type != string(calendariusmodels.EventHappeningTypeSingle) ||
			row.Kind != string(calendariusmodels.EventHappeningKindEvent) ||
			!reflect.DeepEqual(row.Prices, request.Prices) {
			t.Fatalf("canonical stored row = %+v, mutation = %+v", row, created)
		}
		assertOneReceipt(t, observation, calendariusmodels.EventHappeningRequestScope{
			PrincipalID: h.PrimaryUserID(), SpaceID: h.PrimarySpaceID(), RequestID: request.RequestID,
		}, calendariusmodels.EventHappeningOperationCreate, created.Event.ID, createFingerprint, created)
		changedPrice := request
		changedPrice.WithHappeningPrices = calendariusmodels.WithHappeningPrices{Prices: conformancePrices()}
		changedPrice.Prices[0].Amount.Value++
		if _, err = facade.CreateEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(), changedPrice); !errors.Is(err, facade4calendarius.ErrRequestIDConflict) {
			t.Fatalf("changed-price replay error = %v, want ErrRequestIDConflict", err)
		}

		newTitle := "Scheduled later"
		updateRequest := calendariusmodels.UpdateEventHappeningRequest{
			RequestID: "provider-canonical-update", ExpectedVersion: created.Event.Version, Title: &newTitle,
		}
		updated, err := facade.UpdateEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(), created.Event.ID,
			updateRequest)
		if err != nil {
			t.Fatalf("update: %v", err)
		}
		replay, err := facade.CreateEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(), request)
		if err != nil {
			t.Fatalf("replay create: %v", err)
		}
		if replay.Disposition != calendariusmodels.EventHappeningReused || !reflect.DeepEqual(replay.Event, created.Event) {
			t.Fatalf("replay = %+v, want original projection %+v with reused disposition", replay, created.Event)
		}
		if reflect.DeepEqual(replay.Event, updated.Event) {
			t.Fatal("replay leaked the later mutable projection instead of the durable original result")
		}
		observation = h.Observe(t, h.PrimarySpaceID())
		if len(observation.Happenings) != 1 || len(observation.Receipts) != 2 {
			t.Fatalf("post-replay observation = %+v, want one row and two operation receipts", observation)
		}
		updateFingerprint, err := updateRequest.Fingerprint(created.Event.ID)
		if err != nil {
			t.Fatalf("update fingerprint: %v", err)
		}
		assertOneReceipt(t, observation, calendariusmodels.EventHappeningRequestScope{
			PrincipalID: h.PrimaryUserID(), SpaceID: h.PrimarySpaceID(), RequestID: request.RequestID,
		}, calendariusmodels.EventHappeningOperationCreate, created.Event.ID, createFingerprint, created)
		assertOneReceipt(t, observation, calendariusmodels.EventHappeningRequestScope{
			PrincipalID: h.PrimaryUserID(), SpaceID: h.PrimarySpaceID(), RequestID: updateRequest.RequestID,
		}, calendariusmodels.EventHappeningOperationUpdate, created.Event.ID, updateFingerprint, updated)

		other, err := facade.CreateEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(),
			calendariusmodels.CreateEventHappeningRequest{
				RequestID: "provider-other-create", Spec: calendariusmodels.EventHappeningSpec{Title: "Other"},
			})
		if err != nil {
			t.Fatalf("create other target: %v", err)
		}
		crossTarget := updateRequest
		crossTarget.ExpectedVersion = other.Event.Version
		if _, err = facade.UpdateEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(), other.Event.ID, crossTarget); !errors.Is(err, facade4calendarius.ErrRequestIDConflict) {
			t.Fatalf("cross-target reuse error = %v, want ErrRequestIDConflict", err)
		}
	})

	t.Run("HierarchyIsDerivedFromReciprocalLinkageAndEachNodeOwnsPrices", func(t *testing.T) {
		h := newHarness(t)
		facade := h.Facade()
		root := createProviderHierarchyNode(t, facade, h.PrimaryUserID(), h.PrimarySpaceID(),
			"hierarchy-root", "Annual cup", "", 0, 1000, calendariusmodels.EventHappeningTypeRecurring)
		year := createProviderHierarchyNode(t, facade, h.PrimaryUserID(), h.PrimarySpaceID(),
			"hierarchy-year", "2026 cup", root.Event.ID, root.Event.Version, 2000, calendariusmodels.EventHappeningTypeSingle)
		tournament := createProviderHierarchyNode(t, facade, h.PrimaryUserID(), h.PrimarySpaceID(),
			"hierarchy-tournament", "Open tournament", year.Event.ID, year.Event.Version, 3000, calendariusmodels.EventHappeningTypeSingle)
		game := createProviderHierarchyNode(t, facade, h.PrimaryUserID(), h.PrimarySpaceID(),
			"hierarchy-game", "Final game", tournament.Event.ID, tournament.Event.Version, 4000, calendariusmodels.EventHappeningTypeSingle)

		rootAfter := mustGetProviderEvent(t, facade, h.PrimaryUserID(), h.PrimarySpaceID(), root.Event.ID)
		yearAfter := mustGetProviderEvent(t, facade, h.PrimaryUserID(), h.PrimarySpaceID(), year.Event.ID)
		tournamentAfter := mustGetProviderEvent(t, facade, h.PrimaryUserID(), h.PrimarySpaceID(), tournament.Event.ID)
		gameAfter := mustGetProviderEvent(t, facade, h.PrimaryUserID(), h.PrimarySpaceID(), game.Event.ID)
		assertProjectedHierarchy(t, rootAfter, "", []string{year.Event.ID})
		assertProjectedHierarchy(t, yearAfter, root.Event.ID, []string{tournament.Event.ID})
		assertProjectedHierarchy(t, tournamentAfter, year.Event.ID, []string{game.Event.ID})
		assertProjectedHierarchy(t, gameAfter, tournament.Event.ID, nil)
		for i, event := range []calendariusmodels.EventHappening{rootAfter, yearAfter, tournamentAfter, gameAfter} {
			want := int64((i + 1) * 1000)
			if price := event.GetPriceByID("single1"); price == nil || int64(price.Amount.Value) != want {
				t.Fatalf("node %d canonical price = %+v, want value %d", i, price, want)
			}
		}

		observation := h.Observe(t, h.PrimarySpaceID())
		assertRawReciprocalLinkage(t, observation, root.Event.ID, year.Event.ID)
		assertRawReciprocalLinkage(t, observation, year.Event.ID, tournament.Event.ID)
		assertRawReciprocalLinkage(t, observation, tournament.Event.ID, game.Event.ID)
		if len(observation.Happenings) != 4 || len(observation.Receipts) != 4 {
			t.Fatalf("hierarchy atomic state = %+v", observation)
		}
		listed, err := facade.ListEventHappenings(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID())
		if err != nil || len(listed) != 4 {
			t.Fatalf("flat hierarchy list = %+v, %v", listed, err)
		}
	})

	t.Run("ConcurrentCreateCommitsOneRowReceiptAndAudit", func(t *testing.T) {
		h := newHarness(t)
		request := calendariusmodels.CreateEventHappeningRequest{
			RequestID: "provider-concurrent-create", Spec: calendariusmodels.EventHappeningSpec{Title: "Concurrent"},
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
				result, err := h.Facade().CreateEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(), request)
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
		createdCount := 0
		var eventID string
		for result := range results {
			if eventID == "" {
				eventID = result.Event.ID
			}
			if result.Event.ID != eventID {
				t.Fatalf("concurrent IDs = %q and %q", eventID, result.Event.ID)
			}
			if result.Disposition == calendariusmodels.EventHappeningCreated {
				createdCount++
			} else if result.Disposition != calendariusmodels.EventHappeningReused {
				t.Fatalf("concurrent disposition = %q", result.Disposition)
			}
		}
		if createdCount != 1 {
			t.Fatalf("created results = %d, want 1", createdCount)
		}
		observation := h.Observe(t, h.PrimarySpaceID())
		if len(observation.Happenings) != 1 || len(observation.Receipts) != 1 || observation.Receipts[0].AuditCount != 1 {
			t.Fatalf("concurrent persisted state = %+v, want one row, receipt, and audit", observation)
		}
	})

	t.Run("ConcurrentChildCreateCommitsOneReciprocalLinkageEdge", func(t *testing.T) {
		h := newHarness(t)
		parent := createProviderHierarchyNode(t, h.Facade(), h.PrimaryUserID(), h.PrimarySpaceID(),
			"concurrent-child-parent", "Parent", "", 0, 1000, calendariusmodels.EventHappeningTypeSingle)
		request := calendariusmodels.CreateEventHappeningRequest{
			RequestID: "provider-concurrent-child", ParentHappeningID: parent.Event.ID,
			ExpectedParentVersion: parent.Event.Version,
			Spec:                  calendariusmodels.EventHappeningSpec{Title: "One child"},
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
				result, err := h.Facade().CreateEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(), request)
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
			t.Fatalf("concurrent child create: %v", err)
		}
		createdCount := 0
		childID := ""
		for result := range results {
			if childID == "" {
				childID = result.Event.ID
			}
			if result.Event.ID != childID || result.Event.Hierarchy.ParentHappeningID != parent.Event.ID {
				t.Fatalf("concurrent child result = %+v", result)
			}
			if result.Disposition == calendariusmodels.EventHappeningCreated {
				createdCount++
			} else if result.Disposition != calendariusmodels.EventHappeningReused {
				t.Fatalf("concurrent child disposition = %q", result.Disposition)
			}
		}
		if createdCount != 1 {
			t.Fatalf("created child results = %d, want 1", createdCount)
		}
		observation := h.Observe(t, h.PrimarySpaceID())
		if len(observation.Happenings) != 2 || len(observation.Receipts) != 2 {
			t.Fatalf("concurrent child persisted state = %+v", observation)
		}
		assertRawReciprocalLinkage(t, observation, parent.Event.ID, childID)
		parentAfter := mustGetProviderEvent(t, h.Facade(), h.PrimaryUserID(), h.PrimarySpaceID(), parent.Event.ID)
		if parentAfter.Version != parent.Event.Version+1 || !reflect.DeepEqual(parentAfter.Hierarchy.ChildHappeningIDs, []string{childID}) {
			t.Fatalf("parent after concurrent child = %+v", parentAfter)
		}
	})

	t.Run("ConcurrentUpdateCommitsOneVersionReceiptAndAudit", func(t *testing.T) {
		h := newHarness(t)
		created, err := h.Facade().CreateEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(),
			calendariusmodels.CreateEventHappeningRequest{
				RequestID: "concurrent-update-seed", Spec: calendariusmodels.EventHappeningSpec{Title: "Before"},
			})
		if err != nil {
			t.Fatal(err)
		}
		title := "After"
		request := calendariusmodels.UpdateEventHappeningRequest{
			RequestID: "provider-concurrent-update", ExpectedVersion: created.Event.Version, Title: &title,
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
				result, err := h.Facade().UpdateEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(), created.Event.ID, request)
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
			t.Fatalf("concurrent update: %v", err)
		}
		changedCount := 0
		for result := range results {
			if result.Event.ID != created.Event.ID || result.Event.Version != created.Event.Version+1 || result.Event.Title != title {
				t.Fatalf("concurrent update result = %+v", result)
			}
			if result.Disposition == calendariusmodels.EventHappeningChanged {
				changedCount++
			} else if result.Disposition != calendariusmodels.EventHappeningReused {
				t.Fatalf("concurrent disposition = %q", result.Disposition)
			}
		}
		if changedCount != 1 {
			t.Fatalf("changed results = %d, want 1", changedCount)
		}
		observation := h.Observe(t, h.PrimarySpaceID())
		if len(observation.Happenings) != 1 || len(observation.Receipts) != 2 {
			t.Fatalf("concurrent update persisted state = %+v", observation)
		}
		for _, receipt := range observation.Receipts {
			if receipt.AuditCount != 1 {
				t.Fatalf("receipt audit count = %d, want 1: %+v", receipt.AuditCount, receipt)
			}
		}
	})

	t.Run("IdempotencyScopeSeparatesPrincipalAndSpaceButRejectsCrossOperation", func(t *testing.T) {
		h := newHarness(t)
		facade := h.Facade()
		request := calendariusmodels.CreateEventHappeningRequest{
			RequestID: "same-request-id", Spec: calendariusmodels.EventHappeningSpec{Title: "Scoped"},
		}
		primary, err := facade.CreateEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(), request)
		if err != nil {
			t.Fatal(err)
		}
		otherUser, err := facade.CreateEventHappening(context.Background(), h.SecondaryAuthorizedUserID(), h.PrimarySpaceID(), request)
		if err != nil {
			t.Fatal(err)
		}
		otherSpace, err := facade.CreateEventHappening(context.Background(), h.PrimaryUserID(), h.SecondarySpaceID(), request)
		if err != nil {
			t.Fatal(err)
		}
		if primary.Disposition != calendariusmodels.EventHappeningCreated || otherUser.Disposition != calendariusmodels.EventHappeningCreated ||
			otherSpace.Disposition != calendariusmodels.EventHappeningCreated || primary.Event.ID == otherUser.Event.ID {
			t.Fatalf("independent scopes were not independently created: primary=%+v otherUser=%+v otherSpace=%+v", primary, otherUser, otherSpace)
		}
		changed := "Cross operation"
		_, err = facade.UpdateEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(), primary.Event.ID,
			calendariusmodels.UpdateEventHappeningRequest{
				RequestID: request.RequestID, ExpectedVersion: primary.Event.Version, Title: &changed,
			})
		if !errors.Is(err, facade4calendarius.ErrRequestIDConflict) {
			t.Fatalf("cross-operation reuse error = %v, want ErrRequestIDConflict", err)
		}
		primaryObservation := h.Observe(t, h.PrimarySpaceID())
		if len(primaryObservation.Happenings) != 2 || len(primaryObservation.Receipts) != 2 {
			t.Fatalf("primary Space state after conflict = %+v", primaryObservation)
		}
		secondaryObservation := h.Observe(t, h.SecondarySpaceID())
		if len(secondaryObservation.Happenings) != 1 || len(secondaryObservation.Receipts) != 1 {
			t.Fatalf("secondary Space state = %+v", secondaryObservation)
		}
	})

	t.Run("AuthorityIsEnforcedOnEveryFacadeBoundary", func(t *testing.T) {
		h := newHarness(t)
		created, err := h.Facade().CreateEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(),
			calendariusmodels.CreateEventHappeningRequest{
				RequestID: "authorized-seed", Spec: calendariusmodels.EventHappeningSpec{Title: "Authorized"},
			})
		if err != nil {
			t.Fatalf("authorized seed: %v", err)
		}
		before := h.Observe(t, h.PrimarySpaceID())
		if _, err = h.Facade().GetEventHappening(context.Background(), h.UnauthorizedUserID(), h.PrimarySpaceID(), created.Event.ID); !errors.Is(err, facade4calendarius.ErrEventHappeningUnauthorized) {
			t.Fatalf("unauthorized get error = %v", err)
		}
		changed := "Forbidden"
		if _, err = h.Facade().UpdateEventHappening(context.Background(), h.UnauthorizedUserID(), h.PrimarySpaceID(), created.Event.ID,
			calendariusmodels.UpdateEventHappeningRequest{
				RequestID: "unauthorized-update", ExpectedVersion: created.Event.Version, Title: &changed,
			}); !errors.Is(err, facade4calendarius.ErrEventHappeningUnauthorized) {
			t.Fatalf("unauthorized update error = %v", err)
		}
		if _, err = h.Facade().ListEventHappenings(context.Background(), h.UnauthorizedUserID(), h.PrimarySpaceID()); !errors.Is(err, facade4calendarius.ErrEventHappeningUnauthorized) {
			t.Fatalf("unauthorized list error = %v", err)
		}
		if _, err = h.Facade().CreateEventHappening(context.Background(), h.UnauthorizedUserID(), h.PrimarySpaceID(),
			calendariusmodels.CreateEventHappeningRequest{
				RequestID: "unauthorized-create", Spec: calendariusmodels.EventHappeningSpec{Title: "Forbidden"},
			}); !errors.Is(err, facade4calendarius.ErrEventHappeningUnauthorized) {
			t.Fatalf("unauthorized create error = %v", err)
		}
		after := h.Observe(t, h.PrimarySpaceID())
		if !reflect.DeepEqual(after, before) {
			t.Fatalf("unauthorized operations changed provider state: before=%+v after=%+v", before, after)
		}
	})

	t.Run("FiniteAccessAndReceiptScopesAreValidatedBeforeDALAccess", func(t *testing.T) {
		h := newHarness(t)
		before := h.Observe(t, h.PrimarySpaceID())
		for i, scope := range []struct{ userID, spaceID string }{
			{userID: "", spaceID: h.PrimarySpaceID()},
			{userID: string([]byte{0xff}), spaceID: h.PrimarySpaceID()},
			{userID: strings.Repeat("é", calendariusmodels.EventHappeningPrincipalMaxBytes/2+1), spaceID: h.PrimarySpaceID()},
			{userID: h.PrimaryUserID(), spaceID: ""},
			{userID: h.PrimaryUserID(), spaceID: string([]byte{0xff})},
			{userID: h.PrimaryUserID(), spaceID: strings.Repeat("é", calendariusmodels.EventHappeningSpaceIDMaxBytes/2+1)},
		} {
			_, err := h.Facade().CreateEventHappening(context.Background(), scope.userID, scope.spaceID,
				calendariusmodels.CreateEventHappeningRequest{
					RequestID: "invalid-scope-" + string(rune('a'+i)), Spec: calendariusmodels.EventHappeningSpec{Title: "Invalid scope"},
				})
			if !errors.Is(err, facade4calendarius.ErrInvalidEventHappening) {
				t.Fatalf("invalid scope %d error = %v, want ErrInvalidEventHappening", i, err)
			}
		}
		for _, happeningID := range []string{
			"", string([]byte{0xff}), strings.Repeat("é", calendariusmodels.EventHappeningIDMaxBytes/2+1), "id@other-space",
		} {
			if _, err := h.Facade().GetEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(), happeningID); !errors.Is(err, facade4calendarius.ErrInvalidEventHappening) {
				t.Fatalf("invalid happening ID %q error = %v", happeningID, err)
			}
		}
		for _, requestID := range []string{
			"", string([]byte{0xff}), strings.Repeat("é", calendariusmodels.EventHappeningRequestIDMaxBytes/2+1),
		} {
			if _, err := h.Facade().CreateEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(),
				calendariusmodels.CreateEventHappeningRequest{
					RequestID: requestID, Spec: calendariusmodels.EventHappeningSpec{Title: "Invalid request scope"},
				}); !errors.Is(err, facade4calendarius.ErrInvalidEventHappening) {
				t.Fatalf("invalid request ID %q error = %v", requestID, err)
			}
		}
		after := h.Observe(t, h.PrimarySpaceID())
		if !reflect.DeepEqual(after, before) {
			t.Fatalf("invalid scopes changed provider state: before=%+v after=%+v", before, after)
		}
	})

	t.Run("InvalidInputLeavesNoRowsReceiptsOrAudit", func(t *testing.T) {
		h := newHarness(t)
		facade := h.Facade()
		badEnd := scheduledSpecForConformance("Invalid", "2026-08-01", "12:30")
		badEnd.EndTime = "11:30"
		badEnd.EndUTCOffset = conformanceUTCOffset
		for i, spec := range []calendariusmodels.EventHappeningSpec{
			{},
			{Title: string([]byte{0xff})},
			{Title: strings.Repeat("é", calendariusmodels.EventHappeningTitleMaxBytes/2+1)},
			{Title: "Bad location", Location: string([]byte{0xff})},
			{Title: "Long location", Location: strings.Repeat("é", calendariusmodels.EventHappeningLocationMaxBytes/2+1)},
			{Title: "Bad description", Description: string([]byte{0xff})},
			{Title: "Long description", Description: strings.Repeat("é", calendariusmodels.EventHappeningDescriptionMaxBytes/2+1)},
			{Title: "Long duration", Date: "2026-08-01", Time: "12:30", TimeZone: conformanceTimeZone, UTCOffset: conformanceUTCOffset, DurationMinutes: calendariusmodels.EventHappeningDurationMaxMinutes + 1},
			{Title: "Bad zone", Date: "2026-08-01", Time: "12:30", TimeZone: "Mars/Olympus", UTCOffset: "+00:00"},
			badEnd,
		} {
			_, err := facade.CreateEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(),
				calendariusmodels.CreateEventHappeningRequest{RequestID: "invalid-create-" + string(rune('a'+i)), Spec: spec})
			if !errors.Is(err, facade4calendarius.ErrInvalidEventHappening) {
				t.Fatalf("invalid create %d error = %v, want ErrInvalidEventHappening", i, err)
			}
		}
		for i, prices := range [][]*calendariusmodels.HappeningPrice{
			{nil},
			{{Term: calendariusmodels.Term{Unit: calendariusmodels.TermUnitSingle, Length: 1}, Amount: conformancePrices()[0].Amount}},
			func() []*calendariusmodels.HappeningPrice {
				v := conformancePrices()
				v[1].ID = v[0].ID
				return v
			}(),
			overLimitConformancePrices(),
			{{ID: "unknown-term", Term: calendariusmodels.Term{Unit: "fortnight", Length: 1}, Amount: conformancePrices()[0].Amount}},
		} {
			_, err := facade.CreateEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(),
				calendariusmodels.CreateEventHappeningRequest{
					RequestID: "invalid-price-create-" + string(rune('a'+i)),
					Spec:      calendariusmodels.EventHappeningSpec{Title: "Invalid price"},
					WithHappeningPrices: calendariusmodels.WithHappeningPrices{
						Prices: prices,
					},
				})
			if !errors.Is(err, facade4calendarius.ErrInvalidEventHappening) {
				t.Fatalf("invalid price create %d error = %v, want ErrInvalidEventHappening", i, err)
			}
		}
		for i, request := range []calendariusmodels.CreateEventHappeningRequest{
			{RequestID: "invalid-hierarchy-a", ParentHappeningID: "parent@space2", ExpectedParentVersion: 1, Spec: calendariusmodels.EventHappeningSpec{Title: "Cross Space"}},
			{RequestID: "invalid-hierarchy-b", ExpectedParentVersion: 1, Spec: calendariusmodels.EventHappeningSpec{Title: "Missing parent"}},
			{RequestID: "invalid-hierarchy-c", ParentHappeningID: "parent", Spec: calendariusmodels.EventHappeningSpec{Title: "Missing parent version"}},
		} {
			if _, err := facade.CreateEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(), request); !errors.Is(err, facade4calendarius.ErrInvalidEventHappening) {
				t.Fatalf("invalid hierarchy create %d error = %v, want ErrInvalidEventHappening", i, err)
			}
		}
		observation := h.Observe(t, h.PrimarySpaceID())
		if len(observation.Happenings) != 0 || len(observation.Receipts) != 0 {
			t.Fatalf("failed commands persisted state: %+v", observation)
		}
	})

	t.Run("GetAndListFilterCanonicalActiveEventsAndOrderDeterministically", func(t *testing.T) {
		h := newHarness(t)
		base := time.Unix(100, 0).UTC()
		later := h.SeedHappening(t, StoredHappeningSeed{
			SpaceID: h.PrimarySpaceID(), Type: "single", Kind: "event", Status: "active", Version: 1,
			Spec: scheduledSpecForConformance("Later", "2026-08-02", "12:30"), CreatedBy: h.PrimaryUserID(), CreatedAt: base,
		})
		earlier := h.SeedHappening(t, StoredHappeningSeed{
			SpaceID: h.PrimarySpaceID(), Type: "single", Kind: "event", Status: "active", Version: 1,
			Spec: scheduledSpecForConformance("Earlier", "2026-08-01", "12:30"), CreatedBy: h.PrimaryUserID(), CreatedAt: base.Add(time.Second),
		})
		tieB := h.SeedHappening(t, StoredHappeningSeed{
			SpaceID: h.PrimarySpaceID(), ID: "tie-b", Type: "single", Kind: "event", Status: "active", Version: 1,
			Spec: scheduledSpecForConformance("Tie B", "2026-08-01", "12:30"), CreatedBy: h.PrimaryUserID(), CreatedAt: base,
		})
		tieA := h.SeedHappening(t, StoredHappeningSeed{
			SpaceID: h.PrimarySpaceID(), ID: "tie-a", Type: "single", Kind: "event", Status: "active", Version: 1,
			Spec: scheduledSpecForConformance("Tie A", "2026-08-01", "12:30"), CreatedBy: h.PrimaryUserID(), CreatedAt: base,
		})
		planned := h.SeedHappening(t, StoredHappeningSeed{
			SpaceID: h.PrimarySpaceID(), Type: "single", Kind: "event", Status: "active", Version: 1,
			Spec: calendariusmodels.EventHappeningSpec{Title: "Planned"}, CreatedBy: h.PrimaryUserID(), CreatedAt: base.Add(2 * time.Second),
		})
		plannedB := h.SeedHappening(t, StoredHappeningSeed{
			SpaceID: h.PrimarySpaceID(), ID: "planned-b", Type: "single", Kind: "event", Status: "active", Version: 1,
			Spec: calendariusmodels.EventHappeningSpec{Title: "Planned B"}, CreatedBy: h.PrimaryUserID(), CreatedAt: base.Add(3 * time.Second),
		})
		plannedA := h.SeedHappening(t, StoredHappeningSeed{
			SpaceID: h.PrimarySpaceID(), ID: "planned-a", Type: "single", Kind: "event", Status: "active", Version: 1,
			Spec: calendariusmodels.EventHappeningSpec{Title: "Planned A"}, CreatedBy: h.PrimaryUserID(), CreatedAt: base.Add(3 * time.Second),
		})
		archived := h.SeedHappening(t, StoredHappeningSeed{
			SpaceID: h.PrimarySpaceID(), Type: "single", Kind: "event", Status: "archived", Version: 1,
			Spec: calendariusmodels.EventHappeningSpec{Title: "Archived"}, CreatedBy: h.PrimaryUserID(), CreatedAt: base,
		})
		nonEvent := h.SeedHappening(t, StoredHappeningSeed{
			SpaceID: h.PrimarySpaceID(), Type: "single", Kind: "appointment", Status: "active", Version: 1,
			Spec: calendariusmodels.EventHappeningSpec{Title: "Appointment"}, CreatedBy: h.PrimaryUserID(), CreatedAt: base,
		})
		recurring := h.SeedHappening(t, StoredHappeningSeed{
			SpaceID: h.PrimarySpaceID(), Type: "recurring", Kind: "event", Status: "active", Version: 1,
			Recurrence: &calendariusmodels.EventHappeningRecurrence{Repeats: "yearly"},
			Spec:       calendariusmodels.EventHappeningSpec{Title: "Recurring"}, CreatedBy: h.PrimaryUserID(), CreatedAt: base,
		})
		canceled := h.SeedHappening(t, StoredHappeningSeed{
			SpaceID: h.PrimarySpaceID(), Type: "single", Kind: "event", Status: "canceled", Version: 1,
			Spec: calendariusmodels.EventHappeningSpec{Title: "Canceled"}, CreatedBy: h.PrimaryUserID(), CreatedAt: base,
		})
		deleted := h.SeedHappening(t, StoredHappeningSeed{
			SpaceID: h.PrimarySpaceID(), Type: "single", Kind: "event", Status: "deleted", Version: 1,
			Spec: calendariusmodels.EventHappeningSpec{Title: "Deleted"}, CreatedBy: h.PrimaryUserID(), CreatedAt: base,
		})

		listed, err := h.Facade().ListEventHappenings(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID())
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		want := []string{earlier, tieA, tieB, later, recurring, planned, plannedA, plannedB}
		if len(listed) != len(want) {
			t.Fatalf("list = %+v, want IDs %v", listed, want)
		}
		for i, id := range want {
			if listed[i].ID != id {
				t.Fatalf("list[%d].ID = %q, want %q", i, listed[i].ID, id)
			}
		}
		again, err := h.Facade().ListEventHappenings(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID())
		if err != nil || !reflect.DeepEqual(listed, again) {
			t.Fatalf("second list = %+v, %v; first = %+v", again, err, listed)
		}
		for _, id := range []string{nonEvent} {
			if _, err = h.Facade().GetEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(), id); !errors.Is(err, facade4calendarius.ErrEventHappeningNotFound) {
				t.Fatalf("GetEventHappening(%q) error = %v, want not found", id, err)
			}
		}
		if got, err := h.Facade().GetEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(), archived); err != nil || got.Status != calendariusmodels.EventHappeningStatusArchived {
			t.Fatalf("get archived = %+v, %v", got, err)
		}
		for id, status := range map[string]calendariusmodels.EventHappeningStatus{
			canceled: calendariusmodels.EventHappeningStatusCanceled,
			deleted:  calendariusmodels.EventHappeningStatusDeleted,
		} {
			if got, err := h.Facade().GetEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(), id); err != nil || got.Status != status {
				t.Fatalf("get closed %q = %+v, %v, want status %q", id, got, err, status)
			}
		}
	})

	t.Run("ProjectionReusesHappeningOwnedPriceItems", func(t *testing.T) {
		h := newHarness(t)
		prices := conformancePrices()
		id := h.SeedHappening(t, StoredHappeningSeed{
			SpaceID: h.PrimarySpaceID(), Type: "single", Kind: "event", Status: "active", Version: 1,
			Spec: calendariusmodels.EventHappeningSpec{Title: "Priced cockpit"}, Prices: prices,
			CreatedBy: h.PrimaryUserID(), CreatedAt: time.Unix(100, 0).UTC(),
		})
		got, err := h.Facade().GetEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(), id)
		if err != nil {
			t.Fatalf("get priced happening: %v", err)
		}
		if !reflect.DeepEqual(got.Prices, prices) || got.GetPriceByID("single1-team") == nil {
			t.Fatalf("projected prices = %+v, want %+v", got.Prices, prices)
		}
		listed, err := h.Facade().ListEventHappenings(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID())
		if err != nil || len(listed) != 1 || !reflect.DeepEqual(listed[0].Prices, prices) {
			t.Fatalf("listed priced happening = %+v, %v", listed, err)
		}
		observation := h.Observe(t, h.PrimarySpaceID())
		if len(observation.Happenings) != 1 || !reflect.DeepEqual(observation.Happenings[0].Prices, prices) {
			t.Fatalf("stored price observation = %+v", observation.Happenings)
		}
	})

	t.Run("CorruptCanonicalProjectionFailsClosed", func(t *testing.T) {
		h := newHarness(t)
		ids := []string{h.SeedHappening(t, StoredHappeningSeed{
			SpaceID: h.PrimarySpaceID(), Type: "single", Kind: "event", Status: "active", Version: 0,
			Spec: calendariusmodels.EventHappeningSpec{Title: "Corrupt"}, CreatedBy: h.PrimaryUserID(), CreatedAt: time.Unix(1, 0).UTC(),
		}), h.SeedHappening(t, StoredHappeningSeed{
			SpaceID: h.PrimarySpaceID(), Type: "single", Kind: "event", Status: "active", Version: 1,
			Spec: calendariusmodels.EventHappeningSpec{Title: "Redacted creator"}, CreatedAt: time.Unix(2, 0).UTC(),
		}), h.SeedHappening(t, StoredHappeningSeed{
			SpaceID: h.PrimarySpaceID(), Type: "single", Kind: "event", Status: "active", Version: 1,
			Spec: calendariusmodels.EventHappeningSpec{Title: "Corrupt price"}, CreatedBy: h.PrimaryUserID(), CreatedAt: time.Unix(3, 0).UTC(),
			Prices: []*calendariusmodels.HappeningPrice{{
				Term:   calendariusmodels.Term{Unit: calendariusmodels.TermUnitSingle, Length: 1},
				Amount: conformancePrices()[0].Amount,
			}},
		})}
		for _, id := range ids {
			if _, err := h.Facade().GetEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(), id); !errors.Is(err, facade4calendarius.ErrEventHappeningCorrupt) {
				t.Fatalf("get corrupt %q error = %v", id, err)
			}
		}
		if _, err := h.Facade().ListEventHappenings(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID()); !errors.Is(err, facade4calendarius.ErrEventHappeningCorrupt) {
			t.Fatalf("list corrupt error = %v", err)
		}
	})

	t.Run("CorruptOrCyclicRawLinkageFailsClosed", func(t *testing.T) {
		h := newHarness(t)
		base := time.Unix(10, 0).UTC()
		for _, seed := range []StoredHappeningSeed{
			{
				SpaceID: h.PrimarySpaceID(), ID: "cycle-a", Type: "single", Kind: "event", Status: "active", Version: 1,
				Spec: calendariusmodels.EventHappeningSpec{Title: "Cycle A"}, CreatedBy: h.PrimaryUserID(), CreatedAt: base,
				LinkageParentHappeningIDs: []string{"cycle-b"}, LinkageChildHappeningIDs: []string{"cycle-b"},
			},
			{
				SpaceID: h.PrimarySpaceID(), ID: "cycle-b", Type: "single", Kind: "event", Status: "active", Version: 1,
				Spec: calendariusmodels.EventHappeningSpec{Title: "Cycle B"}, CreatedBy: h.PrimaryUserID(), CreatedAt: base,
				LinkageParentHappeningIDs: []string{"cycle-a"}, LinkageChildHappeningIDs: []string{"cycle-a"},
			},
			{
				SpaceID: h.PrimarySpaceID(), ID: "parent-a", Type: "single", Kind: "event", Status: "active", Version: 1,
				Spec: calendariusmodels.EventHappeningSpec{Title: "Parent A"}, CreatedBy: h.PrimaryUserID(), CreatedAt: base,
				LinkageChildHappeningIDs: []string{"multiple-parent-child"},
			},
			{
				SpaceID: h.PrimarySpaceID(), ID: "parent-b", Type: "single", Kind: "event", Status: "active", Version: 1,
				Spec: calendariusmodels.EventHappeningSpec{Title: "Parent B"}, CreatedBy: h.PrimaryUserID(), CreatedAt: base,
				LinkageChildHappeningIDs: []string{"multiple-parent-child"},
			},
			{
				SpaceID: h.PrimarySpaceID(), ID: "multiple-parent-child", Type: "single", Kind: "event", Status: "active", Version: 1,
				Spec: calendariusmodels.EventHappeningSpec{Title: "Two parents"}, CreatedBy: h.PrimaryUserID(), CreatedAt: base,
				LinkageParentHappeningIDs: []string{"parent-a", "parent-b"},
			},
		} {
			h.SeedHappening(t, seed)
		}
		for _, id := range []string{"cycle-a", "cycle-b", "multiple-parent-child"} {
			if _, err := h.Facade().GetEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(), id); !errors.Is(err, facade4calendarius.ErrEventHappeningHierarchyCorrupt) {
				t.Fatalf("get raw-corrupt hierarchy %q error = %v", id, err)
			}
		}
		if _, err := h.Facade().ListEventHappenings(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID()); !errors.Is(err, facade4calendarius.ErrEventHappeningHierarchyCorrupt) {
			t.Fatalf("list raw-corrupt hierarchy error = %v", err)
		}
	})

	t.Run("ParentAttachmentRequiresCanonicalActiveCurrentParent", func(t *testing.T) {
		h := newHarness(t)
		base := time.Unix(20, 0).UTC()
		nonEvent := h.SeedHappening(t, StoredHappeningSeed{
			SpaceID: h.PrimarySpaceID(), Type: "single", Kind: "appointment", Status: "active", Version: 1,
			Spec: calendariusmodels.EventHappeningSpec{Title: "Not an event"}, CreatedBy: h.PrimaryUserID(), CreatedAt: base,
		})
		closed := h.SeedHappening(t, StoredHappeningSeed{
			SpaceID: h.PrimarySpaceID(), Type: "single", Kind: "event", Status: "archived", Version: 1,
			Spec: calendariusmodels.EventHappeningSpec{Title: "Closed"}, CreatedBy: h.PrimaryUserID(), CreatedAt: base,
		})
		for i, tt := range []struct {
			parentID string
			version  int64
			want     error
		}{
			{parentID: "missing", version: 1, want: facade4calendarius.ErrEventHappeningNotFound},
			{parentID: nonEvent, version: 1, want: facade4calendarius.ErrEventHappeningNotFound},
			{parentID: closed, version: 1, want: facade4calendarius.ErrEventHappeningClosed},
			{parentID: closed, version: 2, want: facade4calendarius.ErrEventHappeningClosed},
		} {
			_, err := h.Facade().CreateEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(),
				calendariusmodels.CreateEventHappeningRequest{
					RequestID: "invalid-parent-" + string(rune('a'+i)), ParentHappeningID: tt.parentID,
					ExpectedParentVersion: tt.version, Spec: calendariusmodels.EventHappeningSpec{Title: "Child"},
				})
			if !errors.Is(err, tt.want) {
				t.Fatalf("attach to parent %q error = %v, want %v", tt.parentID, err, tt.want)
			}
		}
		root := createProviderHierarchyNode(t, h.Facade(), h.PrimaryUserID(), h.PrimarySpaceID(), "current-parent", "Current", "", 0, 1000, calendariusmodels.EventHappeningTypeSingle)
		if _, err := h.Facade().CreateEventHappening(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID(),
			calendariusmodels.CreateEventHappeningRequest{
				RequestID: "stale-parent", ParentHappeningID: root.Event.ID, ExpectedParentVersion: root.Event.Version + 1,
				Spec: calendariusmodels.EventHappeningSpec{Title: "Stale child"},
			}); !errors.Is(err, facade4calendarius.ErrEventHappeningVersionConflict) {
			t.Fatalf("stale parent error = %v", err)
		}
		observation := h.Observe(t, h.PrimarySpaceID())
		if len(observation.Happenings) != 3 || len(observation.Receipts) != 1 {
			t.Fatalf("failed parent attachments changed provider state: %+v", observation)
		}
	})

	t.Run("ListFailsInsteadOfTruncatingAtFiniteBound", func(t *testing.T) {
		h := newHarness(t)
		for i := 0; i <= calendariusmodels.EventHappeningListMax; i++ {
			h.SeedHappening(t, StoredHappeningSeed{
				SpaceID: h.PrimarySpaceID(), Type: "single", Kind: "event", Status: "active", Version: 1,
				Spec: calendariusmodels.EventHappeningSpec{Title: "Bounded"}, CreatedBy: h.PrimaryUserID(),
				CreatedAt: time.Unix(int64(i+1), 0).UTC(),
			})
		}
		if _, err := h.Facade().ListEventHappenings(context.Background(), h.PrimaryUserID(), h.PrimarySpaceID()); !errors.Is(err, facade4calendarius.ErrEventHappeningListLimitExceeded) {
			t.Fatalf("over-limit list error = %v, want ErrEventHappeningListLimitExceeded", err)
		}
	})
}

func scheduledSpecForConformance(title, date, clock string) calendariusmodels.EventHappeningSpec {
	return calendariusmodels.EventHappeningSpec{
		Title: title, Date: date, Time: clock, TimeZone: conformanceTimeZone, UTCOffset: conformanceUTCOffset,
		DurationMinutes: 60,
	}
}

func createProviderHierarchyNode(
	t *testing.T,
	facade facade4calendarius.EventHappeningsFacade,
	userID, spaceID, requestID, title, parentID string,
	expectedParentVersion, priceValue int64, eventType calendariusmodels.EventHappeningType,
) calendariusmodels.EventHappeningMutation {
	t.Helper()
	prices := conformancePrices()
	prices[0].Amount.Value = decimalValue(priceValue)
	request := calendariusmodels.CreateEventHappeningRequest{
		RequestID: requestID, ParentHappeningID: parentID, ExpectedParentVersion: expectedParentVersion,
		Type: eventType,
		Spec: calendariusmodels.EventHappeningSpec{Title: title},
		WithHappeningPrices: calendariusmodels.WithHappeningPrices{
			Prices: prices,
		},
	}
	if eventType == calendariusmodels.EventHappeningTypeRecurring {
		request.Recurrence = &calendariusmodels.EventHappeningRecurrence{Repeats: "yearly"}
	}
	created, err := facade.CreateEventHappening(context.Background(), userID, spaceID, request)
	if err != nil {
		t.Fatalf("create hierarchy node %q: %v", title, err)
	}
	return created
}

func decimalValue(value int64) decimal.Decimal64p2 { return decimal.Decimal64p2(value) }

func overLimitConformancePrices() []*calendariusmodels.HappeningPrice {
	prices := make([]*calendariusmodels.HappeningPrice, calendariusmodels.HappeningPricesMax+1)
	for i := range prices {
		price := *conformancePrices()[0]
		price.ID = "bounded-price-" + strconv.Itoa(i)
		prices[i] = &price
	}
	return prices
}

func mustGetProviderEvent(
	t *testing.T,
	facade facade4calendarius.EventHappeningsFacade,
	userID, spaceID, happeningID string,
) calendariusmodels.EventHappening {
	t.Helper()
	event, err := facade.GetEventHappening(context.Background(), userID, spaceID, happeningID)
	if err != nil {
		t.Fatalf("get hierarchy node %q: %v", happeningID, err)
	}
	return event
}

func assertProjectedHierarchy(
	t *testing.T,
	event calendariusmodels.EventHappening,
	parentID string,
	childIDs []string,
) {
	t.Helper()
	if event.Hierarchy.ParentHappeningID != parentID || !reflect.DeepEqual(event.Hierarchy.ChildHappeningIDs, childIDs) {
		t.Fatalf("event %q hierarchy = %+v, want parent=%q children=%v", event.ID, event.Hierarchy, parentID, childIDs)
	}
}

func assertRawReciprocalLinkage(
	t *testing.T,
	observation EventHappeningsProviderObservation,
	parentID, childID string,
) {
	t.Helper()
	parent := findStoredHappeningObservation(t, observation, parentID)
	child := findStoredHappeningObservation(t, observation, childID)
	if parent.SpaceID == "" || parent.SpaceID != child.SpaceID ||
		!observationHasTypedLink(parent, childID, "child", "parent") ||
		!observationRelatedIDsContain(parent.RelatedIDs, parent.SpaceID, childID) ||
		!observationHasTypedLink(child, parentID, "parent", "child") ||
		!observationRelatedIDsContain(child.RelatedIDs, child.SpaceID, parentID) {
		t.Fatalf("raw Linkage is not reciprocal: parent=%+v child=%+v", parent, child)
	}
}

func observationHasTypedLink(
	row StoredHappeningObservation,
	itemID, roleOfItem, roleToItem string,
) bool {
	for _, link := range row.Linkage {
		if link.ModuleID == "calendarius" && link.CollectionID == "happenings" &&
			link.SpaceID == row.SpaceID && link.ItemID == itemID && !strings.Contains(link.ItemID, "@") &&
			slices.Contains(link.RolesOfItem, roleOfItem) && slices.Contains(link.RolesToItem, roleToItem) {
			return true
		}
	}
	return false
}

func findStoredHappeningObservation(
	t *testing.T,
	observation EventHappeningsProviderObservation,
	happeningID string,
) StoredHappeningObservation {
	t.Helper()
	for _, happening := range observation.Happenings {
		if happening.ID == happeningID {
			return happening
		}
	}
	t.Fatalf("no stored observation for Happening %q", happeningID)
	return StoredHappeningObservation{}
}

func observationRelatedIDsContain(relatedIDs []string, spaceID, happeningID string) bool {
	for _, relatedID := range relatedIDs {
		values, err := url.ParseQuery(relatedID)
		if err == nil && values.Get("m") == "calendarius" && values.Get("c") == "happenings" &&
			values.Get("s") == spaceID && values.Get("i") == happeningID {
			return true
		}
	}
	return false
}

func assertOneReceipt(
	t *testing.T,
	observation EventHappeningsProviderObservation,
	scope calendariusmodels.EventHappeningRequestScope,
	operation calendariusmodels.EventHappeningOperation,
	targetID string,
	fingerprint string,
	original calendariusmodels.EventHappeningMutation,
) {
	t.Helper()
	matches := 0
	for _, receipt := range observation.Receipts {
		if receipt.Scope != scope {
			continue
		}
		matches++
		if err := receipt.Scope.Validate(); err != nil {
			t.Fatalf("persisted receipt has invalid durable scope: %+v: %v", receipt, err)
		}
		if receipt.Operation != operation || receipt.TargetHappeningID != targetID || receipt.Fingerprint != fingerprint ||
			receipt.AuditCount != 1 || !reflect.DeepEqual(receipt.OriginalMutation, original) {
			t.Fatalf("receipt = %+v, want operation=%q target=%q one audit and original=%+v", receipt, operation, targetID, original)
		}
	}
	if matches != 1 {
		t.Fatalf("receipt matches for %+v = %d, want 1: %+v", scope, matches, observation.Receipts)
	}
}

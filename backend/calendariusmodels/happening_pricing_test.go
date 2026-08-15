package calendariusmodels

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/crediterra/money"
	"github.com/strongo/decimal"
)

func validHappeningPrices() WithHappeningPrices {
	return WithHappeningPrices{Prices: []*HappeningPrice{
		{
			ID:              "single1",
			Term:            Term{Unit: TermUnitSingle, Length: 1},
			Amount:          money.Amount{Currency: "EUR", Value: 2500},
			ExpenseQuantity: 1,
		},
		{
			// A second price for the same term is valid: stable item IDs,
			// rather than term IDs, distinguish participation prices.
			ID:              "single1-team",
			Term:            Term{Unit: TermUnitSingle, Length: 1},
			Amount:          money.Amount{Currency: "EUR", Value: 5000},
			ExpenseQuantity: 2,
		},
	}}
}

func TestWithHappeningPricesProjection(t *testing.T) {
	prices := validHappeningPrices()
	prices.Prices = append(prices.Prices, &HappeningPrice{
		ID: "quarter1", Term: Term{Unit: TermUnitQuarter, Length: 1},
		Amount: money.Amount{Currency: "EUR", Value: 12000},
	})
	if err := prices.ValidateProjection(); err != nil {
		t.Fatalf("valid projection: %v", err)
	}
	if got := prices.GetPriceByID("single1-team"); got == nil || got.ExpenseQuantity != 2 {
		t.Fatalf("GetPriceByID() = %+v", got)
	}
	if got := prices.GetPriceByID("missing"); got != nil {
		t.Fatalf("GetPriceByID(missing) = %+v", got)
	}
}

func TestWithHappeningPricesRejectsInvalidProjection(t *testing.T) {
	for _, tt := range []struct {
		name   string
		prices WithHappeningPrices
	}{
		{name: "nil item", prices: WithHappeningPrices{Prices: []*HappeningPrice{nil}}},
		{name: "missing stored ID", prices: WithHappeningPrices{Prices: []*HappeningPrice{{Term: Term{Unit: TermUnitSingle, Length: 1}, Amount: money.Amount{Currency: "EUR"}}}}},
		{name: "invalid UTF-8 ID", prices: WithHappeningPrices{Prices: []*HappeningPrice{{ID: string([]byte{0xff}), Term: Term{Unit: TermUnitSingle, Length: 1}, Amount: money.Amount{Currency: "EUR"}}}}},
		{name: "ID byte bound", prices: WithHappeningPrices{Prices: []*HappeningPrice{{ID: strings.Repeat("é", HappeningPriceIDMaxBytes/2+1), Term: Term{Unit: TermUnitSingle, Length: 1}, Amount: money.Amount{Currency: "EUR"}}}}},
		{name: "duplicate ID", prices: func() WithHappeningPrices {
			v := validHappeningPrices()
			v.Prices[1].ID = v.Prices[0].ID
			return v
		}()},
		{name: "unknown term", prices: WithHappeningPrices{Prices: []*HappeningPrice{{ID: "p1", Term: Term{Unit: "fortnight", Length: 1}, Amount: money.Amount{Currency: "EUR"}}}}},
		{name: "negative amount", prices: WithHappeningPrices{Prices: []*HappeningPrice{{ID: "p1", Term: Term{Unit: TermUnitSingle, Length: 1}, Amount: money.Amount{Currency: "EUR", Value: -1}}}}},
		{name: "unsafe amount", prices: WithHappeningPrices{Prices: []*HappeningPrice{{ID: "p1", Term: Term{Unit: TermUnitSingle, Length: 1}, Amount: money.Amount{Currency: "EUR", Value: decimal.Decimal64p2(EventHappeningMaxSafeInteger + 1)}}}}},
		{name: "negative expense quantity", prices: WithHappeningPrices{Prices: []*HappeningPrice{{ID: "p1", Term: Term{Unit: TermUnitSingle, Length: 1}, Amount: money.Amount{Currency: "EUR"}, ExpenseQuantity: -1}}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.prices.ValidateProjection(); err == nil {
				t.Fatal("ValidateProjection() error = nil")
			}
		})
	}
}

func TestWithHappeningPricesFiniteBound(t *testing.T) {
	prices := WithHappeningPrices{Prices: make([]*HappeningPrice, HappeningPricesMax+1)}
	if err := prices.ValidateProjection(); err == nil {
		t.Fatal("over-limit projection accepted")
	}
}

func TestHappeningPriceCreateValueAllowsUnassignedID(t *testing.T) {
	price := HappeningPrice{
		Term:   Term{Unit: TermUnitWeek, Length: 1},
		Amount: money.Amount{Currency: "EUR", Value: 1000},
	}
	if err := price.Validate(); err != nil {
		t.Fatalf("unassigned create-price ID: %v", err)
	}
	if got := price.Term.ID(); got != "week1" {
		t.Fatalf("Term.ID() = %q", got)
	}
}

func TestHappeningPriceAmountUsesCanonicalMinorUnitWireValue(t *testing.T) {
	encoded, err := json.Marshal(money.Amount{Currency: "EUR", Value: 123})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(encoded), `{"currency":"EUR","value":123}`; got != want {
		t.Fatalf("money amount JSON = %s, want %s", got, want)
	}
}

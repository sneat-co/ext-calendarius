package calendariusmodels

import (
	"fmt"
	"strconv"

	"github.com/crediterra/money"
)

const (
	// HappeningPriceIDMaxBytes is the UTF-8 byte limit at the extension
	// projection boundary. Persisted IDs are generated from the finite term ID
	// plus a short collision suffix and are therefore normally much shorter.
	HappeningPriceIDMaxBytes = EventHappeningIDMaxBytes
	// HappeningPricesMax keeps one Happening projection finite. It does not
	// change the generic price mutation API; providers must reject an Event
	// projection that cannot be represented safely at this boundary.
	HappeningPricesMax = 100
)

// WithHappeningPrices is the canonical, Happening-owned price collection.
// Event-facing models embed this type rather than defining Event prices.
type WithHappeningPrices struct {
	Prices []*HappeningPrice `json:"prices,omitempty"`
}

// GetPriceByID returns the canonical Happening price with the supplied stable
// item ID, or nil when it is absent.
func (v WithHappeningPrices) GetPriceByID(priceID string) *HappeningPrice {
	for _, price := range v.Prices {
		if price != nil && price.ID == priceID {
			return price
		}
	}
	return nil
}

// Validate checks the generic price value. An empty price ID remains valid for
// the existing set-prices command, where Calendarius assigns the stored ID.
// Atomic Happening create and public projections use ValidateProjection and
// require caller-assigned IDs, matching the existing Ionic create form.
func (v WithHappeningPrices) Validate() error {
	if len(v.Prices) > HappeningPricesMax {
		return fmt.Errorf("prices exceeds maximum item count %d", HappeningPricesMax)
	}
	for i, price := range v.Prices {
		if price == nil {
			return fmt.Errorf("prices[%d] must not be nil", i)
		}
		if err := price.Validate(); err != nil {
			return fmt.Errorf("prices[%d]: %w", i, err)
		}
	}
	return nil
}

// ValidateProjection additionally requires each stored price item to have a
// unique, finite ID so consumers can persist a typed reference to it.
func (v WithHappeningPrices) ValidateProjection() error {
	if err := v.Validate(); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(v.Prices))
	for i, price := range v.Prices {
		if err := validateEventHappeningText(
			fmt.Sprintf("prices[%d].id", i), price.ID, HappeningPriceIDMaxBytes, true,
		); err != nil {
			return err
		}
		if err := price.validateProjectionNumbers(); err != nil {
			return fmt.Errorf("prices[%d]: %w", i, err)
		}
		if _, exists := seen[price.ID]; exists {
			return fmt.Errorf("prices[%d].id duplicates %q", i, price.ID)
		}
		seen[price.ID] = struct{}{}
	}
	return nil
}

// validateProjectionNumbers applies the lossless JSON/TypeScript bounds used
// by the Event-Happening boundary without changing the existing generic
// set-prices validation semantics.
func (v HappeningPrice) validateProjectionNumbers() error {
	if int64(v.Term.Length) > EventHappeningMaxSafeInteger {
		return fmt.Errorf("term.length exceeds maximum safe integer %d", EventHappeningMaxSafeInteger)
	}
	if int64(v.Amount.Value) > EventHappeningMaxSafeInteger {
		return fmt.Errorf("amount.value exceeds maximum safe integer %d", EventHappeningMaxSafeInteger)
	}
	if int64(v.ExpenseQuantity) > EventHappeningMaxSafeInteger {
		return fmt.Errorf("expenseQuantity exceeds maximum safe integer %d", EventHappeningMaxSafeInteger)
	}
	return nil
}

// HappeningPrice describes one price item owned by a Happening. ID is the
// stable reference assigned by Calendarius. Multiple items may have the same
// term; Calendarius gives each a distinct ID. The item intentionally carries
// no tournament, purpose, or Event-specific association.
type HappeningPrice struct {
	ID     string       `json:"id,omitempty"`
	Term   Term         `json:"term"`
	Amount money.Amount `json:"amount"`

	// Zero means not applicable and is omitted from the wire representation.
	ExpenseQuantity int `json:"expenseQuantity,omitempty"`
}

// Validate matches the existing Happening price rules. ID may be empty only
// before creation; "*" is reserved and never a valid item ID.
func (v HappeningPrice) Validate() error {
	if v.ID == "*" {
		return fmt.Errorf("id must not be %q", v.ID)
	}
	if v.ID != "" {
		if err := validateEventHappeningText("id", v.ID, HappeningPriceIDMaxBytes, true); err != nil {
			return err
		}
	}
	if err := v.Term.Validate(); err != nil {
		return fmt.Errorf("term: %w", err)
	}
	if err := v.Amount.Validate(); err != nil {
		return fmt.Errorf("amount: %w", err)
	}
	if v.Amount.Value < 0 {
		return fmt.Errorf("amount must be positive or zero, got %s", v.Amount.String())
	}
	if v.ExpenseQuantity < 0 {
		return fmt.Errorf("expenseQuantity must be positive or zero, got %d", v.ExpenseQuantity)
	}
	return nil
}

// TermUnit is the Happening price-coverage vocabulary. It does not describe a
// Happening's recurrence cadence: a quarterly price term and a recurring
// schedule are independent canonical concepts.
type TermUnit string

const (
	TermUnitSingle  TermUnit = "single"
	TermUnitSecond  TermUnit = "second"
	TermUnitMinute  TermUnit = "minute"
	TermUnitHour    TermUnit = "hour"
	TermUnitDay     TermUnit = "day"
	TermUnitWeek    TermUnit = "week"
	TermUnitMonth   TermUnit = "month"
	TermUnitQuarter TermUnit = "quarter"
	TermUnitYear    TermUnit = "year"
)

// Term describes the duration to which one Happening price applies.
type Term struct {
	Unit   TermUnit `json:"unit"`
	Length int      `json:"length"`
}

// ID returns the existing deterministic term-derived base price ID.
func (v Term) ID() string { return string(v.Unit) + strconv.Itoa(v.Length) }

func (v Term) String() string {
	if v.Unit == TermUnitSingle {
		return "single"
	}
	if v.Length == 1 {
		return fmt.Sprintf("1 %s", v.Unit)
	}
	return fmt.Sprintf("%d %ss", v.Length, v.Unit)
}

func (v Term) Validate() error {
	switch v.Unit {
	case TermUnitSingle, TermUnitSecond, TermUnitMinute, TermUnitHour,
		TermUnitDay, TermUnitWeek, TermUnitMonth, TermUnitQuarter, TermUnitYear:
	case "":
		return fmt.Errorf("unit is required")
	default:
		return fmt.Errorf("unknown unit %q", v.Unit)
	}
	if v.Length < 1 {
		return fmt.Errorf("length must be positive, got %d", v.Length)
	}
	return nil
}

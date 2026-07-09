// Package calendariusmodels is the shared calendar vocabulary other extensions
// may depend on. It intentionally contains no storage (dbo4*) types and has no
// dependencies beyond the standard library: implementation details stay in
// calendarius/backend; this package only carries what crosses extension
// boundaries.
package calendariusmodels

import "time"

// HappeningSpec is the minimal timing/place a consumer supplies to create the
// single calendarius happening that backs its own record (an eventius event, a
// bookius booking, a school-portal lesson, ...). Generalized from the
// eventius port of the same name; grow fields only when a consumer needs them.
type HappeningSpec struct {
	Title    string
	Start    time.Time
	Location string

	// DurationMinutes is the happening's length in minutes. When zero, the
	// implementation applies its default (60m). Consumers are not required to
	// know an end time — only a start.
	DurationMinutes int
}

// HappeningBrief is the compact read model of an existing happening that may be
// embedded in or returned to other extensions. It mirrors HappeningSpec plus
// identity and cancellation state; it is not the storage schema.
type HappeningBrief struct {
	ID       string
	Title    string
	Start    time.Time
	Location string

	// DurationMinutes is 0 when the happening uses the implementation default.
	DurationMinutes int

	Canceled bool
}

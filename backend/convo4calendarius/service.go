// Package convo4calendarius defines the persistence-free Calendarius port
// used by conversational clients.
package convo4calendarius

import "context"

// Contact is the small, presentation-safe contact view used while resolving
// calendar participants. It deliberately does not expose a Contactus record.
type Contact struct {
	ID          string
	DisplayName string
}

// Event is the small, presentation-safe happening view used by a calendar
// conversation. When is already formatted by the application service.
type Event struct {
	ID            string
	Title         string
	HappeningType string
	When          string
}

// CreateEventRequest contains the fields a conversational client can create.
// Participants are resolved contacts, never database records.
type CreateEventRequest struct {
	SpaceID         string
	Title           string
	Date            string
	Time            string
	DurationMinutes int
	ParticipantIDs  []string
}

// Valid reports whether a conversational request contains the minimum data an
// implementation needs to create a timed event. It keeps controller-side
// validation on the contract while implementations retain authorization and
// persistence responsibilities.
func (request CreateEventRequest) Valid() bool {
	return request.SpaceID != "" && request.Title != "" && request.Date != "" && request.Time != "" && request.DurationMinutes > 0
}

// Service is the Calendarius application port for conversational clients.
// Implementations own authorization, contact resolution, and persistence.
type Service interface {
	ResolveContacts(ctx context.Context, spaceID string, names []string) ([]Contact, error)
	CreateEvent(ctx context.Context, request CreateEventRequest) (Event, error)
	ListEvents(ctx context.Context, spaceID string) ([]Event, error)
	DeleteEvent(ctx context.Context, spaceID, happeningID, happeningType string) error
}

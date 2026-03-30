package models

import "time"

const (
	SeatAvailable = "available"
	SeatHeld      = "held"
	SeatReserved  = "reserved"

	HoldActive  = "active"
	HoldExpired = "expired"
)

type Event struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Seat struct {
	ID      string `json:"id"`
	EventID string `json:"event_id"`
	Number  string `json:"number"`
	Status  string `json:"status"`
	HoldID  string `json:"hold_id,omitempty"`
}

type Hold struct {
	ID        string    `json:"id"`
	EventID   string    `json:"event_id"`
	SeatID    string    `json:"seat_id"`
	UserID    string    `json:"user_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Reservation struct {
	ID        string    `json:"id"`
	HoldID    string    `json:"hold_id"`
	EventID   string    `json:"event_id"`
	SeatID    string    `json:"seat_id"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}
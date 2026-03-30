package store

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"distributed-ticket-reservation/internal/models"
)

type Store struct {
	mu           sync.Mutex
	events       map[string]models.Event
	seats        map[string]models.Seat
	holds        map[string]models.Hold
	reservations map[string]models.Reservation
}

func NewStore() *Store {
	s := &Store{
		events:       make(map[string]models.Event),
		seats:        make(map[string]models.Seat),
		holds:        make(map[string]models.Hold),
		reservations: make(map[string]models.Reservation),
	}
	s.seed()
	go s.cleanupExpiredHolds()
	return s
}

func seatKey(eventID, seatID string) string {
	return eventID + ":" + seatID
}

func (s *Store) seed() {
	event := models.Event{
		ID:   "evt-1",
		Name: "Afrobeats Night Vancouver",
	}
	s.events[event.ID] = event

	for _, seatID := range []string{"A1", "A2", "A3", "A4", "A5"} {
		seat := models.Seat{
			ID:      seatID,
			EventID: event.ID,
			Number:  seatID,
			Status:  models.SeatAvailable,
		}
		s.seats[seatKey(event.ID, seatID)] = seat
	}
}

func (s *Store) ListEvents() []models.Event {
	s.mu.Lock()
	defer s.mu.Unlock()

	events := make([]models.Event, 0, len(s.events))
	for _, e := range s.events {
		events = append(events, e)
	}
	sort.Slice(events, func(i, j int) bool {
		return events[i].ID < events[j].ID
	})
	return events
}

func (s *Store) ListSeats(eventID string) []models.Seat {
	s.mu.Lock()
	defer s.mu.Unlock()

	seats := make([]models.Seat, 0)
	for _, seat := range s.seats {
		if seat.EventID == eventID {
			seats = append(seats, seat)
		}
	}
	sort.Slice(seats, func(i, j int) bool {
		return seats[i].ID < seats[j].ID
	})
	return seats
}

func (s *Store) PlaceHold(eventID, seatID, userID string, ttl time.Duration) (models.Hold, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.events[eventID]; !ok {
		return models.Hold{}, errors.New("event not found")
	}

	key := seatKey(eventID, seatID)
	seat, ok := s.seats[key]
	if !ok {
		return models.Hold{}, errors.New("seat not found")
	}

	if seat.Status != models.SeatAvailable {
		return models.Hold{}, errors.New("seat is not available")
	}

	now := time.Now()
	hold := models.Hold{
		ID:        fmt.Sprintf("hold-%d", now.UnixNano()),
		EventID:   eventID,
		SeatID:    seatID,
		UserID:    userID,
		Status:    models.HoldActive,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}

	seat.Status = models.SeatHeld
	seat.HoldID = hold.ID

	s.holds[hold.ID] = hold
	s.seats[key] = seat

	return hold, nil
}

func (s *Store) ConfirmReservation(holdID, userID string) (models.Reservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	hold, ok := s.holds[holdID]
	if !ok {
		return models.Reservation{}, errors.New("hold not found")
	}

	if hold.Status != models.HoldActive {
		return models.Reservation{}, errors.New("hold is not active")
	}

	if hold.UserID != userID {
		return models.Reservation{}, errors.New("hold does not belong to this user")
	}

	if time.Now().After(hold.ExpiresAt) {
		key := seatKey(hold.EventID, hold.SeatID)
		seat := s.seats[key]
		seat.Status = models.SeatAvailable
		seat.HoldID = ""
		s.seats[key] = seat

		hold.Status = models.HoldExpired
		s.holds[holdID] = hold
		return models.Reservation{}, errors.New("hold expired")
	}

	key := seatKey(hold.EventID, hold.SeatID)
	seat := s.seats[key]

	if seat.Status != models.SeatHeld || seat.HoldID != holdID {
		return models.Reservation{}, errors.New("seat is not held by this hold")
	}

	reservation := models.Reservation{
		ID:        fmt.Sprintf("res-%d", time.Now().UnixNano()),
		HoldID:    hold.ID,
		EventID:   hold.EventID,
		SeatID:    hold.SeatID,
		UserID:    hold.UserID,
		CreatedAt: time.Now(),
	}

	seat.Status = models.SeatReserved
	seat.HoldID = hold.ID

	s.seats[key] = seat
	s.reservations[reservation.ID] = reservation

	return reservation, nil
}

func (s *Store) cleanupExpiredHolds() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()

		for holdID, hold := range s.holds {
			if hold.Status != models.HoldActive {
				continue
			}
			if now.After(hold.ExpiresAt) {
				key := seatKey(hold.EventID, hold.SeatID)
				seat := s.seats[key]

				if seat.Status == models.SeatHeld && seat.HoldID == holdID {
					seat.Status = models.SeatAvailable
					seat.HoldID = ""
					s.seats[key] = seat
				}

				hold.Status = models.HoldExpired
				s.holds[holdID] = hold
			}
		}

		s.mu.Unlock()
	}
}
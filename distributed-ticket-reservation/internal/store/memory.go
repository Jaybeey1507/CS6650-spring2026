package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"distributed-ticket-reservation/internal/models"
)

type MemoryStore struct {
	mu           sync.Mutex
	events       map[string]models.Event
	seats        map[string]models.Seat
	holds        map[string]models.Hold
	reservations map[string]models.Reservation
}

func NewMemoryStore() *MemoryStore {
	s := &MemoryStore{
		events:       make(map[string]models.Event),
		seats:        make(map[string]models.Seat),
		holds:        make(map[string]models.Hold),
		reservations: make(map[string]models.Reservation),
	}
	s.seed()
	return s
}

func seatKey(eventID, seatID string) string {
	return eventID + ":" + seatID
}

func (s *MemoryStore) seed() {
	for _, event := range defaultEvents() {
		s.events[event.ID] = event
	}

	for i := 1; i <= 1000; i++ {
		seatID := fmt.Sprintf("A%d", i)
		seat := models.Seat{
			ID:      seatID,
			EventID: DefaultEventID,
			Number:  seatID,
			Status:  models.SeatAvailable,
		}
		s.seats[seatKey(DefaultEventID, seatID)] = seat
	}
}

func (s *MemoryStore) ListEvents(ctx context.Context) ([]models.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	events := make([]models.Event, 0, len(s.events))
	for _, e := range s.events {
		events = append(events, e)
	}
	sort.Slice(events, func(i, j int) bool {
		return events[i].ID < events[j].ID
	})
	return events, nil
}

func (s *MemoryStore) ListSeats(ctx context.Context, eventID string) ([]models.Seat, error) {
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
	return seats, nil
}

func (s *MemoryStore) PlaceHold(ctx context.Context, eventID, seatID, userID string, ttlSeconds int) (models.Hold, error) {
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
	ttl := time.Duration(ttlSeconds) * time.Second
	if ttlSeconds <= 0 {
		ttl = 2 * time.Minute
	}

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

func (s *MemoryStore) ConfirmReservation(ctx context.Context, holdID, userID string) (models.Reservation, error) {
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
	hold.Status = models.HoldConfirmed

	s.seats[key] = seat
	s.holds[hold.ID] = hold
	s.reservations[reservation.ID] = reservation

	return reservation, nil
}

func (s *MemoryStore) ReleaseExpiredHolds(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

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

	return nil
}
package store

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"distributed-ticket-reservation/internal/models"
)

func TestOnlyOneHoldCanBePlacedForSameSeat(t *testing.T) {
	st := NewMemoryStore()

	var wg sync.WaitGroup
	successes := 0
	var mu sync.Mutex

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			_, err := st.PlaceHold(context.Background(), DefaultEventID, "A1", fmt.Sprintf("user-%d", i), 120)
			if err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	if successes != 1 {
		t.Fatalf("expected exactly 1 successful hold, got %d", successes)
	}
}

func TestConfirmReservationSetsSeatReservedAndHoldConfirmed(t *testing.T) {
	st := NewMemoryStore()

	hold, err := st.PlaceHold(context.Background(), DefaultEventID, "A2", "user-1", 120)
	if err != nil {
		t.Fatalf("place hold failed: %v", err)
	}

	res, err := st.ConfirmReservation(context.Background(), hold.ID, "user-1")
	if err != nil {
		t.Fatalf("confirm reservation failed: %v", err)
	}

	if res.SeatID != "A2" {
		t.Fatalf("expected seat A2, got %s", res.SeatID)
	}

	st.mu.Lock()
	seat := st.seats[seatKey(DefaultEventID, "A2")]
	updatedHold := st.holds[hold.ID]
	st.mu.Unlock()

	if seat.Status != models.SeatReserved {
		t.Fatalf("expected seat to be reserved, got %s", seat.Status)
	}

	if updatedHold.Status != models.HoldConfirmed {
		t.Fatalf("expected hold to be confirmed, got %s", updatedHold.Status)
	}
}

func TestWrongUserCannotConfirmHold(t *testing.T) {
	st := NewMemoryStore()

	hold, err := st.PlaceHold(context.Background(), DefaultEventID, "A3", "user-1", 120)
	if err != nil {
		t.Fatalf("place hold failed: %v", err)
	}

	_, err = st.ConfirmReservation(context.Background(), hold.ID, "user-2")
	if err == nil {
		t.Fatal("expected wrong user confirmation to fail")
	}

	st.mu.Lock()
	seat := st.seats[seatKey(DefaultEventID, "A3")]
	updatedHold := st.holds[hold.ID]
	st.mu.Unlock()

	if seat.Status != models.SeatHeld {
		t.Fatalf("expected seat to remain held, got %s", seat.Status)
	}

	if updatedHold.Status != models.HoldActive {
		t.Fatalf("expected hold to remain active, got %s", updatedHold.Status)
	}
}

func TestExpiredHoldCannotBeConfirmedAndSeatBecomesAvailable(t *testing.T) {
	st := NewMemoryStore()

	hold, err := st.PlaceHold(context.Background(), DefaultEventID, "A4", "user-1", 2)
	if err != nil {
		t.Fatalf("place hold failed: %v", err)
	}

	time.Sleep(2200 * time.Millisecond)
	_ = st.ReleaseExpiredHolds(context.Background())

	_, err = st.ConfirmReservation(context.Background(), hold.ID, "user-1")
	if err == nil {
		t.Fatal("expected expired hold confirmation to fail")
	}

	st.mu.Lock()
	seat := st.seats[seatKey(DefaultEventID, "A4")]
	st.mu.Unlock()

	if seat.Status != models.SeatAvailable {
		t.Fatalf("expected seat to become available again, got %s", seat.Status)
	}
}

func TestCleanupExpiredHoldReleasesSeat(t *testing.T) {
	st := NewMemoryStore()

	hold, err := st.PlaceHold(context.Background(), DefaultEventID, "A5", "user-1", 1)
	if err != nil {
		t.Fatalf("place hold failed: %v", err)
	}

	time.Sleep(1500 * time.Millisecond)
	_ = st.ReleaseExpiredHolds(context.Background())

	st.mu.Lock()
	seat := st.seats[seatKey(DefaultEventID, "A5")]
	updatedHold := st.holds[hold.ID]
	st.mu.Unlock()

	if seat.Status != models.SeatAvailable {
		t.Fatalf("expected seat to be released after expiration, got %s", seat.Status)
	}

	if updatedHold.Status != models.HoldExpired {
		t.Fatalf("expected hold to be expired, got %s", updatedHold.Status)
	}
}

func TestReservedSeatCannotBeHeldAgain(t *testing.T) {
	st := NewMemoryStore()

	hold, err := st.PlaceHold(context.Background(), DefaultEventID, "A6", "user-1", 120)
	if err != nil {
		t.Fatalf("place hold failed: %v", err)
	}

	_, err = st.ConfirmReservation(context.Background(), hold.ID, "user-1")
	if err != nil {
		t.Fatalf("confirm reservation failed: %v", err)
	}

	_, err = st.PlaceHold(context.Background(), DefaultEventID, "A6", "user-2", 120)
	if err == nil {
		t.Fatal("expected hold on reserved seat to fail")
	}
}
package store

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestOnlyOneHoldCanBePlacedForSameSeat(t *testing.T) {
	st := NewStore()

	var wg sync.WaitGroup
	successes := 0
	var mu sync.Mutex

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			_, err := st.PlaceHold("evt-1", "A1", fmt.Sprintf("user-%d", i), 2*time.Minute)
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

func TestConfirmReservation(t *testing.T) {
	st := NewStore()

	hold, err := st.PlaceHold("evt-1", "A2", "user-1", 2*time.Minute)
	if err != nil {
		t.Fatalf("place hold failed: %v", err)
	}

	res, err := st.ConfirmReservation(hold.ID, "user-1")
	if err != nil {
		t.Fatalf("confirm reservation failed: %v", err)
	}

	if res.SeatID != "A2" {
		t.Fatalf("expected seat A2, got %s", res.SeatID)
	}
}
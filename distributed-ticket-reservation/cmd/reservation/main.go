package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"distributed-ticket-reservation/internal/store"
)

type holdRequest struct {
	EventID    string `json:"event_id"`
	SeatID     string `json:"seat_id"`
	UserID     string `json:"user_id"`
	TTLSeconds int    `json:"ttl_seconds"`
}

type confirmRequest struct {
	HoldID string `json:"hold_id"`
	UserID string `json:"user_id"`
}

func main() {
	ctx := context.Background()

	st, err := store.NewBackendFromEnv(ctx)
	if err != nil {
		log.Fatalf("failed to initialize store backend: %v", err)
	}

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			if err := st.ReleaseExpiredHolds(context.Background()); err != nil {
				log.Printf("expired hold cleanup error: %v", err)
			}
		}
	}()

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"status":  "ok",
			"service": "reservation",
		})
	})

	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		events, err := st.ListEvents(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, events)
	})

	mux.HandleFunc("/events/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) != 3 || parts[0] != "events" || parts[2] != "seats" {
			http.NotFound(w, r)
			return
		}

		seats, err := st.ListSeats(r.Context(), parts[1])
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, seats)
	})

	mux.HandleFunc("/holds", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		var req holdRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}

		if req.EventID == "" || req.SeatID == "" || req.UserID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "event_id, seat_id, and user_id are required"})
			return
		}

		hold, err := st.PlaceHold(r.Context(), req.EventID, req.SeatID, req.UserID, req.TTLSeconds)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusCreated, hold)
	})

	mux.HandleFunc("/reservations/confirm", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		var req confirmRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}

		if req.HoldID == "" || req.UserID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "hold_id and user_id are required"})
			return
		}

		reservation, err := st.ConfirmReservation(r.Context(), req.HoldID, req.UserID)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusCreated, reservation)
	})

	log.Println("reservation service listening on :8081")
	log.Fatal(http.ListenAndServe(":8081", mux))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
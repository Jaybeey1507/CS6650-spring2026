package main

import (
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
	st := store.NewStore()
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
		writeJSON(w, http.StatusOK, st.ListEvents())
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

		eventID := parts[1]
		writeJSON(w, http.StatusOK, st.ListSeats(eventID))
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

		ttl := 2 * time.Minute
		if req.TTLSeconds > 0 {
			ttl = time.Duration(req.TTLSeconds) * time.Second
		}

		hold, err := st.PlaceHold(req.EventID, req.SeatID, req.UserID, ttl)
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

		reservation, err := st.ConfirmReservation(req.HoldID, req.UserID)
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
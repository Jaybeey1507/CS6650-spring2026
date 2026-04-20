package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
)

func main() {
	target := os.Getenv("RESERVATION_URL")
	if target == "" {
		target = "http://127.0.0.1:8081"
	}

	u, err := url.Parse(target)
	if err != nil {
		log.Fatalf("invalid RESERVATION_URL: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(u)
	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","service":"gateway"}`))
	})

	mux.Handle("/", proxy)

	log.Println("gateway listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
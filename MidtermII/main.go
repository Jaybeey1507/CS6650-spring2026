package main

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

type Product struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Brand       string `json:"brand"`
}

var (
	products sync.Map // key: int ID, value: Product
)

/*
CircuitBreaker (simple + demo-friendly)
- closed: allow calls
- open: block calls until cooldown, then close and try again
*/
type CircuitBreaker struct {
	failureCount int32
	state        int32 // 0=closed, 1=open
	openedAt     int64 // UnixNano
	threshold    int32
	cooldown     time.Duration
}

func NewCircuitBreaker(threshold int32, cooldown time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold: threshold,
		cooldown:  cooldown,
	}
}

func (cb *CircuitBreaker) Allow() bool {
	// If closed, allow.
	if atomic.LoadInt32(&cb.state) == 0 {
		return true
	}

	// If open, check if cooldown elapsed.
	openedAt := time.Unix(0, atomic.LoadInt64(&cb.openedAt))
	if time.Since(openedAt) >= cb.cooldown {
		// Simple "half-open" behavior for demo: close and reset, then allow one try.
		atomic.StoreInt32(&cb.state, 0)
		atomic.StoreInt32(&cb.failureCount, 0)
		return true
	}

	return false
}

func (cb *CircuitBreaker) OnSuccess() {
	atomic.StoreInt32(&cb.failureCount, 0)
	// keep closed
}

func (cb *CircuitBreaker) OnFailure() {
	fc := atomic.AddInt32(&cb.failureCount, 1)
	if fc >= cb.threshold {
		atomic.StoreInt32(&cb.state, 1) // open
		atomic.StoreInt64(&cb.openedAt, time.Now().UnixNano())
	}
}

func (cb *CircuitBreaker) State() string {
	if atomic.LoadInt32(&cb.state) == 1 {
		return "open"
	}
	return "closed"
}

/*
flakyRecommendationService simulates a downstream dependency.
- Sometimes fails (error)
- Sometimes slow (sleep 1–4s)
- chaos=true makes it more dramatic for the demo
*/
func flakyRecommendationService(ctx context.Context, chaos bool) ([]int, error) {
	failRate := 0.15
	slowRate := 0.25
	if chaos {
		failRate = 0.40
		slowRate = 0.50
	}

	r := rand.Float64()

	// Simulate failure
	if r < failRate {
		return nil, fmt.Errorf("downstream 500 error")
	}

	// Simulate slowness (1–4 seconds)
	if r < failRate+slowRate {
		delay := time.Duration(1000+rand.Intn(3000)) * time.Millisecond
		select {
		case <-time.After(delay):
			// finished sleeping
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	} else {
		// Normal quick response
		select {
		case <-time.After(30 * time.Millisecond):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Fake recommended product IDs
	return []int{1, 2, 3, 4, 5}, nil
}

func main() {
	// Generate 100,000 products at startup
	generateProducts(100000)

	rand.Seed(time.Now().UnixNano())

	// Circuit breaker: 5 failures -> open for 5 seconds
	cb := NewCircuitBreaker(5, 5*time.Second)

	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// /products/search?q={query}&mode=chaos
	// mode=chaos increases downstream failure/slow rates for a more dramatic demo
	r.GET("/products/search", func(c *gin.Context) {
		start := time.Now()
		q := strings.TrimSpace(c.Query("q"))
		qLower := strings.ToLower(q)

		chaos := strings.ToLower(strings.TrimSpace(c.Query("mode"))) == "chaos"

		// We must check exactly 100 products per request.
		const toCheck = 100
		const maxResults = 20

		checked := 0
		totalFound := 0
		results := make([]Product, 0, maxResults)

		// Deterministic iteration approach:
		// We "simulate" picking which 100 we check by using a simple rolling window
		// based on current time (helps variety under load, still bounded).
		seed := int(time.Now().UnixNano() % 100000)
		startID := seed + 1

		for i := 0; i < toCheck; i++ {
			id := startID + i
			if id > 100000 {
				id = id - 100000
			}

			val, ok := products.Load(id)
			checked++ // IMPORTANT: increment for every product checked
			if !ok {
				continue
			}

			p := val.(Product)
			if qLower == "" {
				// Empty query: treat as no matches (keeps behavior stable)
				continue
			}

			// Case-insensitive match on name OR category
			if strings.Contains(strings.ToLower(p.Name), qLower) ||
				strings.Contains(strings.ToLower(p.Category), qLower) {
				totalFound++
				if len(results) < maxResults {
					results = append(results, p)
				}
			}
		}

		// Fail Fast + Circuit Breaker:
		// - If breaker is OPEN: skip downstream and return fallback immediately
		// - If breaker is CLOSED: call downstream with a short timeout
		recs := []int{}
		recStatus := "skipped"

		if cb.Allow() {
			// FAIL FAST: timeout for downstream call (adjust 150-500ms for your demo)
			ctx, cancel := context.WithTimeout(c.Request.Context(), 250*time.Millisecond)
			defer cancel()
			ids, err := flakyRecommendationService(ctx, chaos)
			if err != nil {
				cb.OnFailure()
				// If ctx deadline exceeded -> fail fast. If other error -> also counts.
				recStatus = "failed_fast"
			} else {
				cb.OnSuccess()
				recs = ids
				recStatus = "ok"
			}
		} else {
			// CIRCUIT OPEN: fallback response
			recStatus = "circuit_open_fallback"
		}

		c.JSON(http.StatusOK, gin.H{
			"products":     results,
			"total_found":  totalFound,
			"checked":      checked, // helps you verify exactly 100
			"search_time":  time.Since(start).String(),
			"query":        q,
			"fixed_checks": toCheck,

			// New fields to make your demo obvious + screenshot-friendly
			"recommendations": recs,
			"rec_status":      recStatus,
			"cb_state":        cb.State(),
			"chaos_mode":      chaos,
		})
	})

	// Local run: http://localhost:8080
	// Example:
	//   /products/search?q=book
	//   /products/search?q=book&mode=chaos
	r.Run(":8080")
}

func generateProducts(n int) {
	brands := []string{"Alpha", "Bravo", "Charlie", "Delta", "Echo", "Nova", "Zen", "Apex"}
	categories := []string{"Electronics", "Books", "Home", "Clothing", "Sports", "Toys", "Beauty", "Grocery"}
	descs := []string{
		"High quality and reliable.",
		"Budget-friendly and durable.",
		"Designed for everyday use.",
		"Popular choice with great reviews.",
		"Compact, lightweight, and efficient.",
	}

	for i := 1; i <= n; i++ {
		brand := brands[i%len(brands)]
		category := categories[i%len(categories)]
		desc := descs[i%len(descs)]

		p := Product{
			ID:          i,
			Name:        "Product " + brand + " " + strconv.Itoa(i),
			Category:    category,
			Description: desc,
			Brand:       brand,
		}

		products.Store(i, p)
	}
}
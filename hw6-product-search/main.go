package main

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
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

func main() {
	// Generate 100,000 products at startup
	generateProducts(100000)

	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// /products/search?q={query}
	r.GET("/products/search", func(c *gin.Context) {
		start := time.Now()
		q := strings.TrimSpace(c.Query("q"))
		qLower := strings.ToLower(q)

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

		c.JSON(http.StatusOK, gin.H{
			"products":     results,
			"total_found":  totalFound,
			"checked":      checked, // helps you verify exactly 100
			"search_time":  time.Since(start).String(),
			"query":        q,
			"fixed_checks": toCheck,
		})
	})

	// ECS will map to 8080
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
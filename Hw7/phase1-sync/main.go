package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Item struct {
	ProductID int `json:"product_id"`
	Quantity  int `json:"quantity"`
}

type Order struct {
	OrderID    string    `json:"order_id"`
	CustomerID int       `json:"customer_id"`
	Status     string    `json:"status"`
	Items      []Item    `json:"items"`
	CreatedAt  time.Time `json:"created_at"`
}

var paymentSlots = make(chan struct{}, 1)

func main() {
	r := gin.Default()

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.POST("/orders/sync", func(c *gin.Context) {
		var req struct {
			CustomerID int    `json:"customer_id"`
			Items      []Item `json:"items"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		order := Order{
			OrderID:    uuid.New().String(),
			CustomerID: req.CustomerID,
			Status:     "pending",
			Items:      req.Items,
			CreatedAt:  time.Now(),
		}

		order.Status = "processing"

		paymentSlots <- struct{}{}
		time.Sleep(3 * time.Second)
		<-paymentSlots

		order.Status = "completed"

		c.JSON(http.StatusOK, gin.H{
			"message": "order processed successfully",
			"order":   order,
		})
	})

	r.Run(":8080")
}
package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sns"
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

	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(err)
	}

	snsClient := sns.NewFromConfig(cfg)
	topicArn := os.Getenv("SNS_TOPIC_ARN")
	if topicArn == "" {
		panic("SNS_TOPIC_ARN is required")
	}

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

	r.POST("/orders/async", func(c *gin.Context) {
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

		body, err := json.Marshal(order)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to serialize order"})
			return
		}

		_, err = snsClient.Publish(context.Background(), &sns.PublishInput{
			TopicArn: aws.String(topicArn),
			Message:  aws.String(string(body)),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to publish order"})
			return
		}

		c.JSON(http.StatusAccepted, gin.H{
			"message": "order accepted for asynchronous processing",
			"order":   order,
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	r.Run(":" + port)
}
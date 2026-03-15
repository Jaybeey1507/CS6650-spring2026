package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
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

func handler(ctx context.Context, event events.SNSEvent) error {
	for _, record := range event.Records {
		var order Order
		if err := json.Unmarshal([]byte(record.SNS.Message), &order); err != nil {
			log.Printf("failed to parse order: %v", err)
			continue
		}

		log.Printf("processing order %s for customer %d", order.OrderID, order.CustomerID)
		time.Sleep(3 * time.Second)
		log.Printf("completed order %s", order.OrderID)
	}
	return nil
}

func main() {
	lambda.Start(handler)
}
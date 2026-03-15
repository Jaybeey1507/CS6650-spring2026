package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
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

type SNSMessage struct {
	Type      string `json:"Type"`
	Message   string `json:"Message"`
	MessageID string `json:"MessageId"`
}

var paymentSlots = make(chan struct{}, 1)

func main() {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(err)
	}

	sqsClient := sqs.NewFromConfig(cfg)
	queueURL := os.Getenv("SQS_QUEUE_URL")
	if queueURL == "" {
		panic("SQS_QUEUE_URL is required")
	}

	log.Println("order processor started")

	for {
		out, err := sqsClient.ReceiveMessage(context.Background(), &sqs.ReceiveMessageInput{
			QueueUrl:            &queueURL,
			MaxNumberOfMessages: 10,
			WaitTimeSeconds:     20,
		})
		if err != nil {
			log.Printf("receive error: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		for _, msg := range out.Messages {
			go processMessage(sqsClient, queueURL, msg.Body, *msg.ReceiptHandle)
		}
	}
}

func processMessage(sqsClient *sqs.Client, queueURL string, body *string, receiptHandle string) {
	if body == nil {
		return
	}

	var envelope SNSMessage
	if err := json.Unmarshal([]byte(*body), &envelope); err != nil {
		log.Printf("failed to parse SNS envelope: %v", err)
		return
	}

	var order Order
	if err := json.Unmarshal([]byte(envelope.Message), &order); err != nil {
		log.Printf("failed to parse order: %v", err)
		return
	}

	order.Status = "processing"
	log.Printf("processing order %s for customer %d", order.OrderID, order.CustomerID)

	paymentSlots <- struct{}{}
	time.Sleep(3 * time.Second)
	<-paymentSlots

	order.Status = "completed"
	log.Printf("completed order %s", order.OrderID)

	_, err := sqsClient.DeleteMessage(context.Background(), &sqs.DeleteMessageInput{
		QueueUrl:      &queueURL,
		ReceiptHandle: &receiptHandle,
	})
	if err != nil {
		log.Printf("delete error for order %s: %v", order.OrderID, err)
	}
}
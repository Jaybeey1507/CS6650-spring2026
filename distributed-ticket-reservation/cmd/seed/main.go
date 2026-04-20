package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"distributed-ticket-reservation/internal/models"
	"distributed-ticket-reservation/internal/store"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

type seatItem struct {
	EventID string `dynamodbav:"EventID"`
	SeatID  string `dynamodbav:"SeatID"`
	Number  string `dynamodbav:"Number"`
	Status  string `dynamodbav:"Status"`
}

func main() {
	ctx := context.Background()

	seatsTable := os.Getenv("DDB_SEATS_TABLE")
	if seatsTable == "" {
		log.Fatal("missing DDB_SEATS_TABLE")
	}

	seatCount := 1000
	if v := os.Getenv("SEAT_COUNT"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			log.Fatalf("invalid SEAT_COUNT: %v", err)
		}
		seatCount = parsed
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("load aws config: %v", err)
	}

	client := dynamodb.NewFromConfig(cfg)

	for i := 1; i <= seatCount; i++ {
		seatID := fmt.Sprintf("A%d", i)

		item, err := attributevalue.MarshalMap(seatItem{
			EventID: store.DefaultEventID,
			SeatID:  seatID,
			Number:  seatID,
			Status:  models.SeatAvailable,
		})
		if err != nil {
			log.Fatalf("marshal seat %s: %v", seatID, err)
		}

		_, err = client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: &seatsTable,
			Item:      item,
		})
		if err != nil {
			log.Fatalf("put seat %s: %v", seatID, err)
		}
	}

	log.Printf("seeded %d seats into %s", seatCount, seatsTable)
}
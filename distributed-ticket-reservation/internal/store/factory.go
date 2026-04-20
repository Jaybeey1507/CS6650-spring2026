package store

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

func NewBackendFromEnv(ctx context.Context) (Backend, error) {
	backend := strings.ToLower(strings.TrimSpace(os.Getenv("STORE_BACKEND")))
	if backend == "" || backend == "memory" {
		return NewMemoryStore(), nil
	}

	if backend != "dynamo" {
		return nil, errors.New("STORE_BACKEND must be 'memory' or 'dynamo'")
	}

	seatsTable := os.Getenv("DDB_SEATS_TABLE")
	holdsTable := os.Getenv("DDB_HOLDS_TABLE")
	reservationsTable := os.Getenv("DDB_RESERVATIONS_TABLE")

	if seatsTable == "" || holdsTable == "" || reservationsTable == "" {
		return nil, errors.New("missing DDB_SEATS_TABLE, DDB_HOLDS_TABLE, or DDB_RESERVATIONS_TABLE")
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	client := dynamodb.NewFromConfig(cfg)

	return NewDynamoStore(client, DynamoConfig{
		SeatsTable:        seatsTable,
		HoldsTable:        holdsTable,
		ReservationsTable: reservationsTable,
	}), nil
}
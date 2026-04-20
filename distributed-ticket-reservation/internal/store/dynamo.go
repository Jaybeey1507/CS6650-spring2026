package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"distributed-ticket-reservation/internal/models"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type DynamoConfig struct {
	SeatsTable        string
	HoldsTable        string
	ReservationsTable string
}

type DynamoStore struct {
	client *dynamodb.Client
	cfg    DynamoConfig
}

type dynamoSeat struct {
	EventID       string `dynamodbav:"EventID"`
	SeatID        string `dynamodbav:"SeatID"`
	Number        string `dynamodbav:"Number"`
	Status        string `dynamodbav:"Status"`
	HoldID        string `dynamodbav:"HoldID,omitempty"`
	HoldExpiresAt int64  `dynamodbav:"HoldExpiresAt,omitempty"`
}

type dynamoHold struct {
	HoldID        string `dynamodbav:"HoldID"`
	EventID       string `dynamodbav:"EventID"`
	SeatID        string `dynamodbav:"SeatID"`
	UserID        string `dynamodbav:"UserID"`
	Status        string `dynamodbav:"Status"`
	CreatedAtUnix int64  `dynamodbav:"CreatedAtUnix"`
	ExpiresAtUnix int64  `dynamodbav:"ExpiresAtUnix"`
}

type dynamoReservation struct {
	ReservationID string `dynamodbav:"ReservationID"`
	HoldID        string `dynamodbav:"HoldID"`
	EventID       string `dynamodbav:"EventID"`
	SeatID        string `dynamodbav:"SeatID"`
	UserID        string `dynamodbav:"UserID"`
	CreatedAtUnix int64  `dynamodbav:"CreatedAtUnix"`
}

func NewDynamoStore(client *dynamodb.Client, cfg DynamoConfig) *DynamoStore {
	return &DynamoStore{
		client: client,
		cfg:    cfg,
	}
}

func (s *DynamoStore) ListEvents(ctx context.Context) ([]models.Event, error) {
	return defaultEvents(), nil
}

func (s *DynamoStore) ListSeats(ctx context.Context, eventID string) ([]models.Seat, error) {
	out, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.cfg.SeatsTable),
		KeyConditionExpression: aws.String("EventID = :event_id"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":event_id": &ddbtypes.AttributeValueMemberS{Value: eventID},
		},
	})
	if err != nil {
		return nil, err
	}

	var items []dynamoSeat
	if err := attributevalue.UnmarshalListOfMaps(out.Items, &items); err != nil {
		return nil, err
	}

	seats := make([]models.Seat, 0, len(items))
	for _, item := range items {
		seats = append(seats, models.Seat{
			ID:      item.SeatID,
			EventID: item.EventID,
			Number:  item.Number,
			Status:  item.Status,
			HoldID:  item.HoldID,
		})
	}

	sort.Slice(seats, func(i, j int) bool {
		return seats[i].ID < seats[j].ID
	})

	return seats, nil
}

func (s *DynamoStore) PlaceHold(ctx context.Context, eventID, seatID, userID string, ttlSeconds int) (models.Hold, error) {
	now := time.Now()
	ttl := time.Duration(ttlSeconds) * time.Second
	if ttlSeconds <= 0 {
		ttl = 2 * time.Minute
	}

	hold := models.Hold{
		ID:        fmt.Sprintf("hold-%d", now.UnixNano()),
		EventID:   eventID,
		SeatID:    seatID,
		UserID:    userID,
		Status:    models.HoldActive,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}

	holdItem, err := attributevalue.MarshalMap(dynamoHold{
		HoldID:        hold.ID,
		EventID:       hold.EventID,
		SeatID:        hold.SeatID,
		UserID:        hold.UserID,
		Status:        hold.Status,
		CreatedAtUnix: hold.CreatedAt.Unix(),
		ExpiresAtUnix: hold.ExpiresAt.Unix(),
	})
	if err != nil {
		return models.Hold{}, err
	}

	_, err = s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []ddbtypes.TransactWriteItem{
			{
				Update: &ddbtypes.Update{
					TableName: aws.String(s.cfg.SeatsTable),
					Key: map[string]ddbtypes.AttributeValue{
						"EventID": &ddbtypes.AttributeValueMemberS{Value: eventID},
						"SeatID":  &ddbtypes.AttributeValueMemberS{Value: seatID},
					},
					UpdateExpression:    aws.String("SET #status = :held, HoldID = :hold_id, HoldExpiresAt = :hold_expires_at"),
					ConditionExpression: aws.String("#status = :available"),
					ExpressionAttributeNames: map[string]string{
						"#status": "Status",
					},
					ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
						":held":            &ddbtypes.AttributeValueMemberS{Value: models.SeatHeld},
						":available":       &ddbtypes.AttributeValueMemberS{Value: models.SeatAvailable},
						":hold_id":         &ddbtypes.AttributeValueMemberS{Value: hold.ID},
						":hold_expires_at": &ddbtypes.AttributeValueMemberN{Value: fmt.Sprintf("%d", hold.ExpiresAt.Unix())},
					},
				},
			},
			{
				Put: &ddbtypes.Put{
					TableName:           aws.String(s.cfg.HoldsTable),
					Item:                holdItem,
					ConditionExpression: aws.String("attribute_not_exists(HoldID)"),
				},
			},
		},
	})
	if err != nil {
		return models.Hold{}, errors.New("seat is not available")
	}

	return hold, nil
}

func (s *DynamoStore) ConfirmReservation(ctx context.Context, holdID, userID string) (models.Reservation, error) {
	getOut, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.cfg.HoldsTable),
		Key: map[string]ddbtypes.AttributeValue{
			"HoldID": &ddbtypes.AttributeValueMemberS{Value: holdID},
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return models.Reservation{}, err
	}
	if len(getOut.Item) == 0 {
		return models.Reservation{}, errors.New("hold not found")
	}

	var holdRec dynamoHold
	if err := attributevalue.UnmarshalMap(getOut.Item, &holdRec); err != nil {
		return models.Reservation{}, err
	}

	hold := models.Hold{
		ID:        holdRec.HoldID,
		EventID:   holdRec.EventID,
		SeatID:    holdRec.SeatID,
		UserID:    holdRec.UserID,
		Status:    holdRec.Status,
		CreatedAt: time.Unix(holdRec.CreatedAtUnix, 0),
		ExpiresAt: time.Unix(holdRec.ExpiresAtUnix, 0),
	}

	if hold.Status != models.HoldActive {
		return models.Reservation{}, errors.New("hold is not active")
	}
	if hold.UserID != userID {
		return models.Reservation{}, errors.New("hold does not belong to this user")
	}
	if time.Now().After(hold.ExpiresAt) {
		_ = s.ReleaseExpiredHolds(ctx)
		return models.Reservation{}, errors.New("hold expired")
	}

	reservation := models.Reservation{
		ID:        fmt.Sprintf("res-%d", time.Now().UnixNano()),
		HoldID:    hold.ID,
		EventID:   hold.EventID,
		SeatID:    hold.SeatID,
		UserID:    hold.UserID,
		CreatedAt: time.Now(),
	}

	reservationItem, err := attributevalue.MarshalMap(dynamoReservation{
		ReservationID: reservation.ID,
		HoldID:        reservation.HoldID,
		EventID:       reservation.EventID,
		SeatID:        reservation.SeatID,
		UserID:        reservation.UserID,
		CreatedAtUnix: reservation.CreatedAt.Unix(),
	})
	if err != nil {
		return models.Reservation{}, err
	}

	_, err = s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []ddbtypes.TransactWriteItem{
			{
				Update: &ddbtypes.Update{
					TableName: aws.String(s.cfg.SeatsTable),
					Key: map[string]ddbtypes.AttributeValue{
						"EventID": &ddbtypes.AttributeValueMemberS{Value: hold.EventID},
						"SeatID":  &ddbtypes.AttributeValueMemberS{Value: hold.SeatID},
					},
					UpdateExpression:    aws.String("SET #status = :reserved REMOVE HoldExpiresAt"),
					ConditionExpression: aws.String("#status = :held AND HoldID = :hold_id"),
					ExpressionAttributeNames: map[string]string{
						"#status": "Status",
					},
					ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
						":reserved": &ddbtypes.AttributeValueMemberS{Value: models.SeatReserved},
						":held":     &ddbtypes.AttributeValueMemberS{Value: models.SeatHeld},
						":hold_id":  &ddbtypes.AttributeValueMemberS{Value: hold.ID},
					},
				},
			},
			{
				Update: &ddbtypes.Update{
					TableName: aws.String(s.cfg.HoldsTable),
					Key: map[string]ddbtypes.AttributeValue{
						"HoldID": &ddbtypes.AttributeValueMemberS{Value: hold.ID},
					},
					UpdateExpression:    aws.String("SET #status = :confirmed"),
					ConditionExpression: aws.String("#status = :active AND UserID = :user_id"),
					ExpressionAttributeNames: map[string]string{
						"#status": "Status",
					},
					ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
						":confirmed": &ddbtypes.AttributeValueMemberS{Value: models.HoldConfirmed},
						":active":    &ddbtypes.AttributeValueMemberS{Value: models.HoldActive},
						":user_id":   &ddbtypes.AttributeValueMemberS{Value: userID},
					},
				},
			},
			{
				Put: &ddbtypes.Put{
					TableName:           aws.String(s.cfg.ReservationsTable),
					Item:                reservationItem,
					ConditionExpression: aws.String("attribute_not_exists(ReservationID)"),
				},
			},
		},
	})
	if err != nil {
		return models.Reservation{}, errors.New("seat is not held by this hold")
	}

	return reservation, nil
}

func (s *DynamoStore) ReleaseExpiredHolds(ctx context.Context) error {
	var startKey map[string]ddbtypes.AttributeValue
	nowUnix := time.Now().Unix()

	for {
		out, err := s.client.Scan(ctx, &dynamodb.ScanInput{
			TableName:         aws.String(s.cfg.SeatsTable),
			ExclusiveStartKey: startKey,
			FilterExpression:  aws.String("#status = :held AND HoldExpiresAt <= :now"),
			ExpressionAttributeNames: map[string]string{
				"#status": "Status",
			},
			ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
				":held": &ddbtypes.AttributeValueMemberS{Value: models.SeatHeld},
				":now":  &ddbtypes.AttributeValueMemberN{Value: fmt.Sprintf("%d", nowUnix)},
			},
		})
		if err != nil {
			return err
		}

		var seats []dynamoSeat
		if err := attributevalue.UnmarshalListOfMaps(out.Items, &seats); err != nil {
			return err
		}

		for _, seat := range seats {
			if seat.HoldID == "" {
				continue
			}

			_, _ = s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
				TableName: aws.String(s.cfg.SeatsTable),
				Key: map[string]ddbtypes.AttributeValue{
					"EventID": &ddbtypes.AttributeValueMemberS{Value: seat.EventID},
					"SeatID":  &ddbtypes.AttributeValueMemberS{Value: seat.SeatID},
				},
				UpdateExpression:    aws.String("SET #status = :available REMOVE HoldID, HoldExpiresAt"),
				ConditionExpression: aws.String("#status = :held AND HoldID = :hold_id"),
				ExpressionAttributeNames: map[string]string{
					"#status": "Status",
				},
				ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
					":available": &ddbtypes.AttributeValueMemberS{Value: models.SeatAvailable},
					":held":      &ddbtypes.AttributeValueMemberS{Value: models.SeatHeld},
					":hold_id":   &ddbtypes.AttributeValueMemberS{Value: seat.HoldID},
				},
			})

			_, _ = s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
				TableName: aws.String(s.cfg.HoldsTable),
				Key: map[string]ddbtypes.AttributeValue{
					"HoldID": &ddbtypes.AttributeValueMemberS{Value: seat.HoldID},
				},
				UpdateExpression:    aws.String("SET #status = :expired"),
				ConditionExpression: aws.String("#status = :active"),
				ExpressionAttributeNames: map[string]string{
					"#status": "Status",
				},
				ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
					":expired": &ddbtypes.AttributeValueMemberS{Value: models.HoldExpired},
					":active":  &ddbtypes.AttributeValueMemberS{Value: models.HoldActive},
				},
			})
		}

		if len(out.LastEvaluatedKey) == 0 {
			break
		}
		startKey = out.LastEvaluatedKey
	}

	return nil
}
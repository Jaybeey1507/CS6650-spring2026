package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gin-gonic/gin"
)

type Cart struct {
	CartID     string     `json:"cart_id" dynamodbav:"cart_id"`
	CustomerID int64      `json:"customer_id" dynamodbav:"customer_id"`
	Status     string     `json:"status" dynamodbav:"status"`
	Items      []CartItem `json:"items" dynamodbav:"items"`
	CreatedAt  string     `json:"created_at" dynamodbav:"created_at"`
	UpdatedAt  string     `json:"updated_at" dynamodbav:"updated_at"`
}

type CartItem struct {
	ProductID   int64   `json:"product_id" dynamodbav:"product_id"`
	ProductName string  `json:"product_name" dynamodbav:"product_name"`
	UnitPrice   float64 `json:"unit_price" dynamodbav:"unit_price"`
	Quantity    int     `json:"quantity" dynamodbav:"quantity"`
}

type CreateCartRequest struct {
	CustomerID int64 `json:"customer_id"`
}

type AddItemRequest struct {
	ProductID   int64   `json:"product_id"`
	ProductName string  `json:"product_name"`
	UnitPrice   float64 `json:"unit_price"`
	Quantity    int     `json:"quantity"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

var (
	ddb             *dynamodb.Client
	cartsTable      string
	errCartNotFound = errors.New("cart not found")
)

func main() {
	var err error
	ddb, cartsTable, err = initDynamo()
	if err != nil {
		log.Fatalf("failed to initialize dynamodb: %v", err)
	}

	r := gin.Default()

	r.GET("/health", healthCheck)

	// Step II primary routes
	r.POST("/shopping-carts", createCart)
	r.GET("/shopping-carts/:id", getCartByID)
	r.POST("/shopping-carts/:id/items", addItemToCart)

	// Alias routes
	r.POST("/carts", createCart)
	r.GET("/carts/:id", getCartByID)
	r.POST("/carts/:id/items", addItemToCart)

	log.Println("dynamodb cart server starting on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func initDynamo() (*dynamodb.Client, string, error) {
	region := strings.TrimSpace(os.Getenv("AWS_REGION"))
	table := strings.TrimSpace(os.Getenv("CARTS_TABLE"))

	if region == "" {
		region = "us-west-2"
	}
	if table == "" {
		return nil, "", fmt.Errorf("missing CARTS_TABLE environment variable")
	}

	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		return nil, "", err
	}

	client := dynamodb.NewFromConfig(cfg)

	_, err = client.DescribeTable(context.Background(), &dynamodb.DescribeTableInput{
		TableName: aws.String(table),
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to describe dynamodb table: %w", err)
	}

	return client, table, nil
}

func healthCheck(c *gin.Context) {
	_, err := ddb.DescribeTable(context.Background(), &dynamodb.DescribeTableInput{
		TableName: aws.String(cartsTable),
	})
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status":  "error",
			"storage": "dynamodb",
			"table":   cartsTable,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"storage": "dynamodb",
		"table":   cartsTable,
	})
}

func createCart(c *gin.Context) {
	var req CreateCartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid JSON body",
			Details: err.Error(),
		})
		return
	}

	if req.CustomerID < 1 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid customer_id",
			Details: "customer_id must be >= 1",
		})
		return
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	cart := Cart{
		CartID:     newCartID(),
		CustomerID: req.CustomerID,
		Status:     "active",
		Items:      []CartItem{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	item, err := attributevalue.MarshalMap(cart)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "MARSHAL_ERROR",
			Message: "Failed to marshal cart",
			Details: err.Error(),
		})
		return
	}

	_, err = ddb.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName:           aws.String(cartsTable),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(cart_id)"),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DYNAMODB_ERROR",
			Message: "Failed to create cart",
			Details: err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, cart)
}

func getCartByID(c *gin.Context) {
	cartID := strings.TrimSpace(c.Param("id"))
	if cartID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid cart id",
			Details: "cart id must not be empty",
		})
		return
	}

	consistent := strings.EqualFold(c.DefaultQuery("consistent", "false"), "true")

	cart, err := loadCartByID(context.Background(), cartID, consistent)
	if err != nil {
		if errors.Is(err, errCartNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "NOT_FOUND",
				Message: "Cart not found",
				Details: "No cart exists with the given id",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DYNAMODB_ERROR",
			Message: "Failed to retrieve cart",
			Details: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, cart)
}

func addItemToCart(c *gin.Context) {
	cartID := strings.TrimSpace(c.Param("id"))
	if cartID == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid cart id",
			Details: "cart id must not be empty",
		})
		return
	}

	var req AddItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid JSON body",
			Details: err.Error(),
		})
		return
	}

	if req.ProductID < 1 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid product_id",
			Details: "product_id must be >= 1",
		})
		return
	}
	if strings.TrimSpace(req.ProductName) == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid product_name",
			Details: "product_name must not be empty",
		})
		return
	}
	if req.UnitPrice < 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid unit_price",
			Details: "unit_price must be >= 0",
		})
		return
	}
	if req.Quantity < 1 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid quantity",
			Details: "quantity must be >= 1",
		})
		return
	}

	cart, err := loadCartByID(context.Background(), cartID, true)
	if err != nil {
		if errors.Is(err, errCartNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "NOT_FOUND",
				Message: "Cart not found",
				Details: "No cart exists with the given id",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DYNAMODB_ERROR",
			Message: "Failed to load cart before update",
			Details: err.Error(),
		})
		return
	}

	found := false
	for i := range cart.Items {
		if cart.Items[i].ProductID == req.ProductID {
			cart.Items[i].Quantity += req.Quantity
			cart.Items[i].ProductName = strings.TrimSpace(req.ProductName)
			cart.Items[i].UnitPrice = req.UnitPrice
			found = true
			break
		}
	}

	if !found {
		cart.Items = append(cart.Items, CartItem{
			ProductID:   req.ProductID,
			ProductName: strings.TrimSpace(req.ProductName),
			UnitPrice:   req.UnitPrice,
			Quantity:    req.Quantity,
		})
	}

	cart.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)

	item, err := attributevalue.MarshalMap(cart)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "MARSHAL_ERROR",
			Message: "Failed to marshal updated cart",
			Details: err.Error(),
		})
		return
	}

	_, err = ddb.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(cartsTable),
		Item:      item,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DYNAMODB_ERROR",
			Message: "Failed to save updated cart",
			Details: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, cart)
}

func loadCartByID(ctx context.Context, cartID string, consistent bool) (*Cart, error) {
	out, err := ddb.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(cartsTable),
		Key: map[string]types.AttributeValue{
			"cart_id": &types.AttributeValueMemberS{Value: cartID},
		},
		ConsistentRead: aws.Bool(consistent),
	})
	if err != nil {
		return nil, err
	}
	if out.Item == nil || len(out.Item) == 0 {
		return nil, errCartNotFound
	}

	var cart Cart
	if err := attributevalue.UnmarshalMap(out.Item, &cart); err != nil {
		return nil, err
	}
	if cart.Items == nil {
		cart.Items = []CartItem{}
	}

	return &cart, nil
}

func newCartID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("cart-%d", time.Now().UnixNano())
	}
	return "cart-" + hex.EncodeToString(b)
}
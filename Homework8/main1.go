package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gin-gonic/gin"
)

type Product struct {
	ProductID    int32  `json:"product_id"`
	SKU          string `json:"sku"`
	Manufacturer string `json:"manufacturer"`
	CategoryID   int32  `json:"category_id"`
	Weight       int32  `json:"weight"`
	SomeOtherID  int32  `json:"some_other_id"`
}

type Cart struct {
	CartID     int64      `json:"cart_id"`
	CustomerID int64      `json:"customer_id"`
	Status     string     `json:"status"`
	Items      []CartItem `json:"items"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type CartItem struct {
	CartItemID  int64     `json:"cart_item_id"`
	CartID      int64     `json:"cart_id"`
	ProductID   int64     `json:"product_id"`
	ProductName string    `json:"product_name"`
	UnitPrice   float64   `json:"unit_price"`
	Quantity    int       `json:"quantity"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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

type UpdateItemQuantityRequest struct {
	Quantity int `json:"quantity"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

var (
	mu       sync.RWMutex
	products = map[int32]Product{
		1: {
			ProductID:    1,
			SKU:          "ABC-123-XYZ",
			Manufacturer: "Acme Corporation",
			CategoryID:   456,
			Weight:       1250,
			SomeOtherID:  789,
		},
	}

	db              *sql.DB
	errCartNotFound = errors.New("cart not found")
)

func main() {
	var err error
	db, err = initDB()
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer db.Close()

	r := gin.Default()

	r.GET("/health", healthCheck)

	// Existing product API
	r.GET("/products/:productId", getProductByID)
	r.POST("/products/:productId/details", addProductDetails)

	// Homework 8 cart API
	r.POST("/carts", createCart)
	r.GET("/carts/:cartId", getCartByID)
	r.POST("/carts/:cartId/items", addItemToCart)
	r.PUT("/carts/:cartId/items/:productId", updateCartItemQuantity)
	r.DELETE("/carts/:cartId/items/:productId", removeCartItem)

	log.Println("server starting on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

func initDB() (*sql.DB, error) {
	host := strings.TrimSpace(os.Getenv("DB_HOST"))
	port := strings.TrimSpace(os.Getenv("DB_PORT"))
	name := strings.TrimSpace(os.Getenv("DB_NAME"))
	user := strings.TrimSpace(os.Getenv("DB_USER"))
	password := os.Getenv("DB_PASSWORD")

	if host == "" || port == "" || name == "" || user == "" {
		return nil, fmt.Errorf("missing one or more required DB environment variables")
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&multiStatements=true",
		user, password, host, port, name)

	database, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	database.SetMaxOpenConns(25)
	database.SetMaxIdleConns(10)
	database.SetConnMaxLifetime(5 * time.Minute)

	if err := database.Ping(); err != nil {
		return nil, err
	}

	schemaBytes, err := os.ReadFile("schema.sql")
	if err != nil {
		return nil, fmt.Errorf("failed to read schema.sql: %w", err)
	}

	if _, err := database.Exec(string(schemaBytes)); err != nil {
		return nil, fmt.Errorf("failed to execute schema.sql: %w", err)
	}

	log.Println("database connected and schema initialized")
	return database, nil
}

func healthCheck(c *gin.Context) {
	if db == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "error",
			"db":     "not initialized",
		})
		return
	}

	if err := db.Ping(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "error",
			"db":     "unreachable",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"db":     "connected",
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
			Details: "customer_id must be an integer >= 1",
		})
		return
	}

	result, err := db.Exec(`
		INSERT INTO carts (customer_id, status)
		VALUES (?, 'active')
	`, req.CustomerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Failed to create cart",
			Details: err.Error(),
		})
		return
	}

	cartID, err := result.LastInsertId()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Cart created but ID could not be retrieved",
			Details: err.Error(),
		})
		return
	}

	cart, err := loadCartByID(cartID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Cart created but could not be loaded",
			Details: err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, cart)
}

func getCartByID(c *gin.Context) {
	cartID, ok := parsePositiveInt64Param(c, "cartId")
	if !ok {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid cartId",
			Details: "cartId must be an integer >= 1",
		})
		return
	}

	cart, err := loadCartByID(cartID)
	if err != nil {
		if errors.Is(err, errCartNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "NOT_FOUND",
				Message: "Cart not found",
				Details: "No cart exists with the given cartId",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Failed to retrieve cart",
			Details: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, cart)
}

func addItemToCart(c *gin.Context) {
	cartID, ok := parsePositiveInt64Param(c, "cartId")
	if !ok {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid cartId",
			Details: "cartId must be an integer >= 1",
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
			Details: "product_id must be an integer >= 1",
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

	tx, err := db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Failed to start transaction",
			Details: err.Error(),
		})
		return
	}
	defer tx.Rollback()

	var existingCartID int64
	err = tx.QueryRow(`
		SELECT cart_id
		FROM carts
		WHERE cart_id = ?
	`, cartID).Scan(&existingCartID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "NOT_FOUND",
				Message: "Cart not found",
				Details: "No cart exists with the given cartId",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Failed to check cart",
			Details: err.Error(),
		})
		return
	}

	var existingCartItemID int64
	var existingQuantity int
	err = tx.QueryRow(`
		SELECT cart_item_id, quantity
		FROM cart_items
		WHERE cart_id = ? AND product_id = ?
		FOR UPDATE
	`, cartID, req.ProductID).Scan(&existingCartItemID, &existingQuantity)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Failed to inspect existing cart item",
			Details: err.Error(),
		})
		return
	}

	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.Exec(`
			INSERT INTO cart_items (cart_id, product_id, product_name, unit_price, quantity)
			VALUES (?, ?, ?, ?, ?)
		`, cartID, req.ProductID, strings.TrimSpace(req.ProductName), req.UnitPrice, req.Quantity)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "DATABASE_ERROR",
				Message: "Failed to add item to cart",
				Details: err.Error(),
			})
			return
		}
	} else {
		_, err = tx.Exec(`
			UPDATE cart_items
			SET quantity = ?, product_name = ?, unit_price = ?, updated_at = CURRENT_TIMESTAMP
			WHERE cart_item_id = ?
		`, existingQuantity+req.Quantity, strings.TrimSpace(req.ProductName), req.UnitPrice, existingCartItemID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "DATABASE_ERROR",
				Message: "Failed to update cart item",
				Details: err.Error(),
			})
			return
		}
	}

	_, err = tx.Exec(`
		UPDATE carts
		SET updated_at = CURRENT_TIMESTAMP
		WHERE cart_id = ?
	`, cartID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Failed to update cart timestamp",
			Details: err.Error(),
		})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Failed to commit transaction",
			Details: err.Error(),
		})
		return
	}

	cart, err := loadCartByID(cartID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Item added but updated cart could not be loaded",
			Details: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, cart)
}

func updateCartItemQuantity(c *gin.Context) {
	cartID, ok := parsePositiveInt64Param(c, "cartId")
	if !ok {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid cartId",
			Details: "cartId must be an integer >= 1",
		})
		return
	}

	productID, ok := parsePositiveInt64Param(c, "productId")
	if !ok {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid productId",
			Details: "productId must be an integer >= 1",
		})
		return
	}

	var req UpdateItemQuantityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid JSON body",
			Details: err.Error(),
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

	tx, err := db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Failed to start transaction",
			Details: err.Error(),
		})
		return
	}
	defer tx.Rollback()

	var existingCartID int64
	err = tx.QueryRow(`
		SELECT cart_id
		FROM carts
		WHERE cart_id = ?
	`, cartID).Scan(&existingCartID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "NOT_FOUND",
				Message: "Cart not found",
				Details: "No cart exists with the given cartId",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Failed to check cart",
			Details: err.Error(),
		})
		return
	}

	result, err := tx.Exec(`
		UPDATE cart_items
		SET quantity = ?, updated_at = CURRENT_TIMESTAMP
		WHERE cart_id = ? AND product_id = ?
	`, req.Quantity, cartID, productID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Failed to update cart item",
			Details: err.Error(),
		})
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Failed to confirm cart item update",
			Details: err.Error(),
		})
		return
	}

	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "NOT_FOUND",
			Message: "Cart item not found",
			Details: "No matching product exists in the given cart",
		})
		return
	}

	_, err = tx.Exec(`
		UPDATE carts
		SET updated_at = CURRENT_TIMESTAMP
		WHERE cart_id = ?
	`, cartID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Failed to update cart timestamp",
			Details: err.Error(),
		})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Failed to commit transaction",
			Details: err.Error(),
		})
		return
	}

	cart, err := loadCartByID(cartID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Item updated but cart could not be loaded",
			Details: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, cart)
}

func removeCartItem(c *gin.Context) {
	cartID, ok := parsePositiveInt64Param(c, "cartId")
	if !ok {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid cartId",
			Details: "cartId must be an integer >= 1",
		})
		return
	}

	productID, ok := parsePositiveInt64Param(c, "productId")
	if !ok {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid productId",
			Details: "productId must be an integer >= 1",
		})
		return
	}

	tx, err := db.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Failed to start transaction",
			Details: err.Error(),
		})
		return
	}
	defer tx.Rollback()

	var existingCartID int64
	err = tx.QueryRow(`
		SELECT cart_id
		FROM carts
		WHERE cart_id = ?
	`, cartID).Scan(&existingCartID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "NOT_FOUND",
				Message: "Cart not found",
				Details: "No cart exists with the given cartId",
			})
			return
		}

		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Failed to check cart",
			Details: err.Error(),
		})
		return
	}

	result, err := tx.Exec(`
		DELETE FROM cart_items
		WHERE cart_id = ? AND product_id = ?
	`, cartID, productID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Failed to remove cart item",
			Details: err.Error(),
		})
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Failed to confirm cart item removal",
			Details: err.Error(),
		})
		return
	}

	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "NOT_FOUND",
			Message: "Cart item not found",
			Details: "No matching product exists in the given cart",
		})
		return
	}

	_, err = tx.Exec(`
		UPDATE carts
		SET updated_at = CURRENT_TIMESTAMP
		WHERE cart_id = ?
	`, cartID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Failed to update cart timestamp",
			Details: err.Error(),
		})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Failed to commit transaction",
			Details: err.Error(),
		})
		return
	}

	cart, err := loadCartByID(cartID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "DATABASE_ERROR",
			Message: "Item removed but cart could not be loaded",
			Details: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, cart)
}

func loadCartByID(cartID int64) (*Cart, error) {
	var cart Cart

	err := db.QueryRow(`
		SELECT cart_id, customer_id, status, created_at, updated_at
		FROM carts
		WHERE cart_id = ?
	`, cartID).Scan(
		&cart.CartID,
		&cart.CustomerID,
		&cart.Status,
		&cart.CreatedAt,
		&cart.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errCartNotFound
		}
		return nil, err
	}

	rows, err := db.Query(`
		SELECT cart_item_id, cart_id, product_id, product_name, unit_price, quantity, created_at, updated_at
		FROM cart_items
		WHERE cart_id = ?
		ORDER BY cart_item_id ASC
	`, cartID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cart.Items = make([]CartItem, 0)

	for rows.Next() {
		var item CartItem
		if err := rows.Scan(
			&item.CartItemID,
			&item.CartID,
			&item.ProductID,
			&item.ProductName,
			&item.UnitPrice,
			&item.Quantity,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		cart.Items = append(cart.Items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &cart, nil
}

func getProductByID(c *gin.Context) {
	productID, ok := parsePositiveInt32Param(c, "productId")
	if !ok {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid productId",
			Details: "productId must be an integer >= 1",
		})
		return
	}

	mu.RLock()
	p, exists := products[productID]
	mu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "NOT_FOUND",
			Message: "Product not found",
			Details: "No product exists with the given productId",
		})
		return
	}

	c.JSON(http.StatusOK, p)
}

func addProductDetails(c *gin.Context) {
	productID, ok := parsePositiveInt32Param(c, "productId")
	if !ok {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid productId",
			Details: "productId must be an integer >= 1",
		})
		return
	}

	var body Product
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Invalid JSON body",
			Details: err.Error(),
		})
		return
	}

	if body.ProductID != productID {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "Path productId does not match body product_id",
			Details: "product_id must equal the productId in the URL path",
		})
		return
	}

	if err := validateProduct(body); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "INVALID_INPUT",
			Message: "The provided input data is invalid",
			Details: err.Error(),
		})
		return
	}

	mu.RLock()
	_, exists := products[productID]
	mu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "NOT_FOUND",
			Message: "Product not found",
			Details: "Product must exist before adding details",
		})
		return
	}

	mu.Lock()
	products[productID] = body
	mu.Unlock()

	c.Status(http.StatusNoContent)
}

func parsePositiveInt32Param(c *gin.Context, name string) (int32, bool) {
	raw := c.Param(name)
	v64, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || v64 < 1 {
		return 0, false
	}
	return int32(v64), true
}

func parsePositiveInt64Param(c *gin.Context, name string) (int64, bool) {
	raw := c.Param(name)
	v64, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v64 < 1 {
		return 0, false
	}
	return v64, true
}

func validateProduct(p Product) error {
	if p.ProductID < 1 {
		return simpleErr("product_id must be >= 1")
	}
	if p.CategoryID < 1 {
		return simpleErr("category_id must be >= 1")
	}
	if p.Weight < 0 {
		return simpleErr("weight must be >= 0")
	}
	if p.SomeOtherID < 1 {
		return simpleErr("some_other_id must be >= 1")
	}

	sku := strings.TrimSpace(p.SKU)
	if len(sku) < 1 || len(sku) > 100 {
		return simpleErr("sku length must be between 1 and 100 characters")
	}

	man := strings.TrimSpace(p.Manufacturer)
	if len(man) < 1 || len(man) > 200 {
		return simpleErr("manufacturer length must be between 1 and 200 characters")
	}

	return nil
}

type simpleErr string

func (e simpleErr) Error() string { return string(e) }
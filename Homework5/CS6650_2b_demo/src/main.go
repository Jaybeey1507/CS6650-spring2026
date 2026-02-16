package main

import (
	"net/http"
	"strconv"
	"strings"
	"sync"

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

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

var (
	mu       sync.RWMutex
	products = map[int32]Product{
		// Seed one product so GET works right away and POST can return 204
		1: {
			ProductID:    1,
			SKU:          "ABC-123-XYZ",
			Manufacturer: "Acme Corporation",
			CategoryID:   456,
			Weight:       1250,
			SomeOtherID:  789,
		},
	}
)

func main() {
	r := gin.Default()

	// Product API only
	r.GET("/products/:productId", getProductByID)
	r.POST("/products/:productId/details", addProductDetails)

	r.Run(":8080")
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

	// Strict spec interpretation: return 404 if product not found
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

	c.Status(http.StatusNoContent) // 204
}

func parsePositiveInt32Param(c *gin.Context, name string) (int32, bool) {
	raw := c.Param(name)
	v64, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || v64 < 1 {
		return 0, false
	}
	return int32(v64), true
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

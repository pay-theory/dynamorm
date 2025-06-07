package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/google/uuid"
	"github.com/yourusername/dynamorm"
	"github.com/yourusername/dynamorm/examples/ecommerce/models"
	"github.com/yourusername/dynamorm/lambda"
)

// ProductHandlers manages product-related operations
type ProductHandlers struct {
	db *lambda.OptimizedClient
}

// NewProductHandlers creates a new product handler instance
func NewProductHandlers(db *lambda.OptimizedClient) *ProductHandlers {
	return &ProductHandlers{db: db}
}

// ListProducts handles GET /products requests with pagination and filtering
func (h *ProductHandlers) ListProducts(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Parse query parameters
	limit := 20
	if l := request.QueryStringParameters["limit"]; l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	var lastEvaluatedKey map[string]interface{}
	if cursor := request.QueryStringParameters["cursor"]; cursor != "" {
		// Decode cursor from base64
		if err := json.Unmarshal([]byte(cursor), &lastEvaluatedKey); err != nil {
			return errorResponse(http.StatusBadRequest, "Invalid cursor")
		}
	}

	// Filter parameters
	categoryID := request.QueryStringParameters["category_id"]
	status := request.QueryStringParameters["status"]
	if status == "" {
		status = models.ProductStatusActive
	}

	var products []models.Product
	var nextCursor string
	var err error

	if categoryID != "" {
		// Query by category
		query := h.db.Query("gsi-category").
			Where("CategoryID", "=", categoryID).
			Limit(limit)

		if lastEvaluatedKey != nil {
			query = query.StartFrom(lastEvaluatedKey)
		}

		result, err := query.Execute(ctx, &products)
		if err != nil {
			return errorResponse(http.StatusInternalServerError, "Failed to fetch products")
		}

		if result.LastEvaluatedKey != nil {
			cursor, _ := json.Marshal(result.LastEvaluatedKey)
			nextCursor = string(cursor)
		}
	} else {
		// Scan all products with status filter
		scan := h.db.Scan().
			Filter("Status", "=", status).
			Limit(limit)

		if lastEvaluatedKey != nil {
			scan = scan.StartFrom(lastEvaluatedKey)
		}

		result, err := scan.Execute(ctx, &products)
		if err != nil {
			return errorResponse(http.StatusInternalServerError, "Failed to fetch products")
		}

		if result.LastEvaluatedKey != nil {
			cursor, _ := json.Marshal(result.LastEvaluatedKey)
			nextCursor = string(cursor)
		}
	}

	// Filter by tags if requested
	if tags := request.QueryStringParameters["tags"]; tags != "" {
		tagList := strings.Split(tags, ",")
		filtered := make([]models.Product, 0)
		for _, product := range products {
			for _, tag := range tagList {
				if contains(product.Tags, tag) {
					filtered = append(filtered, product)
					break
				}
			}
		}
		products = filtered
	}

	// Build response
	response := map[string]interface{}{
		"products": products,
		"metadata": map[string]interface{}{
			"count": len(products),
			"limit": limit,
		},
	}

	if nextCursor != "" {
		response["metadata"].(map[string]interface{})["next_cursor"] = nextCursor
	}

	return jsonResponse(http.StatusOK, response)
}

// GetProduct handles GET /products/{id} requests
func (h *ProductHandlers) GetProduct(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	productID := request.PathParameters["id"]
	if productID == "" {
		return errorResponse(http.StatusBadRequest, "Product ID is required")
	}

	var product models.Product
	err := h.db.GetItem(ctx, productID, &product)
	if err != nil {
		if err == dynamorm.ErrItemNotFound {
			return errorResponse(http.StatusNotFound, "Product not found")
		}
		return errorResponse(http.StatusInternalServerError, "Failed to fetch product")
	}

	// Only return active products to customers
	if product.Status != models.ProductStatusActive && !isAdminRequest(request) {
		return errorResponse(http.StatusNotFound, "Product not found")
	}

	// Calculate available stock
	availableStock := product.Stock - product.Reserved
	productResponse := map[string]interface{}{
		"product": product,
		"availability": map[string]interface{}{
			"in_stock":        availableStock > 0,
			"available_stock": availableStock,
		},
	}

	return jsonResponse(http.StatusOK, productResponse)
}

// GetProductBySKU handles GET /products/sku/{sku} requests
func (h *ProductHandlers) GetProductBySKU(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	sku := request.PathParameters["sku"]
	if sku == "" {
		return errorResponse(http.StatusBadRequest, "SKU is required")
	}

	var products []models.Product
	result, err := h.db.Query("gsi-sku").
		Where("SKU", "=", sku).
		Limit(1).
		Execute(ctx, &products)

	if err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to fetch product")
	}

	if len(products) == 0 {
		return errorResponse(http.StatusNotFound, "Product not found")
	}

	product := products[0]

	// Only return active products to customers
	if product.Status != models.ProductStatusActive && !isAdminRequest(request) {
		return errorResponse(http.StatusNotFound, "Product not found")
	}

	return jsonResponse(http.StatusOK, map[string]interface{}{
		"product": product,
	})
}

// CreateProduct handles POST /products requests (admin only)
func (h *ProductHandlers) CreateProduct(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if !isAdminRequest(request) {
		return errorResponse(http.StatusForbidden, "Admin access required")
	}

	var product models.Product
	if err := json.Unmarshal([]byte(request.Body), &product); err != nil {
		return errorResponse(http.StatusBadRequest, "Invalid request body")
	}

	// Validate required fields
	if product.Name == "" || product.SKU == "" || product.CategoryID == "" {
		return errorResponse(http.StatusBadRequest, "Name, SKU, and category_id are required")
	}

	// Set defaults
	product.ID = uuid.New().String()
	if product.Status == "" {
		product.Status = models.ProductStatusActive
	}
	product.CreatedAt = time.Now()
	product.UpdatedAt = time.Now()
	product.Version = 1

	// Ensure price is valid
	if product.Price < 0 {
		return errorResponse(http.StatusBadRequest, "Price cannot be negative")
	}

	// Create the product
	if err := h.db.PutItem(ctx, &product); err != nil {
		if strings.Contains(err.Error(), "ConditionalCheckFailedException") {
			return errorResponse(http.StatusConflict, "Product with this SKU already exists")
		}
		return errorResponse(http.StatusInternalServerError, "Failed to create product")
	}

	return jsonResponse(http.StatusCreated, map[string]interface{}{
		"product": product,
	})
}

// UpdateProduct handles PUT /products/{id} requests (admin only)
func (h *ProductHandlers) UpdateProduct(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if !isAdminRequest(request) {
		return errorResponse(http.StatusForbidden, "Admin access required")
	}

	productID := request.PathParameters["id"]
	if productID == "" {
		return errorResponse(http.StatusBadRequest, "Product ID is required")
	}

	// Parse update data
	var updates map[string]interface{}
	if err := json.Unmarshal([]byte(request.Body), &updates); err != nil {
		return errorResponse(http.StatusBadRequest, "Invalid request body")
	}

	// Get current product for version check
	var currentProduct models.Product
	if err := h.db.GetItem(ctx, productID, &currentProduct); err != nil {
		if err == dynamorm.ErrItemNotFound {
			return errorResponse(http.StatusNotFound, "Product not found")
		}
		return errorResponse(http.StatusInternalServerError, "Failed to fetch product")
	}

	// Build update expression
	update := h.db.Update(productID).
		Set("UpdatedAt", time.Now())

	// Update allowed fields
	if name, ok := updates["name"].(string); ok && name != "" {
		update = update.Set("Name", name)
	}
	if description, ok := updates["description"].(string); ok {
		update = update.Set("Description", description)
	}
	if price, ok := updates["price"].(float64); ok && price >= 0 {
		update = update.Set("Price", int(price))
	}
	if stock, ok := updates["stock"].(float64); ok && stock >= 0 {
		update = update.Set("Stock", int(stock))
	}
	if status, ok := updates["status"].(string); ok {
		update = update.Set("Status", status)
	}
	if featured, ok := updates["featured"].(bool); ok {
		update = update.Set("Featured", featured)
	}

	// Handle arrays and complex types
	if tags, ok := updates["tags"].([]interface{}); ok {
		stringTags := make([]string, len(tags))
		for i, tag := range tags {
			stringTags[i] = tag.(string)
		}
		update = update.Set("Tags", stringTags)
	}

	// Increment version for optimistic locking
	update = update.Add("Version", 1).
		Condition("Version", "=", currentProduct.Version)

	// Execute update
	result, err := update.Execute(ctx, &models.Product{})
	if err != nil {
		if strings.Contains(err.Error(), "ConditionalCheckFailedException") {
			return errorResponse(http.StatusConflict, "Product was modified by another request")
		}
		return errorResponse(http.StatusInternalServerError, "Failed to update product")
	}

	// Get updated product
	var updatedProduct models.Product
	if err := h.db.GetItem(ctx, productID, &updatedProduct); err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to fetch updated product")
	}

	return jsonResponse(http.StatusOK, map[string]interface{}{
		"product": updatedProduct,
	})
}

// SearchProducts handles GET /products/search requests
func (h *ProductHandlers) SearchProducts(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	query := request.QueryStringParameters["q"]
	if query == "" {
		return errorResponse(http.StatusBadRequest, "Search query is required")
	}

	// For a real implementation, you might use Elasticsearch or AWS CloudSearch
	// This is a simple implementation using scan with filters
	var products []models.Product

	result, err := h.db.Scan().
		Filter("Status", "=", models.ProductStatusActive).
		Execute(ctx, &products)

	if err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to search products")
	}

	// Simple text matching (in production, use proper search service)
	searchLower := strings.ToLower(query)
	var matches []models.Product

	for _, product := range products {
		if strings.Contains(strings.ToLower(product.Name), searchLower) ||
			strings.Contains(strings.ToLower(product.Description), searchLower) ||
			strings.Contains(strings.ToLower(product.SKU), searchLower) {
			matches = append(matches, product)
		}

		// Also check tags
		for _, tag := range product.Tags {
			if strings.Contains(strings.ToLower(tag), searchLower) {
				matches = append(matches, product)
				break
			}
		}
	}

	// Sort by relevance (simple implementation - name matches first)
	// In production, use proper scoring algorithm

	return jsonResponse(http.StatusOK, map[string]interface{}{
		"products": matches,
		"metadata": map[string]interface{}{
			"query": query,
			"count": len(matches),
		},
	})
}

// UpdateInventory handles POST /products/{id}/inventory requests (admin only)
func (h *ProductHandlers) UpdateInventory(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if !isAdminRequest(request) {
		return errorResponse(http.StatusForbidden, "Admin access required")
	}

	productID := request.PathParameters["id"]
	if productID == "" {
		return errorResponse(http.StatusBadRequest, "Product ID is required")
	}

	var inventoryUpdate struct {
		Adjustment int    `json:"adjustment"` // Can be positive or negative
		Type       string `json:"type"`       // sale, return, adjustment, restock
		Reason     string `json:"reason"`
	}

	if err := json.Unmarshal([]byte(request.Body), &inventoryUpdate); err != nil {
		return errorResponse(http.StatusBadRequest, "Invalid request body")
	}

	// Get current product
	var product models.Product
	if err := h.db.GetItem(ctx, productID, &product); err != nil {
		if err == dynamorm.ErrItemNotFound {
			return errorResponse(http.StatusNotFound, "Product not found")
		}
		return errorResponse(http.StatusInternalServerError, "Failed to fetch product")
	}

	// Calculate new stock level
	newStock := product.Stock + inventoryUpdate.Adjustment
	if newStock < 0 {
		return errorResponse(http.StatusBadRequest, "Insufficient stock")
	}

	// Update stock with optimistic locking
	update := h.db.Update(productID).
		Set("Stock", newStock).
		Set("UpdatedAt", time.Now()).
		Add("Version", 1).
		Condition("Version", "=", product.Version)

	_, err := update.Execute(ctx, &models.Product{})
	if err != nil {
		if strings.Contains(err.Error(), "ConditionalCheckFailedException") {
			return errorResponse(http.StatusConflict, "Product was modified by another request")
		}
		return errorResponse(http.StatusInternalServerError, "Failed to update inventory")
	}

	// Create inventory movement record
	movement := models.InventoryMovement{
		ID:            uuid.New().String(),
		ProductID:     productID,
		Timestamp:     time.Now(),
		Type:          inventoryUpdate.Type,
		Quantity:      inventoryUpdate.Adjustment,
		ReferenceType: "manual_adjustment",
		Notes:         inventoryUpdate.Reason,
		UserID:        getAdminUserID(request),
		CreatedAt:     time.Now(),
	}

	if err := h.db.PutItem(ctx, &movement); err != nil {
		// Log error but don't fail the request
		fmt.Printf("Failed to create inventory movement record: %v\n", err)
	}

	return jsonResponse(http.StatusOK, map[string]interface{}{
		"product": map[string]interface{}{
			"id":    productID,
			"stock": newStock,
		},
		"movement": movement,
	})
}

// Utility functions

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func isAdminRequest(request events.APIGatewayProxyRequest) bool {
	// In production, validate JWT token or API key
	// This is a simplified check for demo purposes
	return request.Headers["X-Admin-Token"] == "admin-secret-token"
}

func getAdminUserID(request events.APIGatewayProxyRequest) string {
	// In production, extract from JWT token
	return "admin-user"
}

func jsonResponse(statusCode int, body interface{}) (events.APIGatewayProxyResponse, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusInternalServerError,
			Body:       `{"error":"Failed to marshal response"}`,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		}, nil
	}

	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Body:       string(jsonBody),
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	}, nil
}

func errorResponse(statusCode int, message string) (events.APIGatewayProxyResponse, error) {
	return jsonResponse(statusCode, map[string]string{
		"error": message,
	})
}

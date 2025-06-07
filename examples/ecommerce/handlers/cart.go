package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/google/uuid"

	"github.com/example/dynamorm"
	"github.com/example/dynamorm/examples/ecommerce/models"
)

// CartHandler handles shopping cart operations
type CartHandler struct {
	db *dynamorm.DB
}

// NewCartHandler creates a new cart handler
func NewCartHandler() (*CartHandler, error) {
	db, err := dynamorm.New(
		dynamorm.WithLambdaOptimization(),
		dynamorm.WithConnectionPool(10),
		dynamorm.WithRegion("us-east-1"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DynamoDB: %w", err)
	}

	// Register models
	db.Model(&models.Cart{})
	db.Model(&models.Product{})
	db.Model(&models.Customer{})
	db.Model(&models.Inventory{})

	return &CartHandler{db: db}, nil
}

// HandleRequest routes requests to appropriate handlers
func (h *CartHandler) HandleRequest(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	switch request.HTTPMethod {
	case "GET":
		return h.getCart(ctx, request)
	case "POST":
		if request.Path == "/cart/items" {
			return h.addToCart(ctx, request)
		}
		return h.createCart(ctx, request)
	case "PUT":
		return h.updateCartItem(ctx, request)
	case "DELETE":
		if request.PathParameters["itemId"] != "" {
			return h.removeCartItem(ctx, request)
		}
		return h.clearCart(ctx, request)
	default:
		return errorResponse(http.StatusMethodNotAllowed, "Method not allowed"), nil
	}
}

// getCart retrieves the current cart
func (h *CartHandler) getCart(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	sessionID := getSessionID(request)
	if sessionID == "" {
		return errorResponse(http.StatusBadRequest, "Session ID required"), nil
	}

	// Get cart by session ID
	var cart models.Cart
	err := h.db.Model(&models.Cart{}).
		Index("gsi-session").
		Where("SessionID", "=", sessionID).
		First(&cart)

	if err != nil {
		if err == dynamorm.ErrNotFound {
			// Create empty cart
			cart = models.Cart{
				ID:        uuid.New().String(),
				SessionID: sessionID,
				Items:     []models.CartItem{},
				Currency:  "USD",
				ExpiresAt: time.Now().Add(24 * time.Hour),
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}

			// Associate with customer if authenticated
			if customerID := getCustomerID(request); customerID != "" {
				cart.CustomerID = customerID
			}

			if err := h.db.Model(&cart).Create(); err != nil {
				return errorResponse(http.StatusInternalServerError, "Failed to create cart"), nil
			}
		} else {
			return errorResponse(http.StatusInternalServerError, "Failed to fetch cart"), nil
		}
	}

	// Check if cart has expired
	if cart.ExpiresAt.Before(time.Now()) {
		// Cart expired, create new one
		cart = models.Cart{
			ID:         uuid.New().String(),
			SessionID:  sessionID,
			CustomerID: cart.CustomerID, // Keep customer association
			Items:      []models.CartItem{},
			Currency:   "USD",
			ExpiresAt:  time.Now().Add(24 * time.Hour),
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := h.db.Model(&cart).Create(); err != nil {
			return errorResponse(http.StatusInternalServerError, "Failed to create cart"), nil
		}
	}

	// Calculate totals
	h.calculateCartTotals(&cart)

	return successResponse(http.StatusOK, cart), nil
}

// addToCart adds an item to the cart
func (h *CartHandler) addToCart(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	sessionID := getSessionID(request)
	if sessionID == "" {
		return errorResponse(http.StatusBadRequest, "Session ID required"), nil
	}

	// Parse request
	var req struct {
		ProductID string `json:"product_id"`
		VariantID string `json:"variant_id,omitempty"`
		Quantity  int    `json:"quantity"`
	}

	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return errorResponse(http.StatusBadRequest, "Invalid request body"), nil
	}

	// Validate
	if req.ProductID == "" || req.Quantity <= 0 {
		return errorResponse(http.StatusBadRequest, "Invalid product or quantity"), nil
	}

	// Get product
	var product models.Product
	err := h.db.Model(&models.Product{}).
		Where("ID", "=", req.ProductID).
		First(&product)

	if err != nil {
		if err == dynamorm.ErrNotFound {
			return errorResponse(http.StatusNotFound, "Product not found"), nil
		}
		return errorResponse(http.StatusInternalServerError, "Failed to fetch product"), nil
	}

	// Check if product is active
	if product.Status != models.ProductStatusActive {
		return errorResponse(http.StatusBadRequest, "Product is not available"), nil
	}

	// Get variant if specified
	var variant *models.ProductVariant
	if req.VariantID != "" {
		for _, v := range product.Variants {
			if v.ID == req.VariantID {
				variant = &v
				break
			}
		}
		if variant == nil {
			return errorResponse(http.StatusNotFound, "Variant not found"), nil
		}
	}

	// Check stock
	availableStock := product.Stock - product.Reserved
	if variant != nil {
		availableStock = variant.Stock
	}

	if availableStock < req.Quantity {
		return errorResponse(http.StatusBadRequest, fmt.Sprintf("Only %d items available", availableStock)), nil
	}

	// Get or create cart
	var cart models.Cart
	err = h.db.Model(&models.Cart{}).
		Index("gsi-session").
		Where("SessionID", "=", sessionID).
		First(&cart)

	if err != nil {
		if err == dynamorm.ErrNotFound {
			// Create new cart
			cart = models.Cart{
				ID:        uuid.New().String(),
				SessionID: sessionID,
				Items:     []models.CartItem{},
				Currency:  "USD",
				ExpiresAt: time.Now().Add(24 * time.Hour),
				CreatedAt: time.Now(),
			}

			// Associate with customer if authenticated
			if customerID := getCustomerID(request); customerID != "" {
				cart.CustomerID = customerID
			}
		} else {
			return errorResponse(http.StatusInternalServerError, "Failed to fetch cart"), nil
		}
	}

	// Prepare cart item
	price := product.Price
	if variant != nil && variant.Price > 0 {
		price = variant.Price
	}

	cartItem := models.CartItem{
		ProductID: product.ID,
		VariantID: req.VariantID,
		SKU:       product.SKU,
		Name:      product.Name,
		Price:     price,
		Quantity:  req.Quantity,
		Subtotal:  price * req.Quantity,
	}

	if len(product.Images) > 0 {
		cartItem.Image = product.Images[0].URL
	}

	if variant != nil {
		cartItem.SKU = variant.SKU
		cartItem.Name = fmt.Sprintf("%s - %s", product.Name, variant.Name)
		cartItem.Attributes = variant.Attributes
	}

	// Check if item already exists in cart
	itemFound := false
	for i, item := range cart.Items {
		if item.ProductID == cartItem.ProductID && item.VariantID == cartItem.VariantID {
			// Update quantity
			cart.Items[i].Quantity += req.Quantity
			cart.Items[i].Subtotal = cart.Items[i].Price * cart.Items[i].Quantity
			itemFound = true
			break
		}
	}

	if !itemFound {
		cart.Items = append(cart.Items, cartItem)
	}

	// Update cart
	cart.UpdatedAt = time.Now()
	cart.ExpiresAt = time.Now().Add(24 * time.Hour) // Reset expiry

	// Calculate totals
	h.calculateCartTotals(&cart)

	// Save cart
	if err := h.db.Model(&cart).Save(); err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to update cart"), nil
	}

	// Reserve inventory (best effort, don't fail the request)
	go h.reserveInventory(product.ID, req.VariantID, req.Quantity)

	return successResponse(http.StatusOK, map[string]interface{}{
		"cart":    cart,
		"message": "Item added to cart",
	}), nil
}

// updateCartItem updates quantity of an item in cart
func (h *CartHandler) updateCartItem(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	sessionID := getSessionID(request)
	if sessionID == "" {
		return errorResponse(http.StatusBadRequest, "Session ID required"), nil
	}

	itemID := request.PathParameters["itemId"]
	if itemID == "" {
		return errorResponse(http.StatusBadRequest, "Item ID required"), nil
	}

	// Parse request
	var req struct {
		Quantity int `json:"quantity"`
	}

	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return errorResponse(http.StatusBadRequest, "Invalid request body"), nil
	}

	if req.Quantity < 0 {
		return errorResponse(http.StatusBadRequest, "Invalid quantity"), nil
	}

	// Get cart
	var cart models.Cart
	err := h.db.Model(&models.Cart{}).
		Index("gsi-session").
		Where("SessionID", "=", sessionID).
		First(&cart)

	if err != nil {
		if err == dynamorm.ErrNotFound {
			return errorResponse(http.StatusNotFound, "Cart not found"), nil
		}
		return errorResponse(http.StatusInternalServerError, "Failed to fetch cart"), nil
	}

	// Find and update item
	itemFound := false
	newItems := []models.CartItem{}

	for _, item := range cart.Items {
		if item.ProductID == itemID || item.SKU == itemID {
			if req.Quantity > 0 {
				// Check stock
				var product models.Product
				err := h.db.Model(&models.Product{}).
					Where("ID", "=", item.ProductID).
					First(&product)

				if err == nil {
					availableStock := product.Stock - product.Reserved
					if availableStock < req.Quantity {
						return errorResponse(http.StatusBadRequest, fmt.Sprintf("Only %d items available", availableStock)), nil
					}
				}

				item.Quantity = req.Quantity
				item.Subtotal = item.Price * item.Quantity
				newItems = append(newItems, item)
			}
			// If quantity is 0, item is removed
			itemFound = true
		} else {
			newItems = append(newItems, item)
		}
	}

	if !itemFound {
		return errorResponse(http.StatusNotFound, "Item not found in cart"), nil
	}

	// Update cart
	cart.Items = newItems
	cart.UpdatedAt = time.Now()
	cart.ExpiresAt = time.Now().Add(24 * time.Hour) // Reset expiry

	// Calculate totals
	h.calculateCartTotals(&cart)

	// Save cart
	if err := h.db.Model(&cart).Save(); err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to update cart"), nil
	}

	return successResponse(http.StatusOK, cart), nil
}

// removeCartItem removes an item from cart
func (h *CartHandler) removeCartItem(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	sessionID := getSessionID(request)
	if sessionID == "" {
		return errorResponse(http.StatusBadRequest, "Session ID required"), nil
	}

	itemID := request.PathParameters["itemId"]
	if itemID == "" {
		return errorResponse(http.StatusBadRequest, "Item ID required"), nil
	}

	// Get cart
	var cart models.Cart
	err := h.db.Model(&models.Cart{}).
		Index("gsi-session").
		Where("SessionID", "=", sessionID).
		First(&cart)

	if err != nil {
		if err == dynamorm.ErrNotFound {
			return errorResponse(http.StatusNotFound, "Cart not found"), nil
		}
		return errorResponse(http.StatusInternalServerError, "Failed to fetch cart"), nil
	}

	// Remove item
	newItems := []models.CartItem{}
	itemFound := false

	for _, item := range cart.Items {
		if item.ProductID == itemID || item.SKU == itemID {
			itemFound = true
			// Release reserved inventory
			go h.releaseInventory(item.ProductID, item.VariantID, item.Quantity)
		} else {
			newItems = append(newItems, item)
		}
	}

	if !itemFound {
		return errorResponse(http.StatusNotFound, "Item not found in cart"), nil
	}

	// Update cart
	cart.Items = newItems
	cart.UpdatedAt = time.Now()
	cart.ExpiresAt = time.Now().Add(24 * time.Hour) // Reset expiry

	// Calculate totals
	h.calculateCartTotals(&cart)

	// Save cart
	if err := h.db.Model(&cart).Save(); err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to update cart"), nil
	}

	return successResponse(http.StatusOK, cart), nil
}

// clearCart empties the cart
func (h *CartHandler) clearCart(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	sessionID := getSessionID(request)
	if sessionID == "" {
		return errorResponse(http.StatusBadRequest, "Session ID required"), nil
	}

	// Get cart
	var cart models.Cart
	err := h.db.Model(&models.Cart{}).
		Index("gsi-session").
		Where("SessionID", "=", sessionID).
		First(&cart)

	if err != nil {
		if err == dynamorm.ErrNotFound {
			return errorResponse(http.StatusNotFound, "Cart not found"), nil
		}
		return errorResponse(http.StatusInternalServerError, "Failed to fetch cart"), nil
	}

	// Release all reserved inventory
	for _, item := range cart.Items {
		go h.releaseInventory(item.ProductID, item.VariantID, item.Quantity)
	}

	// Clear cart
	cart.Items = []models.CartItem{}
	cart.Subtotal = 0
	cart.Discount = 0
	cart.Tax = 0
	cart.Total = 0
	cart.UpdatedAt = time.Now()
	cart.ExpiresAt = time.Now().Add(24 * time.Hour) // Reset expiry

	// Save cart
	if err := h.db.Model(&cart).Save(); err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to clear cart"), nil
	}

	return successResponse(http.StatusOK, map[string]interface{}{
		"cart":    cart,
		"message": "Cart cleared successfully",
	}), nil
}

// Helper functions

func (h *CartHandler) calculateCartTotals(cart *models.Cart) {
	subtotal := 0
	for _, item := range cart.Items {
		subtotal += item.Subtotal
	}

	cart.Subtotal = subtotal

	// Calculate tax (simple 10% for demo)
	if cart.Subtotal > 0 {
		cart.Tax = cart.Subtotal / 10
	}

	// Calculate total
	cart.Total = cart.Subtotal + cart.Tax - cart.Discount
}

func (h *CartHandler) reserveInventory(productID, variantID string, quantity int) {
	// Update product reserved count
	_ = h.db.Model(&models.Product{}).
		Where("ID", "=", productID).
		Increment("Reserved", quantity)
}

func (h *CartHandler) releaseInventory(productID, variantID string, quantity int) {
	// Update product reserved count
	_ = h.db.Model(&models.Product{}).
		Where("ID", "=", productID).
		Decrement("Reserved", quantity)
}

func getSessionID(request events.APIGatewayProxyRequest) string {
	// First check header
	if sessionID := request.Headers["X-Session-ID"]; sessionID != "" {
		return sessionID
	}

	// Then check cookie
	for _, cookie := range request.Headers["Cookie"] {
		// Parse cookie to find session ID
		// This is simplified - in production use proper cookie parsing
		if len(cookie) > 10 && cookie[:10] == "session_id" {
			return cookie[11:]
		}
	}

	return ""
}

func getCustomerID(request events.APIGatewayProxyRequest) string {
	// Extract from JWT claims or headers
	return request.Headers["X-Customer-ID"]
}

func successResponse(statusCode int, data interface{}) events.APIGatewayProxyResponse {
	body, _ := json.Marshal(map[string]interface{}{
		"success": true,
		"data":    data,
	})

	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Content-Type":                "application/json",
			"Access-Control-Allow-Origin": "*",
		},
		Body: string(body),
	}
}

func errorResponse(statusCode int, message string) events.APIGatewayProxyResponse {
	body, _ := json.Marshal(map[string]interface{}{
		"success": false,
		"error":   message,
	})

	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Content-Type":                "application/json",
			"Access-Control-Allow-Origin": "*",
		},
		Body: string(body),
	}
}

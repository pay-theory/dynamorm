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

// OrderHandlers manages order-related operations
type OrderHandlers struct {
	db          *lambda.OptimizedClient
	cartHandler *CartHandlers
	prodHandler *ProductHandlers
}

// NewOrderHandlers creates a new order handler instance
func NewOrderHandlers(db *lambda.OptimizedClient) *OrderHandlers {
	return &OrderHandlers{
		db:          db,
		cartHandler: NewCartHandlers(db),
		prodHandler: NewProductHandlers(db),
	}
}

// CreateOrder handles POST /orders - creates order from cart
func (h *OrderHandlers) CreateOrder(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	var orderRequest struct {
		CartID          string         `json:"cart_id"`
		CustomerID      string         `json:"customer_id"`
		Email           string         `json:"email"`
		Phone           string         `json:"phone,omitempty"`
		ShippingAddress models.Address `json:"shipping_address"`
		BillingAddress  models.Address `json:"billing_address"`
		PaymentMethod   string         `json:"payment_method"`
		PaymentID       string         `json:"payment_id,omitempty"`
		ShippingMethod  string         `json:"shipping_method"`
		Notes           string         `json:"notes,omitempty"`
	}

	if err := json.Unmarshal([]byte(request.Body), &orderRequest); err != nil {
		return errorResponse(http.StatusBadRequest, "Invalid request body")
	}

	// Validate required fields
	if orderRequest.CartID == "" || orderRequest.CustomerID == "" || orderRequest.Email == "" {
		return errorResponse(http.StatusBadRequest, "cart_id, customer_id, and email are required")
	}

	// Get cart
	var cart models.Cart
	if err := h.db.GetItem(ctx, orderRequest.CartID, &cart); err != nil {
		if err == dynamorm.ErrItemNotFound {
			return errorResponse(http.StatusNotFound, "Cart not found")
		}
		return errorResponse(http.StatusInternalServerError, "Failed to fetch cart")
	}

	// Validate cart
	if len(cart.Items) == 0 {
		return errorResponse(http.StatusBadRequest, "Cart is empty")
	}

	if cart.CustomerID != "" && cart.CustomerID != orderRequest.CustomerID {
		return errorResponse(http.StatusForbidden, "Cart belongs to another customer")
	}

	// Start transaction to create order and update inventory
	tx := h.db.Transaction()

	// Create order
	order := models.Order{
		ID:              uuid.New().String(),
		OrderNumber:     generateOrderNumber(),
		CustomerID:      orderRequest.CustomerID,
		OrderDate:       time.Now(),
		Status:          models.OrderStatusPending,
		StatusDate:      time.Now(),
		Email:           orderRequest.Email,
		Phone:           orderRequest.Phone,
		Items:           convertCartItemsToOrderItems(cart.Items),
		ShippingAddress: orderRequest.ShippingAddress,
		BillingAddress:  orderRequest.BillingAddress,
		Subtotal:        cart.Subtotal,
		ShippingCost:    calculateShipping(orderRequest.ShippingMethod, cart.Items),
		Tax:             calculateTax(cart.Subtotal, orderRequest.ShippingAddress),
		Total:           0, // Will calculate below
		Currency:        cart.Currency,
		PaymentStatus:   models.PaymentStatusPending,
		PaymentMethod:   orderRequest.PaymentMethod,
		PaymentID:       orderRequest.PaymentID,
		ShippingMethod:  orderRequest.ShippingMethod,
		Notes:           orderRequest.Notes,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		Version:         1,
	}

	// Calculate total
	order.Total = order.Subtotal + order.ShippingCost + order.Tax - order.Discount

	// Add order creation to transaction
	tx.Put(&order)

	// Update inventory for each item
	inventoryUpdates := make(map[string]int)
	for _, item := range cart.Items {
		inventoryUpdates[item.ProductID] = item.Quantity
	}

	// Check and update inventory
	for productID, quantity := range inventoryUpdates {
		var product models.Product
		if err := h.db.GetItem(ctx, productID, &product); err != nil {
			tx.Cancel()
			return errorResponse(http.StatusInternalServerError, fmt.Sprintf("Failed to check inventory for product %s", productID))
		}

		availableStock := product.Stock - product.Reserved
		if availableStock < quantity {
			tx.Cancel()
			return errorResponse(http.StatusConflict, fmt.Sprintf("Insufficient stock for product %s. Available: %d, Requested: %d", product.Name, availableStock, quantity))
		}

		// Update product with reserved stock
		tx.Update(productID).
			Add("Reserved", quantity).
			Condition("Version", "=", product.Version)
	}

	// Create inventory movements
	for productID, quantity := range inventoryUpdates {
		movement := models.InventoryMovement{
			ID:            uuid.New().String(),
			ProductID:     productID,
			Timestamp:     time.Now(),
			Type:          "sale",
			Quantity:      -quantity,
			ReferenceType: "order",
			ReferenceID:   order.ID,
			UserID:        orderRequest.CustomerID,
			CreatedAt:     time.Now(),
		}
		tx.Put(&movement)
	}

	// Delete cart after successful order
	tx.Delete(cart.ID)

	// Execute transaction
	if err := tx.Commit(ctx); err != nil {
		if strings.Contains(err.Error(), "ConditionalCheckFailedException") {
			return errorResponse(http.StatusConflict, "Inventory changed during order processing. Please try again.")
		}
		return errorResponse(http.StatusInternalServerError, "Failed to create order")
	}

	// Update customer statistics (non-transactional, best effort)
	go h.updateCustomerStats(ctx, orderRequest.CustomerID, order.Total)

	// Send order confirmation email (async)
	go h.sendOrderConfirmation(order)

	return jsonResponse(http.StatusCreated, map[string]any{
		"order":   order,
		"message": "Order created successfully",
	})
}

// GetOrder handles GET /orders/{id}
func (h *OrderHandlers) GetOrder(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	orderID := request.PathParameters["id"]
	if orderID == "" {
		return errorResponse(http.StatusBadRequest, "Order ID is required")
	}

	var order models.Order
	if err := h.db.GetItem(ctx, orderID, &order); err != nil {
		if err == dynamorm.ErrItemNotFound {
			return errorResponse(http.StatusNotFound, "Order not found")
		}
		return errorResponse(http.StatusInternalServerError, "Failed to fetch order")
	}

	// Check authorization
	customerID := request.Headers["X-Customer-ID"]
	if !isAdminRequest(request) && order.CustomerID != customerID {
		return errorResponse(http.StatusForbidden, "Access denied")
	}

	return jsonResponse(http.StatusOK, map[string]any{
		"order": order,
	})
}

// GetOrderByNumber handles GET /orders/number/{orderNumber}
func (h *OrderHandlers) GetOrderByNumber(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	orderNumber := request.PathParameters["orderNumber"]
	if orderNumber == "" {
		return errorResponse(http.StatusBadRequest, "Order number is required")
	}

	var orders []models.Order
	result, err := h.db.Query("gsi-order-number").
		Where("OrderNumber", "=", orderNumber).
		Limit(1).
		Execute(ctx, &orders)

	if err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to fetch order")
	}

	if len(orders) == 0 {
		return errorResponse(http.StatusNotFound, "Order not found")
	}

	order := orders[0]

	// Check authorization
	customerID := request.Headers["X-Customer-ID"]
	if !isAdminRequest(request) && order.CustomerID != customerID {
		return errorResponse(http.StatusForbidden, "Access denied")
	}

	return jsonResponse(http.StatusOK, map[string]any{
		"order": order,
	})
}

// ListOrders handles GET /orders
func (h *OrderHandlers) ListOrders(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	customerID := request.QueryStringParameters["customer_id"]
	status := request.QueryStringParameters["status"]

	// Pagination
	limit := 20
	if l := request.QueryStringParameters["limit"]; l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	var lastEvaluatedKey map[string]any
	if cursor := request.QueryStringParameters["cursor"]; cursor != "" {
		if err := json.Unmarshal([]byte(cursor), &lastEvaluatedKey); err != nil {
			return errorResponse(http.StatusBadRequest, "Invalid cursor")
		}
	}

	var orders []models.Order
	var nextCursor string

	if customerID != "" {
		// Query by customer
		query := h.db.Query("gsi-customer").
			Where("CustomerID", "=", customerID).
			ScanIndexForward(false). // Most recent first
			Limit(limit)

		if lastEvaluatedKey != nil {
			query = query.StartFrom(lastEvaluatedKey)
		}

		result, err := query.Execute(ctx, &orders)
		if err != nil {
			return errorResponse(http.StatusInternalServerError, "Failed to fetch orders")
		}

		if result.LastEvaluatedKey != nil {
			cursor, _ := json.Marshal(result.LastEvaluatedKey)
			nextCursor = string(cursor)
		}
	} else if status != "" {
		// Query by status
		query := h.db.Query("gsi-status-date").
			Where("Status", "=", status).
			ScanIndexForward(false). // Most recent first
			Limit(limit)

		if lastEvaluatedKey != nil {
			query = query.StartFrom(lastEvaluatedKey)
		}

		result, err := query.Execute(ctx, &orders)
		if err != nil {
			return errorResponse(http.StatusInternalServerError, "Failed to fetch orders")
		}

		if result.LastEvaluatedKey != nil {
			cursor, _ := json.Marshal(result.LastEvaluatedKey)
			nextCursor = string(cursor)
		}
	} else {
		// Admin: scan all orders
		if !isAdminRequest(request) {
			return errorResponse(http.StatusForbidden, "Customer ID is required")
		}

		scan := h.db.Scan().Limit(limit)
		if lastEvaluatedKey != nil {
			scan = scan.StartFrom(lastEvaluatedKey)
		}

		result, err := scan.Execute(ctx, &orders)
		if err != nil {
			return errorResponse(http.StatusInternalServerError, "Failed to fetch orders")
		}

		if result.LastEvaluatedKey != nil {
			cursor, _ := json.Marshal(result.LastEvaluatedKey)
			nextCursor = string(cursor)
		}
	}

	// Build response
	response := map[string]any{
		"orders": orders,
		"metadata": map[string]any{
			"count": len(orders),
			"limit": limit,
		},
	}

	if nextCursor != "" {
		response["metadata"].(map[string]any)["next_cursor"] = nextCursor
	}

	return jsonResponse(http.StatusOK, response)
}

// UpdateOrderStatus handles PUT /orders/{id}/status
func (h *OrderHandlers) UpdateOrderStatus(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if !isAdminRequest(request) {
		return errorResponse(http.StatusForbidden, "Admin access required")
	}

	orderID := request.PathParameters["id"]
	if orderID == "" {
		return errorResponse(http.StatusBadRequest, "Order ID is required")
	}

	var statusUpdate struct {
		Status         string `json:"status"`
		TrackingNumber string `json:"tracking_number,omitempty"`
		Notes          string `json:"notes,omitempty"`
	}

	if err := json.Unmarshal([]byte(request.Body), &statusUpdate); err != nil {
		return errorResponse(http.StatusBadRequest, "Invalid request body")
	}

	// Validate status
	validStatuses := []string{
		models.OrderStatusPending,
		models.OrderStatusProcessing,
		models.OrderStatusShipped,
		models.OrderStatusDelivered,
		models.OrderStatusCancelled,
		models.OrderStatusRefunded,
	}

	isValid := false
	for _, s := range validStatuses {
		if statusUpdate.Status == s {
			isValid = true
			break
		}
	}

	if !isValid {
		return errorResponse(http.StatusBadRequest, "Invalid status")
	}

	// Get current order
	var order models.Order
	if err := h.db.GetItem(ctx, orderID, &order); err != nil {
		if err == dynamorm.ErrItemNotFound {
			return errorResponse(http.StatusNotFound, "Order not found")
		}
		return errorResponse(http.StatusInternalServerError, "Failed to fetch order")
	}

	// Validate status transition
	if !isValidStatusTransition(order.Status, statusUpdate.Status) {
		return errorResponse(http.StatusBadRequest, fmt.Sprintf("Cannot transition from %s to %s", order.Status, statusUpdate.Status))
	}

	// Build update
	update := h.db.Update(orderID).
		Set("Status", statusUpdate.Status).
		Set("StatusDate", time.Now()).
		Set("UpdatedAt", time.Now()).
		Add("Version", 1).
		Condition("Version", "=", order.Version)

	// Add optional fields
	if statusUpdate.TrackingNumber != "" && statusUpdate.Status == models.OrderStatusShipped {
		update = update.Set("TrackingNumber", statusUpdate.TrackingNumber)
	}

	if statusUpdate.Notes != "" {
		existingNotes := order.Notes
		if existingNotes != "" {
			existingNotes += "\n---\n"
		}
		existingNotes += fmt.Sprintf("[%s] %s: %s", time.Now().Format(time.RFC3339), statusUpdate.Status, statusUpdate.Notes)
		update = update.Set("Notes", existingNotes)
	}

	// Handle special status actions
	switch statusUpdate.Status {
	case models.OrderStatusShipped:
		update = update.Set("FulfillmentDate", time.Now())
		// Update payment status if needed
		if order.PaymentStatus == models.PaymentStatusPending {
			update = update.Set("PaymentStatus", models.PaymentStatusPaid)
		}

	case models.OrderStatusCancelled:
		// Release reserved inventory
		tx := h.db.Transaction()
		tx.Update(orderID).
			Set("Status", statusUpdate.Status).
			Set("StatusDate", time.Now()).
			Set("UpdatedAt", time.Now()).
			Add("Version", 1).
			Condition("Version", "=", order.Version)

		// Release inventory reservations
		for _, item := range order.Items {
			tx.Update(item.ProductID).
				Add("Reserved", -item.Quantity)
		}

		if err := tx.Commit(ctx); err != nil {
			return errorResponse(http.StatusInternalServerError, "Failed to cancel order")
		}

		// Get updated order
		if err := h.db.GetItem(ctx, orderID, &order); err != nil {
			return errorResponse(http.StatusInternalServerError, "Failed to fetch updated order")
		}

		return jsonResponse(http.StatusOK, map[string]any{
			"order":   order,
			"message": "Order cancelled and inventory released",
		})

	case models.OrderStatusRefunded:
		update = update.Set("PaymentStatus", models.PaymentStatusRefunded)
	}

	// Execute update
	_, err := update.Execute(ctx, &models.Order{})
	if err != nil {
		if strings.Contains(err.Error(), "ConditionalCheckFailedException") {
			return errorResponse(http.StatusConflict, "Order was modified by another request")
		}
		return errorResponse(http.StatusInternalServerError, "Failed to update order status")
	}

	// Get updated order
	if err := h.db.GetItem(ctx, orderID, &order); err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to fetch updated order")
	}

	// Send status update notification
	go h.sendStatusUpdateNotification(order)

	return jsonResponse(http.StatusOK, map[string]any{
		"order":   order,
		"message": fmt.Sprintf("Order status updated to %s", statusUpdate.Status),
	})
}

// CancelOrder handles POST /orders/{id}/cancel
func (h *OrderHandlers) CancelOrder(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	orderID := request.PathParameters["id"]
	if orderID == "" {
		return errorResponse(http.StatusBadRequest, "Order ID is required")
	}

	var cancelRequest struct {
		Reason string `json:"reason"`
	}

	if err := json.Unmarshal([]byte(request.Body), &cancelRequest); err != nil {
		return errorResponse(http.StatusBadRequest, "Invalid request body")
	}

	// Get order
	var order models.Order
	if err := h.db.GetItem(ctx, orderID, &order); err != nil {
		if err == dynamorm.ErrItemNotFound {
			return errorResponse(http.StatusNotFound, "Order not found")
		}
		return errorResponse(http.StatusInternalServerError, "Failed to fetch order")
	}

	// Check authorization
	customerID := request.Headers["X-Customer-ID"]
	if !isAdminRequest(request) && order.CustomerID != customerID {
		return errorResponse(http.StatusForbidden, "Access denied")
	}

	// Check if order can be cancelled
	if order.Status == models.OrderStatusShipped ||
		order.Status == models.OrderStatusDelivered ||
		order.Status == models.OrderStatusCancelled {
		return errorResponse(http.StatusBadRequest, fmt.Sprintf("Cannot cancel order in %s status", order.Status))
	}

	// Use the UpdateOrderStatus handler with cancel status
	cancelUpdate := map[string]any{
		"status": models.OrderStatusCancelled,
		"notes":  cancelRequest.Reason,
	}

	request.Body = mustMarshal(cancelUpdate)

	// Add admin header to pass authorization
	if request.Headers == nil {
		request.Headers = make(map[string]string)
	}
	request.Headers["X-Admin-Token"] = "admin-secret-token"

	return h.UpdateOrderStatus(ctx, request)
}

// Helper functions

func generateOrderNumber() string {
	// In production, use a more sophisticated method
	timestamp := time.Now().Unix()
	random := uuid.New().String()[:8]
	return fmt.Sprintf("ORD-%d-%s", timestamp, strings.ToUpper(random))
}

func convertCartItemsToOrderItems(cartItems []models.CartItem) []models.OrderItem {
	orderItems := make([]models.OrderItem, len(cartItems))
	for i, item := range cartItems {
		orderItems[i] = models.OrderItem{
			ProductID:  item.ProductID,
			VariantID:  item.VariantID,
			SKU:        item.SKU,
			Name:       item.Name,
			Price:      item.Price,
			Quantity:   item.Quantity,
			Subtotal:   item.Subtotal,
			Image:      item.Image,
			Attributes: item.Attributes,
		}
	}
	return orderItems
}

func calculateShipping(method string, items []models.CartItem) int {
	// Simple shipping calculation
	// In production, integrate with shipping providers
	base := 0
	switch method {
	case "standard":
		base = 599 // $5.99
	case "express":
		base = 1299 // $12.99
	case "overnight":
		base = 2499 // $24.99
	}

	// Add weight-based cost
	// This is simplified - real implementation would calculate actual weight
	itemCount := 0
	for _, item := range items {
		itemCount += item.Quantity
	}

	if itemCount > 5 {
		base += 200 // $2.00 for heavy orders
	}

	return base
}

func calculateTax(subtotal int, address models.Address) int {
	// Simple tax calculation
	// In production, use proper tax calculation service
	taxRate := 0.0

	switch address.State {
	case "CA":
		taxRate = 0.0725 // 7.25%
	case "NY":
		taxRate = 0.08 // 8%
	case "TX":
		taxRate = 0.0625 // 6.25%
	default:
		taxRate = 0.06 // 6% default
	}

	return int(float64(subtotal) * taxRate)
}

func isValidStatusTransition(from, to string) bool {
	validTransitions := map[string][]string{
		models.OrderStatusPending:    {models.OrderStatusProcessing, models.OrderStatusCancelled},
		models.OrderStatusProcessing: {models.OrderStatusShipped, models.OrderStatusCancelled},
		models.OrderStatusShipped:    {models.OrderStatusDelivered},
		models.OrderStatusDelivered:  {models.OrderStatusRefunded},
		models.OrderStatusCancelled:  {}, // Terminal state
		models.OrderStatusRefunded:   {}, // Terminal state
	}

	allowed, exists := validTransitions[from]
	if !exists {
		return false
	}

	for _, status := range allowed {
		if status == to {
			return true
		}
	}

	return false
}

func (h *OrderHandlers) updateCustomerStats(ctx context.Context, customerID string, orderTotal int) {
	// Update customer statistics
	// This is a best-effort operation, failures are logged but don't affect the order

	update := h.db.Update(customerID).
		Add("OrderCount", 1).
		Add("TotalSpent", orderTotal).
		Set("LastOrderDate", time.Now())

	if _, err := update.Execute(ctx, &models.Customer{}); err != nil {
		fmt.Printf("Failed to update customer stats: %v\n", err)
	}
}

func (h *OrderHandlers) sendOrderConfirmation(order models.Order) {
	// In production, integrate with email service
	fmt.Printf("Sending order confirmation email to %s for order %s\n", order.Email, order.OrderNumber)
}

func (h *OrderHandlers) sendStatusUpdateNotification(order models.Order) {
	// In production, integrate with notification service
	fmt.Printf("Sending status update notification for order %s: %s\n", order.OrderNumber, order.Status)
}

func mustMarshal(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

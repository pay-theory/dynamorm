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

// InventoryHandlers manages inventory-related operations
type InventoryHandlers struct {
	db *lambda.OptimizedClient
}

// NewInventoryHandlers creates a new inventory handler instance
func NewInventoryHandlers(db *lambda.OptimizedClient) *InventoryHandlers {
	return &InventoryHandlers{db: db}
}

// GetInventory handles GET /inventory/{productId}
func (h *InventoryHandlers) GetInventory(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	productID := request.PathParameters["productId"]
	locationID := request.QueryStringParameters["location_id"]

	if productID == "" {
		return errorResponse(http.StatusBadRequest, "Product ID is required")
	}

	if locationID == "" {
		locationID = "main" // Default location
	}

	// Composite key for inventory
	inventoryID := fmt.Sprintf("%s#%s", productID, locationID)

	var inventory models.Inventory
	err := h.db.GetItem(ctx, inventoryID, &inventory)
	if err != nil {
		if err == dynamorm.ErrItemNotFound {
			// Return zero inventory if not found
			inventory = models.Inventory{
				ID:         inventoryID,
				ProductID:  productID,
				LocationID: locationID,
				Available:  0,
				Reserved:   0,
				Incoming:   0,
			}
		} else {
			return errorResponse(http.StatusInternalServerError, "Failed to fetch inventory")
		}
	}

	// Get product details for additional info
	var product models.Product
	if err := h.db.GetItem(ctx, productID, &product); err == nil {
		return jsonResponse(http.StatusOK, map[string]interface{}{
			"inventory": inventory,
			"product": map[string]interface{}{
				"id":   product.ID,
				"name": product.Name,
				"sku":  product.SKU,
			},
		})
	}

	return jsonResponse(http.StatusOK, map[string]interface{}{
		"inventory": inventory,
	})
}

// ListInventory handles GET /inventory with filtering
func (h *InventoryHandlers) ListInventory(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if !isAdminRequest(request) {
		return errorResponse(http.StatusForbidden, "Admin access required")
	}

	locationID := request.QueryStringParameters["location_id"]
	lowStock := request.QueryStringParameters["low_stock"] == "true"

	limit := 50
	if l := request.QueryStringParameters["limit"]; l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	var inventories []models.Inventory

	// Scan inventory records
	scan := h.db.Scan().Limit(limit)

	if locationID != "" {
		scan = scan.Filter("LocationID", "=", locationID)
	}

	result, err := scan.Execute(ctx, &inventories)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to fetch inventory")
	}

	// Filter for low stock if requested
	if lowStock {
		filtered := make([]models.Inventory, 0)
		for _, inv := range inventories {
			if inv.Available <= inv.ReorderPoint && inv.ReorderPoint > 0 {
				filtered = append(filtered, inv)
			}
		}
		inventories = filtered
	}

	// Enrich with product details
	enrichedInventory := make([]map[string]interface{}, len(inventories))
	for i, inv := range inventories {
		enrichedInventory[i] = map[string]interface{}{
			"inventory": inv,
		}

		var product models.Product
		if err := h.db.GetItem(ctx, inv.ProductID, &product); err == nil {
			enrichedInventory[i]["product"] = map[string]interface{}{
				"id":   product.ID,
				"name": product.Name,
				"sku":  product.SKU,
			}
		}
	}

	response := map[string]interface{}{
		"inventories": enrichedInventory,
		"metadata": map[string]interface{}{
			"count": len(inventories),
			"limit": limit,
		},
	}

	if result.LastEvaluatedKey != nil {
		cursor, _ := json.Marshal(result.LastEvaluatedKey)
		response["metadata"].(map[string]interface{})["next_cursor"] = string(cursor)
	}

	return jsonResponse(http.StatusOK, response)
}

// UpdateInventory handles PUT /inventory/{productId}
func (h *InventoryHandlers) UpdateInventory(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if !isAdminRequest(request) {
		return errorResponse(http.StatusForbidden, "Admin access required")
	}

	productID := request.PathParameters["productId"]
	if productID == "" {
		return errorResponse(http.StatusBadRequest, "Product ID is required")
	}

	var update struct {
		LocationID      string `json:"location_id"`
		Available       *int   `json:"available,omitempty"`
		Reserved        *int   `json:"reserved,omitempty"`
		Incoming        *int   `json:"incoming,omitempty"`
		ReorderPoint    *int   `json:"reorder_point,omitempty"`
		ReorderQuantity *int   `json:"reorder_quantity,omitempty"`
	}

	if err := json.Unmarshal([]byte(request.Body), &update); err != nil {
		return errorResponse(http.StatusBadRequest, "Invalid request body")
	}

	if update.LocationID == "" {
		update.LocationID = "main"
	}

	inventoryID := fmt.Sprintf("%s#%s", productID, update.LocationID)

	// Get current inventory
	var inventory models.Inventory
	err := h.db.GetItem(ctx, inventoryID, &inventory)
	if err != nil && err != dynamorm.ErrItemNotFound {
		return errorResponse(http.StatusInternalServerError, "Failed to fetch inventory")
	}

	// If doesn't exist, create new
	if err == dynamorm.ErrItemNotFound {
		inventory = models.Inventory{
			ID:         inventoryID,
			ProductID:  productID,
			LocationID: update.LocationID,
			Version:    1,
		}
	}

	// Build update expression
	updateExpr := h.db.Update(inventoryID).
		Set("UpdatedAt", time.Now())

	// Track changes for movement records
	movements := []models.InventoryMovement{}

	if update.Available != nil {
		oldAvailable := inventory.Available
		updateExpr = updateExpr.Set("Available", *update.Available)

		if oldAvailable != *update.Available {
			movement := models.InventoryMovement{
				ID:            uuid.New().String(),
				ProductID:     productID,
				LocationID:    update.LocationID,
				Timestamp:     time.Now(),
				Type:          "adjustment",
				Quantity:      *update.Available - oldAvailable,
				ReferenceType: "manual",
				ReferenceID:   "admin-update",
				UserID:        getAdminUserID(request),
				CreatedAt:     time.Now(),
			}
			movements = append(movements, movement)
		}
	}

	if update.Reserved != nil {
		updateExpr = updateExpr.Set("Reserved", *update.Reserved)
	}

	if update.Incoming != nil {
		updateExpr = updateExpr.Set("Incoming", *update.Incoming)
		if *update.Incoming > 0 {
			updateExpr = updateExpr.Set("LastRestocked", time.Now())
		}
	}

	if update.ReorderPoint != nil {
		updateExpr = updateExpr.Set("ReorderPoint", *update.ReorderPoint)
	}

	if update.ReorderQuantity != nil {
		updateExpr = updateExpr.Set("ReorderQuantity", *update.ReorderQuantity)
	}

	// Use optimistic locking
	updateExpr = updateExpr.
		Add("Version", 1).
		Condition("Version", "=", inventory.Version)

	// Execute update
	_, err = updateExpr.Execute(ctx, &models.Inventory{})
	if err != nil {
		// If item doesn't exist, create it
		if strings.Contains(err.Error(), "ConditionalCheckFailedException") && inventory.Version == 1 {
			inventory.UpdatedAt = time.Now()
			if update.Available != nil {
				inventory.Available = *update.Available
			}
			if update.Reserved != nil {
				inventory.Reserved = *update.Reserved
			}
			if update.Incoming != nil {
				inventory.Incoming = *update.Incoming
			}
			if update.ReorderPoint != nil {
				inventory.ReorderPoint = *update.ReorderPoint
			}
			if update.ReorderQuantity != nil {
				inventory.ReorderQuantity = *update.ReorderQuantity
			}

			if err := h.db.PutItem(ctx, &inventory); err != nil {
				return errorResponse(http.StatusInternalServerError, "Failed to create inventory record")
			}
		} else {
			return errorResponse(http.StatusConflict, "Inventory was modified by another request")
		}
	}

	// Create movement records
	for _, movement := range movements {
		if err := h.db.PutItem(ctx, &movement); err != nil {
			// Log but don't fail
			fmt.Printf("Failed to create movement record: %v\n", err)
		}
	}

	// Get updated inventory
	if err := h.db.GetItem(ctx, inventoryID, &inventory); err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to fetch updated inventory")
	}

	return jsonResponse(http.StatusOK, map[string]interface{}{
		"inventory": inventory,
		"movements": movements,
	})
}

// AdjustInventory handles POST /inventory/{productId}/adjust
func (h *InventoryHandlers) AdjustInventory(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if !isAdminRequest(request) {
		return errorResponse(http.StatusForbidden, "Admin access required")
	}

	productID := request.PathParameters["productId"]
	if productID == "" {
		return errorResponse(http.StatusBadRequest, "Product ID is required")
	}

	var adjustment struct {
		LocationID string `json:"location_id"`
		Quantity   int    `json:"quantity"` // Positive or negative
		Type       string `json:"type"`     // restock, sale, damage, loss, return
		Reason     string `json:"reason"`
		Reference  string `json:"reference,omitempty"`
	}

	if err := json.Unmarshal([]byte(request.Body), &adjustment); err != nil {
		return errorResponse(http.StatusBadRequest, "Invalid request body")
	}

	if adjustment.LocationID == "" {
		adjustment.LocationID = "main"
	}

	if adjustment.Type == "" {
		return errorResponse(http.StatusBadRequest, "Adjustment type is required")
	}

	inventoryID := fmt.Sprintf("%s#%s", productID, adjustment.LocationID)

	// Start transaction for atomic update
	tx := h.db.Transaction()

	// Get current inventory with lock
	var inventory models.Inventory
	err := h.db.GetItem(ctx, inventoryID, &inventory)
	if err != nil && err != dynamorm.ErrItemNotFound {
		return errorResponse(http.StatusInternalServerError, "Failed to fetch inventory")
	}

	// Calculate new available quantity
	newAvailable := inventory.Available + adjustment.Quantity
	if newAvailable < 0 {
		return errorResponse(http.StatusBadRequest, fmt.Sprintf("Insufficient inventory. Available: %d, Requested adjustment: %d", inventory.Available, adjustment.Quantity))
	}

	// Update inventory
	if err == dynamorm.ErrItemNotFound {
		// Create new inventory record
		inventory = models.Inventory{
			ID:         inventoryID,
			ProductID:  productID,
			LocationID: adjustment.LocationID,
			Available:  adjustment.Quantity,
			UpdatedAt:  time.Now(),
			Version:    1,
		}
		tx.Put(&inventory)
	} else {
		// Update existing
		tx.Update(inventoryID).
			Set("Available", newAvailable).
			Set("UpdatedAt", time.Now()).
			Add("Version", 1).
			Condition("Version", "=", inventory.Version)
	}

	// Create movement record
	movement := models.InventoryMovement{
		ID:            uuid.New().String(),
		ProductID:     productID,
		LocationID:    adjustment.LocationID,
		Timestamp:     time.Now(),
		Type:          adjustment.Type,
		Quantity:      adjustment.Quantity,
		ReferenceType: "adjustment",
		ReferenceID:   adjustment.Reference,
		Notes:         adjustment.Reason,
		UserID:        getAdminUserID(request),
		CreatedAt:     time.Now(),
	}
	tx.Put(&movement)

	// Execute transaction
	if err := tx.Commit(ctx); err != nil {
		if strings.Contains(err.Error(), "ConditionalCheckFailedException") {
			return errorResponse(http.StatusConflict, "Inventory was modified by another request")
		}
		return errorResponse(http.StatusInternalServerError, "Failed to adjust inventory")
	}

	// Get updated inventory
	if err := h.db.GetItem(ctx, inventoryID, &inventory); err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to fetch updated inventory")
	}

	return jsonResponse(http.StatusOK, map[string]interface{}{
		"inventory": inventory,
		"movement":  movement,
		"message":   fmt.Sprintf("Inventory adjusted by %d", adjustment.Quantity),
	})
}

// TransferInventory handles POST /inventory/transfer
func (h *InventoryHandlers) TransferInventory(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if !isAdminRequest(request) {
		return errorResponse(http.StatusForbidden, "Admin access required")
	}

	var transfer struct {
		ProductID    string `json:"product_id"`
		FromLocation string `json:"from_location"`
		ToLocation   string `json:"to_location"`
		Quantity     int    `json:"quantity"`
		Reason       string `json:"reason"`
	}

	if err := json.Unmarshal([]byte(request.Body), &transfer); err != nil {
		return errorResponse(http.StatusBadRequest, "Invalid request body")
	}

	// Validate
	if transfer.ProductID == "" || transfer.FromLocation == "" || transfer.ToLocation == "" {
		return errorResponse(http.StatusBadRequest, "product_id, from_location, and to_location are required")
	}

	if transfer.Quantity <= 0 {
		return errorResponse(http.StatusBadRequest, "Quantity must be positive")
	}

	if transfer.FromLocation == transfer.ToLocation {
		return errorResponse(http.StatusBadRequest, "Source and destination locations must be different")
	}

	fromInventoryID := fmt.Sprintf("%s#%s", transfer.ProductID, transfer.FromLocation)
	toInventoryID := fmt.Sprintf("%s#%s", transfer.ProductID, transfer.ToLocation)

	// Start transaction
	tx := h.db.Transaction()

	// Get source inventory
	var fromInventory models.Inventory
	if err := h.db.GetItem(ctx, fromInventoryID, &fromInventory); err != nil {
		if err == dynamorm.ErrItemNotFound {
			return errorResponse(http.StatusNotFound, "Source inventory not found")
		}
		return errorResponse(http.StatusInternalServerError, "Failed to fetch source inventory")
	}

	// Check available quantity
	if fromInventory.Available < transfer.Quantity {
		return errorResponse(http.StatusBadRequest, fmt.Sprintf("Insufficient inventory at source. Available: %d, Requested: %d", fromInventory.Available, transfer.Quantity))
	}

	// Update source inventory
	tx.Update(fromInventoryID).
		Add("Available", -transfer.Quantity).
		Set("UpdatedAt", time.Now()).
		Add("Version", 1).
		Condition("Version", "=", fromInventory.Version)

	// Get or create destination inventory
	var toInventory models.Inventory
	err := h.db.GetItem(ctx, toInventoryID, &toInventory)
	if err != nil && err != dynamorm.ErrItemNotFound {
		return errorResponse(http.StatusInternalServerError, "Failed to fetch destination inventory")
	}

	if err == dynamorm.ErrItemNotFound {
		// Create new destination inventory
		toInventory = models.Inventory{
			ID:         toInventoryID,
			ProductID:  transfer.ProductID,
			LocationID: transfer.ToLocation,
			Available:  transfer.Quantity,
			UpdatedAt:  time.Now(),
			Version:    1,
		}
		tx.Put(&toInventory)
	} else {
		// Update existing destination inventory
		tx.Update(toInventoryID).
			Add("Available", transfer.Quantity).
			Set("UpdatedAt", time.Now()).
			Add("Version", 1).
			Condition("Version", "=", toInventory.Version)
	}

	// Create movement records
	transferID := uuid.New().String()

	// Outgoing movement
	outMovement := models.InventoryMovement{
		ID:            uuid.New().String(),
		ProductID:     transfer.ProductID,
		LocationID:    transfer.FromLocation,
		Timestamp:     time.Now(),
		Type:          "transfer",
		Quantity:      -transfer.Quantity,
		ReferenceType: "transfer",
		ReferenceID:   transferID,
		Notes:         fmt.Sprintf("Transfer to %s: %s", transfer.ToLocation, transfer.Reason),
		UserID:        getAdminUserID(request),
		CreatedAt:     time.Now(),
	}
	tx.Put(&outMovement)

	// Incoming movement
	inMovement := models.InventoryMovement{
		ID:            uuid.New().String(),
		ProductID:     transfer.ProductID,
		LocationID:    transfer.ToLocation,
		Timestamp:     time.Now(),
		Type:          "transfer",
		Quantity:      transfer.Quantity,
		ReferenceType: "transfer",
		ReferenceID:   transferID,
		Notes:         fmt.Sprintf("Transfer from %s: %s", transfer.FromLocation, transfer.Reason),
		UserID:        getAdminUserID(request),
		CreatedAt:     time.Now(),
	}
	tx.Put(&inMovement)

	// Execute transaction
	if err := tx.Commit(ctx); err != nil {
		if strings.Contains(err.Error(), "ConditionalCheckFailedException") {
			return errorResponse(http.StatusConflict, "Inventory was modified during transfer")
		}
		return errorResponse(http.StatusInternalServerError, "Failed to complete transfer")
	}

	return jsonResponse(http.StatusOK, map[string]interface{}{
		"transfer_id": transferID,
		"from": map[string]interface{}{
			"location":      transfer.FromLocation,
			"new_available": fromInventory.Available - transfer.Quantity,
		},
		"to": map[string]interface{}{
			"location":      transfer.ToLocation,
			"new_available": toInventory.Available + transfer.Quantity,
		},
		"quantity": transfer.Quantity,
		"message":  "Transfer completed successfully",
	})
}

// GetInventoryMovements handles GET /inventory/{productId}/movements
func (h *InventoryHandlers) GetInventoryMovements(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	productID := request.PathParameters["productId"]
	if productID == "" {
		return errorResponse(http.StatusBadRequest, "Product ID is required")
	}

	// Parse query parameters
	startDate := request.QueryStringParameters["start_date"]
	endDate := request.QueryStringParameters["end_date"]
	movementType := request.QueryStringParameters["type"]
	locationID := request.QueryStringParameters["location_id"]

	limit := 50
	if l := request.QueryStringParameters["limit"]; l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	// Query movements for product
	query := h.db.Query("gsi-product").
		Where("ProductID", "=", productID).
		ScanIndexForward(false). // Most recent first
		Limit(limit)

	// Add date range if specified
	if startDate != "" {
		startTime, err := time.Parse(time.RFC3339, startDate)
		if err == nil {
			query = query.Where("Timestamp", ">=", startTime)
		}
	}

	if endDate != "" {
		endTime, err := time.Parse(time.RFC3339, endDate)
		if err == nil {
			query = query.Where("Timestamp", "<=", endTime)
		}
	}

	var movements []models.InventoryMovement
	result, err := query.Execute(ctx, &movements)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to fetch movements")
	}

	// Filter by type and location if specified
	if movementType != "" || locationID != "" {
		filtered := make([]models.InventoryMovement, 0)
		for _, movement := range movements {
			if movementType != "" && movement.Type != movementType {
				continue
			}
			if locationID != "" && movement.LocationID != locationID {
				continue
			}
			filtered = append(filtered, movement)
		}
		movements = filtered
	}

	// Calculate summary statistics
	stats := map[string]int{
		"total_in":  0,
		"total_out": 0,
		"net":       0,
	}

	for _, movement := range movements {
		if movement.Quantity > 0 {
			stats["total_in"] += movement.Quantity
		} else {
			stats["total_out"] += -movement.Quantity
		}
		stats["net"] += movement.Quantity
	}

	response := map[string]interface{}{
		"movements":  movements,
		"statistics": stats,
		"metadata": map[string]interface{}{
			"product_id": productID,
			"count":      len(movements),
			"limit":      limit,
		},
	}

	if result.LastEvaluatedKey != nil {
		cursor, _ := json.Marshal(result.LastEvaluatedKey)
		response["metadata"].(map[string]interface{})["next_cursor"] = string(cursor)
	}

	return jsonResponse(http.StatusOK, response)
}

// BulkUpdateInventory handles POST /inventory/bulk-update
func (h *InventoryHandlers) BulkUpdateInventory(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if !isAdminRequest(request) {
		return errorResponse(http.StatusForbidden, "Admin access required")
	}

	var bulkUpdate struct {
		Updates []struct {
			ProductID  string `json:"product_id"`
			LocationID string `json:"location_id"`
			Available  int    `json:"available"`
			Type       string `json:"type"`
			Reason     string `json:"reason"`
		} `json:"updates"`
	}

	if err := json.Unmarshal([]byte(request.Body), &bulkUpdate); err != nil {
		return errorResponse(http.StatusBadRequest, "Invalid request body")
	}

	if len(bulkUpdate.Updates) == 0 {
		return errorResponse(http.StatusBadRequest, "No updates provided")
	}

	if len(bulkUpdate.Updates) > 25 {
		return errorResponse(http.StatusBadRequest, "Maximum 25 updates allowed per request")
	}

	// Process updates in transaction batches (DynamoDB limit: 25 items per transaction)
	successful := 0
	failed := 0
	errors := []string{}

	tx := h.db.Transaction()
	batchCount := 0

	for i, update := range bulkUpdate.Updates {
		if update.LocationID == "" {
			update.LocationID = "main"
		}

		inventoryID := fmt.Sprintf("%s#%s", update.ProductID, update.LocationID)

		// Get current inventory
		var inventory models.Inventory
		err := h.db.GetItem(ctx, inventoryID, &inventory)

		if err != nil && err != dynamorm.ErrItemNotFound {
			errors = append(errors, fmt.Sprintf("Product %s: Failed to fetch inventory", update.ProductID))
			failed++
			continue
		}

		if err == dynamorm.ErrItemNotFound {
			// Create new inventory
			inventory = models.Inventory{
				ID:         inventoryID,
				ProductID:  update.ProductID,
				LocationID: update.LocationID,
				Available:  update.Available,
				UpdatedAt:  time.Now(),
				Version:    1,
			}
			tx.Put(&inventory)
		} else {
			// Update existing
			tx.Update(inventoryID).
				Set("Available", update.Available).
				Set("UpdatedAt", time.Now()).
				Add("Version", 1).
				Condition("Version", "=", inventory.Version)
		}

		// Add movement record
		movement := models.InventoryMovement{
			ID:            uuid.New().String(),
			ProductID:     update.ProductID,
			LocationID:    update.LocationID,
			Timestamp:     time.Now(),
			Type:          update.Type,
			Quantity:      update.Available - inventory.Available,
			ReferenceType: "bulk_update",
			ReferenceID:   fmt.Sprintf("bulk-%s", time.Now().Format("20060102-150405")),
			Notes:         update.Reason,
			UserID:        getAdminUserID(request),
			CreatedAt:     time.Now(),
		}
		tx.Put(&movement)

		batchCount += 2 // inventory + movement

		// Execute batch if at limit or last item
		if batchCount >= 20 || i == len(bulkUpdate.Updates)-1 {
			if err := tx.Commit(ctx); err != nil {
				errors = append(errors, fmt.Sprintf("Batch failed: %v", err))
				failed += (batchCount / 2)
			} else {
				successful += (batchCount / 2)
			}

			// Start new transaction for next batch
			if i < len(bulkUpdate.Updates)-1 {
				tx = h.db.Transaction()
				batchCount = 0
			}
		}
	}

	status := http.StatusOK
	if failed > 0 && successful == 0 {
		status = http.StatusInternalServerError
	} else if failed > 0 {
		status = http.StatusPartialContent
	}

	return jsonResponse(status, map[string]interface{}{
		"successful": successful,
		"failed":     failed,
		"total":      len(bulkUpdate.Updates),
		"errors":     errors,
	})
}

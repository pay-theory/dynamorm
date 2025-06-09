package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yourusername/dynamorm/examples/ecommerce/handlers"
	"github.com/yourusername/dynamorm/examples/ecommerce/models"
	"github.com/yourusername/dynamorm/lambda"
)

var (
	testDB           *lambda.OptimizedClient
	productHandler   *handlers.ProductHandlers
	cartHandler      *handlers.CartHandlers
	orderHandler     *handlers.OrderHandlers
	inventoryHandler *handlers.InventoryHandlers
)

func TestMain(m *testing.M) {
	// Setup test database
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithEndpointResolver(aws.EndpointResolverFunc(
			func(service, region string) (aws.Endpoint, error) {
				if service == dynamodb.ServiceID {
					return aws.Endpoint{
						URL: "http://localhost:8000",
					}, nil
				}
				return aws.Endpoint{}, &aws.EndpointNotFoundError{}
			},
		)),
	)
	if err != nil {
		panic(fmt.Sprintf("Failed to load config: %v", err))
	}

	// Initialize DynamORM with Lambda optimizations
	db, err := lambda.NewOptimizedClient(cfg, "ecommerce_test", lambda.Options{
		ConnectionPoolSize: 10,
		EnableCompression:  true,
		CacheSize:          100,
		PrewarmConnections: 5,
	})
	if err != nil {
		panic(fmt.Sprintf("Failed to create database client: %v", err))
	}

	testDB = db

	// Initialize handlers
	productHandler = handlers.NewProductHandlers(testDB)
	cartHandler = handlers.NewCartHandlers(testDB)
	orderHandler = handlers.NewOrderHandlers(testDB)
	inventoryHandler = handlers.NewInventoryHandlers(testDB)

	// Create test tables
	if err := createTestTables(); err != nil {
		panic(fmt.Sprintf("Failed to create test tables: %v", err))
	}

	// Run tests
	code := m.Run()

	// Cleanup
	if err := cleanupTestTables(); err != nil {
		fmt.Printf("Failed to cleanup test tables: %v\n", err)
	}

	os.Exit(code)
}

func TestFullPurchaseFlow(t *testing.T) {
	ctx := context.Background()

	// Step 1: Create test products
	products := createTestProducts(t, ctx)
	require.Len(t, products, 3, "Should create 3 test products")

	// Step 2: Browse products
	t.Run("Browse Products", func(t *testing.T) {
		// List all products
		req := events.APIGatewayProxyRequest{
			HTTPMethod: "GET",
			Path:       "/products",
			QueryStringParameters: map[string]string{
				"limit": "10",
			},
		}

		resp, err := productHandler.ListProducts(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]any
		err = json.Unmarshal([]byte(resp.Body), &result)
		require.NoError(t, err)

		productsResp := result["products"].([]any)
		assert.GreaterOrEqual(t, len(productsResp), 3)
	})

	// Step 3: Get product details
	t.Run("Get Product Details", func(t *testing.T) {
		req := events.APIGatewayProxyRequest{
			HTTPMethod: "GET",
			Path:       fmt.Sprintf("/products/%s", products[0].ID),
			PathParameters: map[string]string{
				"id": products[0].ID,
			},
		}

		resp, err := productHandler.GetProduct(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]any
		err = json.Unmarshal([]byte(resp.Body), &result)
		require.NoError(t, err)

		product := result["product"].(map[string]any)
		assert.Equal(t, products[0].Name, product["name"])
	})

	// Step 4: Create cart and add items
	var cartID string
	customerID := uuid.New().String()

	t.Run("Create Cart and Add Items", func(t *testing.T) {
		// Create cart
		cartReq := map[string]any{
			"customer_id": customerID,
			"currency":    "USD",
		}

		req := events.APIGatewayProxyRequest{
			HTTPMethod: "POST",
			Path:       "/carts",
			Body:       mustMarshal(cartReq),
		}

		resp, err := cartHandler.CreateCart(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var result map[string]any
		err = json.Unmarshal([]byte(resp.Body), &result)
		require.NoError(t, err)

		cart := result["cart"].(map[string]any)
		cartID = cart["id"].(string)
		assert.NotEmpty(t, cartID)

		// Add items to cart
		for i, product := range products[:2] {
			addItemReq := map[string]any{
				"product_id": product.ID,
				"quantity":   i + 1,
			}

			req := events.APIGatewayProxyRequest{
				HTTPMethod: "POST",
				Path:       fmt.Sprintf("/carts/%s/items", cartID),
				PathParameters: map[string]string{
					"id": cartID,
				},
				Body: mustMarshal(addItemReq),
			}

			resp, err := cartHandler.AddItem(ctx, req)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		}
	})

	// Step 5: View cart
	t.Run("View Cart", func(t *testing.T) {
		req := events.APIGatewayProxyRequest{
			HTTPMethod: "GET",
			Path:       fmt.Sprintf("/carts/%s", cartID),
			PathParameters: map[string]string{
				"id": cartID,
			},
		}

		resp, err := cartHandler.GetCart(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]any
		err = json.Unmarshal([]byte(resp.Body), &result)
		require.NoError(t, err)

		cart := result["cart"].(map[string]any)
		items := cart["items"].([]any)
		assert.Len(t, items, 2)
		assert.Greater(t, cart["total"].(float64), 0.0)
	})

	// Step 6: Create order from cart
	var orderID string
	t.Run("Create Order", func(t *testing.T) {
		orderReq := map[string]any{
			"cart_id":     cartID,
			"customer_id": customerID,
			"email":       "test@example.com",
			"phone":       "+1234567890",
			"shipping_address": map[string]any{
				"first_name":  "John",
				"last_name":   "Doe",
				"address1":    "123 Main St",
				"city":        "San Francisco",
				"state":       "CA",
				"postal_code": "94102",
				"country":     "US",
			},
			"billing_address": map[string]any{
				"first_name":  "John",
				"last_name":   "Doe",
				"address1":    "123 Main St",
				"city":        "San Francisco",
				"state":       "CA",
				"postal_code": "94102",
				"country":     "US",
			},
			"payment_method":  "credit_card",
			"payment_id":      "pay_" + uuid.New().String(),
			"shipping_method": "standard",
		}

		req := events.APIGatewayProxyRequest{
			HTTPMethod: "POST",
			Path:       "/orders",
			Body:       mustMarshal(orderReq),
		}

		resp, err := orderHandler.CreateOrder(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var result map[string]any
		err = json.Unmarshal([]byte(resp.Body), &result)
		require.NoError(t, err)

		order := result["order"].(map[string]any)
		orderID = order["id"].(string)
		assert.NotEmpty(t, orderID)
		assert.Equal(t, models.OrderStatusPending, order["status"])
		assert.Greater(t, order["total"].(float64), 0.0)
	})

	// Step 7: Verify inventory was updated
	t.Run("Verify Inventory Updated", func(t *testing.T) {
		for _, product := range products[:2] {
			req := events.APIGatewayProxyRequest{
				HTTPMethod: "GET",
				Path:       fmt.Sprintf("/inventory/%s", product.ID),
				PathParameters: map[string]string{
					"productId": product.ID,
				},
			}

			resp, err := inventoryHandler.GetInventory(ctx, req)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result map[string]any
			err = json.Unmarshal([]byte(resp.Body), &result)
			require.NoError(t, err)

			inventory := result["inventory"].(map[string]any)
			// Check that reserved quantity increased
			assert.Greater(t, inventory["reserved"].(float64), 0.0)
		}
	})

	// Step 8: Update order status
	t.Run("Update Order Status", func(t *testing.T) {
		// Process order
		statusUpdate := map[string]any{
			"status": models.OrderStatusProcessing,
			"notes":  "Payment confirmed, preparing for shipment",
		}

		req := events.APIGatewayProxyRequest{
			HTTPMethod: "PUT",
			Path:       fmt.Sprintf("/orders/%s/status", orderID),
			PathParameters: map[string]string{
				"id": orderID,
			},
			Headers: map[string]string{
				"X-Admin-Token": "admin-secret-token",
			},
			Body: mustMarshal(statusUpdate),
		}

		resp, err := orderHandler.UpdateOrderStatus(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Ship order
		statusUpdate = map[string]any{
			"status":          models.OrderStatusShipped,
			"tracking_number": "TRACK123456",
			"notes":           "Order shipped via FedEx",
		}

		req.Body = mustMarshal(statusUpdate)
		resp, err = orderHandler.UpdateOrderStatus(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]any
		err = json.Unmarshal([]byte(resp.Body), &result)
		require.NoError(t, err)

		order := result["order"].(map[string]any)
		assert.Equal(t, models.OrderStatusShipped, order["status"])
		assert.Equal(t, "TRACK123456", order["tracking_number"])
	})

	// Step 9: Test order listing
	t.Run("List Customer Orders", func(t *testing.T) {
		req := events.APIGatewayProxyRequest{
			HTTPMethod: "GET",
			Path:       "/orders",
			QueryStringParameters: map[string]string{
				"customer_id": customerID,
			},
		}

		resp, err := orderHandler.ListOrders(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]any
		err = json.Unmarshal([]byte(resp.Body), &result)
		require.NoError(t, err)

		orders := result["orders"].([]any)
		assert.GreaterOrEqual(t, len(orders), 1)
	})
}

func TestInventoryManagement(t *testing.T) {
	ctx := context.Background()

	// Create test product
	product := createTestProduct(t, ctx, "Inventory Test Product", 10000, 50)

	t.Run("Adjust Inventory", func(t *testing.T) {
		adjustment := map[string]any{
			"location_id": "main",
			"quantity":    10,
			"type":        "restock",
			"reason":      "New shipment arrived",
		}

		req := events.APIGatewayProxyRequest{
			HTTPMethod: "POST",
			Path:       fmt.Sprintf("/inventory/%s/adjust", product.ID),
			PathParameters: map[string]string{
				"productId": product.ID,
			},
			Headers: map[string]string{
				"X-Admin-Token": "admin-secret-token",
			},
			Body: mustMarshal(adjustment),
		}

		resp, err := inventoryHandler.AdjustInventory(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]any
		err = json.Unmarshal([]byte(resp.Body), &result)
		require.NoError(t, err)

		inventory := result["inventory"].(map[string]any)
		assert.Equal(t, 60.0, inventory["available"].(float64))
	})

	t.Run("Transfer Inventory", func(t *testing.T) {
		// First, create inventory at source location
		setupInventory(t, ctx, product.ID, "warehouse", 100)

		transfer := map[string]any{
			"product_id":    product.ID,
			"from_location": "warehouse",
			"to_location":   "store1",
			"quantity":      30,
			"reason":        "Store replenishment",
		}

		req := events.APIGatewayProxyRequest{
			HTTPMethod: "POST",
			Path:       "/inventory/transfer",
			Headers: map[string]string{
				"X-Admin-Token": "admin-secret-token",
			},
			Body: mustMarshal(transfer),
		}

		resp, err := inventoryHandler.TransferInventory(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]any
		err = json.Unmarshal([]byte(resp.Body), &result)
		require.NoError(t, err)

		from := result["from"].(map[string]any)
		to := result["to"].(map[string]any)

		assert.Equal(t, 70.0, from["new_available"].(float64))
		assert.Equal(t, 30.0, to["new_available"].(float64))
	})

	t.Run("View Inventory Movements", func(t *testing.T) {
		req := events.APIGatewayProxyRequest{
			HTTPMethod: "GET",
			Path:       fmt.Sprintf("/inventory/%s/movements", product.ID),
			PathParameters: map[string]string{
				"productId": product.ID,
			},
			QueryStringParameters: map[string]string{
				"limit": "20",
			},
		}

		resp, err := inventoryHandler.GetInventoryMovements(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]any
		err = json.Unmarshal([]byte(resp.Body), &result)
		require.NoError(t, err)

		movements := result["movements"].([]any)
		assert.GreaterOrEqual(t, len(movements), 2) // At least adjustment and transfer

		stats := result["statistics"].(map[string]any)
		assert.Greater(t, stats["total_in"].(float64), 0.0)
		assert.Greater(t, stats["total_out"].(float64), 0.0)
	})
}

func TestCartOperations(t *testing.T) {
	ctx := context.Background()

	// Create test products
	products := createTestProducts(t, ctx)

	t.Run("Cart TTL Expiration", func(t *testing.T) {
		// Create cart with short TTL
		cartReq := map[string]any{
			"session_id": uuid.New().String(),
			"currency":   "USD",
			"ttl_hours":  1, // 1 hour TTL
		}

		req := events.APIGatewayProxyRequest{
			HTTPMethod: "POST",
			Path:       "/carts",
			Body:       mustMarshal(cartReq),
		}

		resp, err := cartHandler.CreateCart(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var result map[string]any
		err = json.Unmarshal([]byte(resp.Body), &result)
		require.NoError(t, err)

		cart := result["cart"].(map[string]any)
		expiresAt, err := time.Parse(time.RFC3339, cart["expires_at"].(string))
		require.NoError(t, err)

		// Verify TTL is set correctly
		expectedExpiry := time.Now().Add(1 * time.Hour)
		assert.WithinDuration(t, expectedExpiry, expiresAt, 5*time.Minute)
	})

	t.Run("Update Cart Item Quantity", func(t *testing.T) {
		// Create cart and add item
		cartID := createCartWithItems(t, ctx, products[:1])

		// Update quantity
		updateReq := map[string]any{
			"quantity": 5,
		}

		req := events.APIGatewayProxyRequest{
			HTTPMethod: "PUT",
			Path:       fmt.Sprintf("/carts/%s/items/%s", cartID, products[0].ID),
			PathParameters: map[string]string{
				"id":        cartID,
				"productId": products[0].ID,
			},
			Body: mustMarshal(updateReq),
		}

		resp, err := cartHandler.UpdateItem(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify update
		cart := getCart(t, ctx, cartID)
		items := cart["items"].([]any)
		assert.Len(t, items, 1)
		assert.Equal(t, 5.0, items[0].(map[string]any)["quantity"].(float64))
	})

	t.Run("Remove Cart Item", func(t *testing.T) {
		// Create cart with multiple items
		cartID := createCartWithItems(t, ctx, products)

		// Remove one item
		req := events.APIGatewayProxyRequest{
			HTTPMethod: "DELETE",
			Path:       fmt.Sprintf("/carts/%s/items/%s", cartID, products[1].ID),
			PathParameters: map[string]string{
				"id":        cartID,
				"productId": products[1].ID,
			},
		}

		resp, err := cartHandler.RemoveItem(ctx, req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Verify removal
		cart := getCart(t, ctx, cartID)
		items := cart["items"].([]any)
		assert.Len(t, items, 2) // Started with 3, removed 1
	})
}

// Helper functions

func createTestProducts(t *testing.T, ctx context.Context) []models.Product {
	products := []models.Product{
		{
			Name:       "Test Product 1",
			SKU:        fmt.Sprintf("TEST-SKU-%d", time.Now().Unix()),
			CategoryID: "electronics",
			Price:      2999, // $29.99
			Stock:      100,
			Status:     models.ProductStatusActive,
		},
		{
			Name:       "Test Product 2",
			SKU:        fmt.Sprintf("TEST-SKU-%d-2", time.Now().Unix()),
			CategoryID: "electronics",
			Price:      4999, // $49.99
			Stock:      50,
			Status:     models.ProductStatusActive,
		},
		{
			Name:       "Test Product 3",
			SKU:        fmt.Sprintf("TEST-SKU-%d-3", time.Now().Unix()),
			CategoryID: "accessories",
			Price:      1499, // $14.99
			Stock:      200,
			Status:     models.ProductStatusActive,
		},
	}

	for i := range products {
		product := &products[i]
		req := events.APIGatewayProxyRequest{
			HTTPMethod: "POST",
			Path:       "/products",
			Headers: map[string]string{
				"X-Admin-Token": "admin-secret-token",
			},
			Body: mustMarshal(product),
		}

		resp, err := productHandler.CreateProduct(ctx, req)
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		var result map[string]any
		err = json.Unmarshal([]byte(resp.Body), &result)
		require.NoError(t, err)

		createdProduct := result["product"].(map[string]any)
		product.ID = createdProduct["id"].(string)
	}

	return products
}

func createTestProduct(t *testing.T, ctx context.Context, name string, price, stock int) models.Product {
	product := models.Product{
		Name:       name,
		SKU:        fmt.Sprintf("TEST-SKU-%d", time.Now().UnixNano()),
		CategoryID: "test",
		Price:      price,
		Stock:      stock,
		Status:     models.ProductStatusActive,
	}

	req := events.APIGatewayProxyRequest{
		HTTPMethod: "POST",
		Path:       "/products",
		Headers: map[string]string{
			"X-Admin-Token": "admin-secret-token",
		},
		Body: mustMarshal(product),
	}

	resp, err := productHandler.CreateProduct(ctx, req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var result map[string]any
	err = json.Unmarshal([]byte(resp.Body), &result)
	require.NoError(t, err)

	createdProduct := result["product"].(map[string]any)
	product.ID = createdProduct["id"].(string)

	return product
}

func createCartWithItems(t *testing.T, ctx context.Context, products []models.Product) string {
	// Create cart
	cartReq := map[string]any{
		"session_id": uuid.New().String(),
		"currency":   "USD",
	}

	req := events.APIGatewayProxyRequest{
		HTTPMethod: "POST",
		Path:       "/carts",
		Body:       mustMarshal(cartReq),
	}

	resp, err := cartHandler.CreateCart(ctx, req)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var result map[string]any
	err = json.Unmarshal([]byte(resp.Body), &result)
	require.NoError(t, err)

	cart := result["cart"].(map[string]any)
	cartID := cart["id"].(string)

	// Add items
	for _, product := range products {
		addItemReq := map[string]any{
			"product_id": product.ID,
			"quantity":   1,
		}

		req := events.APIGatewayProxyRequest{
			HTTPMethod: "POST",
			Path:       fmt.Sprintf("/carts/%s/items", cartID),
			PathParameters: map[string]string{
				"id": cartID,
			},
			Body: mustMarshal(addItemReq),
		}

		resp, err := cartHandler.AddItem(ctx, req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}

	return cartID
}

func getCart(t *testing.T, ctx context.Context, cartID string) map[string]any {
	req := events.APIGatewayProxyRequest{
		HTTPMethod: "GET",
		Path:       fmt.Sprintf("/carts/%s", cartID),
		PathParameters: map[string]string{
			"id": cartID,
		},
	}

	resp, err := cartHandler.GetCart(ctx, req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	err = json.Unmarshal([]byte(resp.Body), &result)
	require.NoError(t, err)

	return result["cart"].(map[string]any)
}

func setupInventory(t *testing.T, ctx context.Context, productID, locationID string, quantity int) {
	update := map[string]any{
		"location_id": locationID,
		"available":   quantity,
	}

	req := events.APIGatewayProxyRequest{
		HTTPMethod: "PUT",
		Path:       fmt.Sprintf("/inventory/%s", productID),
		PathParameters: map[string]string{
			"productId": productID,
		},
		Headers: map[string]string{
			"X-Admin-Token": "admin-secret-token",
		},
		Body: mustMarshal(update),
	}

	resp, err := inventoryHandler.UpdateInventory(ctx, req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func mustMarshal(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func createTestTables() error {
	// In production, use CloudFormation or CDK
	// This is simplified for testing
	return nil
}

func cleanupTestTables() error {
	// Cleanup test data
	return nil
}

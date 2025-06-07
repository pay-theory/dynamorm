package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"

	"github.com/yourusername/dynamorm/examples/ecommerce/handlers"
	dlambda "github.com/yourusername/dynamorm/lambda"
)

var (
	db               *dlambda.OptimizedClient
	productHandler   *handlers.ProductHandlers
	cartHandler      *handlers.CartHandlers
	orderHandler     *handlers.OrderHandlers
	inventoryHandler *handlers.InventoryHandlers
)

func init() {
	// Initialize during cold start
	tableName := os.Getenv("TABLE_NAME")
	if tableName == "" {
		log.Fatal("TABLE_NAME environment variable is required")
	}

	// Load AWS configuration
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	// Initialize DynamORM with Lambda optimizations
	db, err = dlambda.NewOptimizedClient(cfg, tableName, dlambda.Options{
		ConnectionPoolSize: 20,
		EnableCompression:  true,
		CacheSize:          200,
		PrewarmConnections: 10,
	})
	if err != nil {
		log.Fatalf("Failed to initialize DynamORM: %v", err)
	}

	// Pre-warm connections during cold start
	ctx := context.Background()
	if err := db.Prewarm(ctx); err != nil {
		log.Printf("Warning: Failed to prewarm connections: %v", err)
	}

	// Initialize handlers
	productHandler = handlers.NewProductHandlers(db)
	cartHandler = handlers.NewCartHandlers(db)
	orderHandler = handlers.NewOrderHandlers(db)
	inventoryHandler = handlers.NewInventoryHandlers(db)

	log.Println("Lambda function initialized successfully")
}

func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Log request for debugging
	log.Printf("Handling request: %s %s", request.HTTPMethod, request.Path)

	// Route based on path and method
	path := request.Path
	method := request.HTTPMethod

	// Product routes
	if strings.HasPrefix(path, "/products") {
		switch {
		case path == "/products" && method == "GET":
			return productHandler.ListProducts(ctx, request)
		case path == "/products" && method == "POST":
			return productHandler.CreateProduct(ctx, request)
		case path == "/products/search" && method == "GET":
			return productHandler.SearchProducts(ctx, request)
		case strings.HasPrefix(path, "/products/sku/") && method == "GET":
			return productHandler.GetProductBySKU(ctx, request)
		case strings.HasSuffix(path, "/inventory") && method == "POST":
			return productHandler.UpdateInventory(ctx, request)
		case strings.Contains(path, "/products/") && method == "GET":
			return productHandler.GetProduct(ctx, request)
		case strings.Contains(path, "/products/") && method == "PUT":
			return productHandler.UpdateProduct(ctx, request)
		}
	}

	// Cart routes
	if strings.HasPrefix(path, "/carts") {
		switch {
		case path == "/carts" && method == "POST":
			return cartHandler.CreateCart(ctx, request)
		case strings.HasPrefix(path, "/carts/session/") && method == "GET":
			return cartHandler.GetCartBySession(ctx, request)
		case strings.HasSuffix(path, "/items") && method == "POST":
			return cartHandler.AddItem(ctx, request)
		case strings.Contains(path, "/items/") && method == "PUT":
			return cartHandler.UpdateItem(ctx, request)
		case strings.Contains(path, "/items/") && method == "DELETE":
			return cartHandler.RemoveItem(ctx, request)
		case strings.Contains(path, "/carts/") && method == "GET":
			return cartHandler.GetCart(ctx, request)
		case strings.Contains(path, "/carts/") && method == "PUT":
			return cartHandler.UpdateCart(ctx, request)
		case strings.Contains(path, "/carts/") && method == "DELETE":
			return cartHandler.DeleteCart(ctx, request)
		}
	}

	// Order routes
	if strings.HasPrefix(path, "/orders") {
		switch {
		case path == "/orders" && method == "GET":
			return orderHandler.ListOrders(ctx, request)
		case path == "/orders" && method == "POST":
			return orderHandler.CreateOrder(ctx, request)
		case strings.HasPrefix(path, "/orders/number/") && method == "GET":
			return orderHandler.GetOrderByNumber(ctx, request)
		case strings.HasSuffix(path, "/status") && method == "PUT":
			return orderHandler.UpdateOrderStatus(ctx, request)
		case strings.HasSuffix(path, "/cancel") && method == "POST":
			return orderHandler.CancelOrder(ctx, request)
		case strings.Contains(path, "/orders/") && method == "GET":
			return orderHandler.GetOrder(ctx, request)
		}
	}

	// Inventory routes
	if strings.HasPrefix(path, "/inventory") {
		switch {
		case path == "/inventory" && method == "GET":
			return inventoryHandler.ListInventory(ctx, request)
		case path == "/inventory/transfer" && method == "POST":
			return inventoryHandler.TransferInventory(ctx, request)
		case path == "/inventory/bulk-update" && method == "POST":
			return inventoryHandler.BulkUpdateInventory(ctx, request)
		case strings.HasSuffix(path, "/adjust") && method == "POST":
			return inventoryHandler.AdjustInventory(ctx, request)
		case strings.HasSuffix(path, "/movements") && method == "GET":
			return inventoryHandler.GetInventoryMovements(ctx, request)
		case strings.Contains(path, "/inventory/") && method == "GET":
			return inventoryHandler.GetInventory(ctx, request)
		case strings.Contains(path, "/inventory/") && method == "PUT":
			return inventoryHandler.UpdateInventory(ctx, request)
		}
	}

	// Health check
	if path == "/health" && method == "GET" {
		return events.APIGatewayProxyResponse{
			StatusCode: 200,
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Body: `{"status":"healthy","service":"ecommerce-api"}`,
		}, nil
	}

	// Not found
	return events.APIGatewayProxyResponse{
		StatusCode: 404,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: fmt.Sprintf(`{"error":"Route not found: %s %s"}`, method, path),
	}, nil
}

func main() {
	// Start Lambda runtime
	lambda.Start(handler)
}

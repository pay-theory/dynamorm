package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/google/uuid"

	"github.com/yourusername/dynamorm/examples/ecommerce/handlers"
	"github.com/yourusername/dynamorm/examples/ecommerce/models"
	"github.com/yourusername/dynamorm/lambda"
)

// BenchmarkProductCreation measures product creation performance
func BenchmarkProductCreation(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			product := models.Product{
				Name:       fmt.Sprintf("Benchmark Product %d", time.Now().UnixNano()),
				SKU:        fmt.Sprintf("BENCH-SKU-%d", time.Now().UnixNano()),
				CategoryID: "benchmark",
				Price:      2999,
				Stock:      100,
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
			if err != nil {
				b.Fatalf("Failed to create product: %v", err)
			}
			if resp.StatusCode != 201 {
				b.Fatalf("Expected status 201, got %d", resp.StatusCode)
			}
		}
	})

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "products/sec")
}

// BenchmarkProductQuery measures product listing performance
func BenchmarkProductQuery(b *testing.B) {
	ctx := context.Background()

	// Create test products
	for i := 0; i < 100; i++ {
		product := models.Product{
			Name:       fmt.Sprintf("Query Test Product %d", i),
			SKU:        fmt.Sprintf("QUERY-SKU-%d-%d", time.Now().Unix(), i),
			CategoryID: "benchmark",
			Price:      1000 + i*100,
			Stock:      50,
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

		productHandler.CreateProduct(ctx, req)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := events.APIGatewayProxyRequest{
				HTTPMethod: "GET",
				Path:       "/products",
				QueryStringParameters: map[string]string{
					"limit":       "20",
					"category_id": "benchmark",
				},
			}

			resp, err := productHandler.ListProducts(ctx, req)
			if err != nil {
				b.Fatalf("Failed to list products: %v", err)
			}
			if resp.StatusCode != 200 {
				b.Fatalf("Expected status 200, got %d", resp.StatusCode)
			}
		}
	})

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "queries/sec")
}

// BenchmarkCartOperations measures cart operation performance
func BenchmarkCartOperations(b *testing.B) {
	ctx := context.Background()

	// Create test product
	product := createTestProductBench(b, ctx, "Cart Benchmark Product", 2999, 1000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create cart
		cartReq := map[string]interface{}{
			"session_id": fmt.Sprintf("bench-session-%d", i),
			"currency":   "USD",
		}

		req := events.APIGatewayProxyRequest{
			HTTPMethod: "POST",
			Path:       "/carts",
			Body:       mustMarshal(cartReq),
		}

		resp, err := cartHandler.CreateCart(ctx, req)
		if err != nil {
			b.Fatalf("Failed to create cart: %v", err)
		}

		var result map[string]interface{}
		json.Unmarshal([]byte(resp.Body), &result)
		cart := result["cart"].(map[string]interface{})
		cartID := cart["id"].(string)

		// Add item
		addItemReq := map[string]interface{}{
			"product_id": product.ID,
			"quantity":   1,
		}

		req = events.APIGatewayProxyRequest{
			HTTPMethod: "POST",
			Path:       fmt.Sprintf("/carts/%s/items", cartID),
			PathParameters: map[string]string{
				"id": cartID,
			},
			Body: mustMarshal(addItemReq),
		}

		resp, err = cartHandler.AddItem(ctx, req)
		if err != nil {
			b.Fatalf("Failed to add item: %v", err)
		}
	}

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "cart_ops/sec")
}

// BenchmarkOrderCreation measures order creation performance
func BenchmarkOrderCreation(b *testing.B) {
	ctx := context.Background()

	// Create test products
	products := createTestProductsBench(b, ctx)
	customerID := uuid.New().String()

	// Pre-create carts with items
	cartIDs := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		cartID := createCartWithItemsBench(b, ctx, products[:2])
		cartIDs[i] = cartID
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		orderReq := map[string]interface{}{
			"cart_id":     cartIDs[i],
			"customer_id": customerID,
			"email":       "bench@example.com",
			"shipping_address": map[string]interface{}{
				"first_name":  "Benchmark",
				"last_name":   "User",
				"address1":    "123 Bench St",
				"city":        "San Francisco",
				"state":       "CA",
				"postal_code": "94102",
				"country":     "US",
			},
			"billing_address": map[string]interface{}{
				"first_name":  "Benchmark",
				"last_name":   "User",
				"address1":    "123 Bench St",
				"city":        "San Francisco",
				"state":       "CA",
				"postal_code": "94102",
				"country":     "US",
			},
			"payment_method":  "credit_card",
			"payment_id":      fmt.Sprintf("pay_bench_%d", i),
			"shipping_method": "standard",
		}

		req := events.APIGatewayProxyRequest{
			HTTPMethod: "POST",
			Path:       "/orders",
			Body:       mustMarshal(orderReq),
		}

		resp, err := orderHandler.CreateOrder(ctx, req)
		if err != nil {
			b.Fatalf("Failed to create order: %v", err)
		}
		if resp.StatusCode != 201 {
			b.Fatalf("Expected status 201, got %d: %s", resp.StatusCode, resp.Body)
		}
	}

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "orders/sec")
}

// BenchmarkInventoryUpdates measures inventory update performance
func BenchmarkInventoryUpdates(b *testing.B) {
	ctx := context.Background()

	// Create test product with high stock
	product := createTestProductBench(b, ctx, "Inventory Benchmark Product", 2999, 10000)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			adjustment := map[string]interface{}{
				"location_id": "main",
				"quantity":    1, // Small adjustment
				"type":        "sale",
				"reason":      "Benchmark test",
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
			if err != nil {
				b.Fatalf("Failed to adjust inventory: %v", err)
			}
			if resp.StatusCode != 200 {
				b.Fatalf("Expected status 200, got %d", resp.StatusCode)
			}
		}
	})

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "updates/sec")
}

// BenchmarkConcurrentOrders measures concurrent order processing
func BenchmarkConcurrentOrders(b *testing.B) {
	ctx := context.Background()

	// Create shared products
	products := createTestProductsBench(b, ctx)

	// Number of concurrent workers
	workers := 10
	ordersPerWorker := b.N / workers

	b.ResetTimer()

	var wg sync.WaitGroup
	var successCount int64
	var errorCount int64

	start := time.Now()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for i := 0; i < ordersPerWorker; i++ {
				// Create cart
				customerID := fmt.Sprintf("customer-%d-%d", workerID, i)
				cartID := createCartWithItemsBench(b, ctx, products[:2])

				// Create order
				orderReq := map[string]interface{}{
					"cart_id":     cartID,
					"customer_id": customerID,
					"email":       fmt.Sprintf("bench%d@example.com", workerID),
					"shipping_address": map[string]interface{}{
						"first_name":  "Concurrent",
						"last_name":   "Test",
						"address1":    "123 Bench St",
						"city":        "San Francisco",
						"state":       "CA",
						"postal_code": "94102",
						"country":     "US",
					},
					"billing_address": map[string]interface{}{
						"first_name":  "Concurrent",
						"last_name":   "Test",
						"address1":    "123 Bench St",
						"city":        "San Francisco",
						"state":       "CA",
						"postal_code": "94102",
						"country":     "US",
					},
					"payment_method":  "credit_card",
					"payment_id":      fmt.Sprintf("pay_%d_%d", workerID, i),
					"shipping_method": "standard",
				}

				req := events.APIGatewayProxyRequest{
					HTTPMethod: "POST",
					Path:       "/orders",
					Body:       mustMarshal(orderReq),
				}

				resp, err := orderHandler.CreateOrder(ctx, req)
				if err != nil || resp.StatusCode != 201 {
					atomic.AddInt64(&errorCount, 1)
				} else {
					atomic.AddInt64(&successCount, 1)
				}
			}
		}(w)
	}

	wg.Wait()
	elapsed := time.Since(start)

	b.ReportMetric(float64(successCount)/elapsed.Seconds(), "orders/sec")
	b.ReportMetric(float64(errorCount), "errors")
	b.ReportMetric(float64(successCount)/float64(successCount+errorCount)*100, "success_rate_%")
}

// BenchmarkComplexQuery measures complex query performance
func BenchmarkComplexQuery(b *testing.B) {
	ctx := context.Background()

	// Create products with various attributes
	categories := []string{"electronics", "clothing", "books", "toys", "sports"}
	tags := []string{"sale", "new", "featured", "bestseller", "limited"}

	for i := 0; i < 500; i++ {
		product := models.Product{
			Name:       fmt.Sprintf("Complex Query Product %d", i),
			SKU:        fmt.Sprintf("COMPLEX-SKU-%d", time.Now().UnixNano()),
			CategoryID: categories[i%len(categories)],
			Price:      1000 + (i * 100),
			Stock:      50 + i,
			Status:     models.ProductStatusActive,
			Tags:       []string{tags[i%len(tags)], tags[(i+1)%len(tags)]},
			Featured:   i%3 == 0,
		}

		req := events.APIGatewayProxyRequest{
			HTTPMethod: "POST",
			Path:       "/products",
			Headers: map[string]string{
				"X-Admin-Token": "admin-secret-token",
			},
			Body: mustMarshal(product),
		}

		productHandler.CreateProduct(ctx, req)
	}

	queries := []map[string]string{
		{"category_id": "electronics", "limit": "20"},
		{"tags": "sale,featured", "limit": "10"},
		{"status": "active", "limit": "50"},
		{"q": "Complex Query", "limit": "20"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		query := queries[i%len(queries)]

		var req events.APIGatewayProxyRequest
		if _, hasSearch := query["q"]; hasSearch {
			req = events.APIGatewayProxyRequest{
				HTTPMethod:            "GET",
				Path:                  "/products/search",
				QueryStringParameters: query,
			}
			resp, err := productHandler.SearchProducts(ctx, req)
			if err != nil {
				b.Fatalf("Failed to search products: %v", err)
			}
			if resp.StatusCode != 200 {
				b.Fatalf("Expected status 200, got %d", resp.StatusCode)
			}
		} else {
			req = events.APIGatewayProxyRequest{
				HTTPMethod:            "GET",
				Path:                  "/products",
				QueryStringParameters: query,
			}
			resp, err := productHandler.ListProducts(ctx, req)
			if err != nil {
				b.Fatalf("Failed to list products: %v", err)
			}
			if resp.StatusCode != 200 {
				b.Fatalf("Expected status 200, got %d", resp.StatusCode)
			}
		}
	}

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "queries/sec")
}

// BenchmarkLambdaColdStart simulates Lambda cold start performance
func BenchmarkLambdaColdStart(b *testing.B) {
	for i := 0; i < b.N; i++ {
		// Simulate cold start by creating new client
		start := time.Now()

		// Initialize new Lambda-optimized client
		cfg, _ := config.LoadDefaultConfig(context.Background())
		db, _ := lambda.NewOptimizedClient(cfg, "benchmark_test", lambda.Options{
			ConnectionPoolSize: 10,
			EnableCompression:  true,
			CacheSize:          100,
			PrewarmConnections: 5,
		})

		// Create handlers
		prodHandler := handlers.NewProductHandlers(db)

		// Make first request
		req := events.APIGatewayProxyRequest{
			HTTPMethod: "GET",
			Path:       "/products",
			QueryStringParameters: map[string]string{
				"limit": "1",
			},
		}

		resp, err := prodHandler.ListProducts(context.Background(), req)
		if err != nil {
			b.Fatalf("Failed to list products: %v", err)
		}
		if resp.StatusCode != 200 {
			b.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		coldStartTime := time.Since(start)
		b.ReportMetric(float64(coldStartTime.Milliseconds()), "cold_start_ms")
	}
}

// Benchmark results summary printer
func BenchmarkSummary(b *testing.B) {
	b.Skip("Run individual benchmarks to see results")

	fmt.Println("\n=== DynamORM E-commerce Performance Benchmarks ===")
	fmt.Println("\nExpected Performance Metrics:")
	fmt.Println("- Product Creation: 500-1000 ops/sec")
	fmt.Println("- Product Queries: 1000-2000 queries/sec")
	fmt.Println("- Cart Operations: 800-1500 ops/sec")
	fmt.Println("- Order Creation: 200-500 orders/sec")
	fmt.Println("- Inventory Updates: 1000-2000 updates/sec")
	fmt.Println("- Concurrent Orders: 150-300 orders/sec")
	fmt.Println("- Complex Queries: 500-1000 queries/sec")
	fmt.Println("- Lambda Cold Start: < 200ms")
	fmt.Println("\nNote: Actual performance depends on DynamoDB provisioned capacity and network latency")
}

// Benchmark-specific helper functions

func createTestProductsBench(b *testing.B, ctx context.Context) []models.Product {
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
		if err != nil {
			b.Fatalf("Failed to create product: %v", err)
		}
		if resp.StatusCode != 201 {
			b.Fatalf("Expected status 201, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		err = json.Unmarshal([]byte(resp.Body), &result)
		if err != nil {
			b.Fatalf("Failed to unmarshal response: %v", err)
		}

		createdProduct := result["product"].(map[string]interface{})
		product.ID = createdProduct["id"].(string)
	}

	return products
}

func createTestProductBench(b *testing.B, ctx context.Context, name string, price, stock int) models.Product {
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
	if err != nil {
		b.Fatalf("Failed to create product: %v", err)
	}
	if resp.StatusCode != 201 {
		b.Fatalf("Expected status 201, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	err = json.Unmarshal([]byte(resp.Body), &result)
	if err != nil {
		b.Fatalf("Failed to unmarshal response: %v", err)
	}

	createdProduct := result["product"].(map[string]interface{})
	product.ID = createdProduct["id"].(string)

	return product
}

func createCartWithItemsBench(b *testing.B, ctx context.Context, products []models.Product) string {
	// Create cart
	cartReq := map[string]interface{}{
		"session_id": uuid.New().String(),
		"currency":   "USD",
	}

	req := events.APIGatewayProxyRequest{
		HTTPMethod: "POST",
		Path:       "/carts",
		Body:       mustMarshal(cartReq),
	}

	resp, err := cartHandler.CreateCart(ctx, req)
	if err != nil {
		b.Fatalf("Failed to create cart: %v", err)
	}
	if resp.StatusCode != 201 {
		b.Fatalf("Expected status 201, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	err = json.Unmarshal([]byte(resp.Body), &result)
	if err != nil {
		b.Fatalf("Failed to unmarshal response: %v", err)
	}

	cart := result["cart"].(map[string]interface{})
	cartID := cart["id"].(string)

	// Add items
	for _, product := range products {
		addItemReq := map[string]interface{}{
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
		if err != nil {
			b.Fatalf("Failed to add item: %v", err)
		}
		if resp.StatusCode != 200 {
			b.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}
	}

	return cartID
}

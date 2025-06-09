package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/gorilla/mux"

	"github.com/yourusername/dynamorm/examples/ecommerce/handlers"
	"github.com/yourusername/dynamorm/lambda"
)

// Server configuration
type ServerConfig struct {
	Port              string
	DynamoDBEndpoint  string
	TableName         string
	Region            string
	ConnectionPool    int
	EnableCompression bool
}

func main() {
	// Load configuration
	cfg := loadConfig()

	// Initialize DynamoDB client
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.Region),
		config.WithEndpointResolver(aws.EndpointResolverFunc(
			func(service, region string) (aws.Endpoint, error) {
				if service == dynamodb.ServiceID && cfg.DynamoDBEndpoint != "" {
					return aws.Endpoint{
						URL: cfg.DynamoDBEndpoint,
					}, nil
				}
				return aws.Endpoint{}, &aws.EndpointNotFoundError{}
			},
		)),
	)
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	// Initialize DynamORM with Lambda optimizations
	db, err := lambda.NewOptimizedClient(awsCfg, cfg.TableName, lambda.Options{
		ConnectionPoolSize: cfg.ConnectionPool,
		EnableCompression:  cfg.EnableCompression,
		CacheSize:          100,
		PrewarmConnections: 5,
	})
	if err != nil {
		log.Fatalf("Failed to initialize DynamORM: %v", err)
	}

	// Initialize handlers
	productHandler := handlers.NewProductHandlers(db)
	cartHandler := handlers.NewCartHandlers(db)
	orderHandler := handlers.NewOrderHandlers(db)
	inventoryHandler := handlers.NewInventoryHandlers(db)

	// Setup routes
	router := mux.NewRouter()

	// Middleware
	router.Use(loggingMiddleware)
	router.Use(corsMiddleware)

	// Health check
	router.HandleFunc("/health", healthCheckHandler).Methods("GET")

	// Product routes
	router.HandleFunc("/products", adaptHandler(productHandler.ListProducts)).Methods("GET")
	router.HandleFunc("/products", adaptHandler(productHandler.CreateProduct)).Methods("POST")
	router.HandleFunc("/products/search", adaptHandler(productHandler.SearchProducts)).Methods("GET")
	router.HandleFunc("/products/{id}", adaptHandler(productHandler.GetProduct)).Methods("GET")
	router.HandleFunc("/products/{id}", adaptHandler(productHandler.UpdateProduct)).Methods("PUT")
	router.HandleFunc("/products/sku/{sku}", adaptHandler(productHandler.GetProductBySKU)).Methods("GET")
	router.HandleFunc("/products/{id}/inventory", adaptHandler(productHandler.UpdateInventory)).Methods("POST")

	// Cart routes
	router.HandleFunc("/carts", adaptHandler(cartHandler.CreateCart)).Methods("POST")
	router.HandleFunc("/carts/{id}", adaptHandler(cartHandler.GetCart)).Methods("GET")
	router.HandleFunc("/carts/{id}", adaptHandler(cartHandler.UpdateCart)).Methods("PUT")
	router.HandleFunc("/carts/{id}", adaptHandler(cartHandler.DeleteCart)).Methods("DELETE")
	router.HandleFunc("/carts/session/{sessionId}", adaptHandler(cartHandler.GetCartBySession)).Methods("GET")
	router.HandleFunc("/carts/{id}/items", adaptHandler(cartHandler.AddItem)).Methods("POST")
	router.HandleFunc("/carts/{id}/items/{productId}", adaptHandler(cartHandler.UpdateItem)).Methods("PUT")
	router.HandleFunc("/carts/{id}/items/{productId}", adaptHandler(cartHandler.RemoveItem)).Methods("DELETE")

	// Order routes
	router.HandleFunc("/orders", adaptHandler(orderHandler.CreateOrder)).Methods("POST")
	router.HandleFunc("/orders", adaptHandler(orderHandler.ListOrders)).Methods("GET")
	router.HandleFunc("/orders/{id}", adaptHandler(orderHandler.GetOrder)).Methods("GET")
	router.HandleFunc("/orders/number/{orderNumber}", adaptHandler(orderHandler.GetOrderByNumber)).Methods("GET")
	router.HandleFunc("/orders/{id}/status", adaptHandler(orderHandler.UpdateOrderStatus)).Methods("PUT")
	router.HandleFunc("/orders/{id}/cancel", adaptHandler(orderHandler.CancelOrder)).Methods("POST")

	// Inventory routes
	router.HandleFunc("/inventory/{productId}", adaptHandler(inventoryHandler.GetInventory)).Methods("GET")
	router.HandleFunc("/inventory", adaptHandler(inventoryHandler.ListInventory)).Methods("GET")
	router.HandleFunc("/inventory/{productId}", adaptHandler(inventoryHandler.UpdateInventory)).Methods("PUT")
	router.HandleFunc("/inventory/{productId}/adjust", adaptHandler(inventoryHandler.AdjustInventory)).Methods("POST")
	router.HandleFunc("/inventory/transfer", adaptHandler(inventoryHandler.TransferInventory)).Methods("POST")
	router.HandleFunc("/inventory/{productId}/movements", adaptHandler(inventoryHandler.GetInventoryMovements)).Methods("GET")
	router.HandleFunc("/inventory/bulk-update", adaptHandler(inventoryHandler.BulkUpdateInventory)).Methods("POST")

	// Start server
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		log.Printf("Starting server on port %s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

func loadConfig() ServerConfig {
	return ServerConfig{
		Port:              getEnv("PORT", "8080"),
		DynamoDBEndpoint:  getEnv("DYNAMODB_ENDPOINT", "http://localhost:8000"),
		TableName:         getEnv("TABLE_NAME", "ecommerce_local"),
		Region:            getEnv("AWS_REGION", "us-east-1"),
		ConnectionPool:    10,
		EnableCompression: true,
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// adaptHandler converts Lambda handler to HTTP handler
func adaptHandler(lambdaHandler func(context.Context, events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Convert HTTP request to Lambda event
		event := httpToLambdaEvent(r)

		// Call Lambda handler
		response, err := lambdaHandler(r.Context(), event)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Write response
		for key, value := range response.Headers {
			w.Header().Set(key, value)
		}
		w.WriteHeader(response.StatusCode)
		w.Write([]byte(response.Body))
	}
}

func httpToLambdaEvent(r *http.Request) events.APIGatewayProxyRequest {
	// Extract path parameters
	vars := mux.Vars(r)
	pathParams := make(map[string]string)
	for key, value := range vars {
		pathParams[key] = value
	}

	// Extract query parameters
	queryParams := make(map[string]string)
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			queryParams[key] = values[0]
		}
	}

	// Extract headers
	headers := make(map[string]string)
	for key, values := range r.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	// Read body
	var body string
	if r.Body != nil {
		defer r.Body.Close()
		bodyBytes, _ := ioutil.ReadAll(r.Body)
		body = string(bodyBytes)
	}

	return events.APIGatewayProxyRequest{
		HTTPMethod:            r.Method,
		Path:                  r.URL.Path,
		PathParameters:        pathParams,
		QueryStringParameters: queryParams,
		Headers:               headers,
		Body:                  body,
	}
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]any{
		"status": "healthy",
		"time":   time.Now().UTC(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		log.Printf(
			"%s %s %d %s",
			r.Method,
			r.RequestURI,
			wrapped.statusCode,
			time.Since(start),
		)
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Customer-ID, X-Admin-Token")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

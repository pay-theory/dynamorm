package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/example/dynamorm"
	"github.com/example/dynamorm/examples/payment"
)

// QueryRequest represents the query parameters
type QueryRequest struct {
	Status     string `json:"status,omitempty"`
	StartDate  string `json:"start_date,omitempty"`
	EndDate    string `json:"end_date,omitempty"`
	CustomerID string `json:"customer_id,omitempty"`
	MinAmount  int64  `json:"min_amount,omitempty"`
	MaxAmount  int64  `json:"max_amount,omitempty"`
	Limit      int    `json:"limit,omitempty"`
	Cursor     string `json:"cursor,omitempty"`
}

// QueryResponse represents the paginated response
type QueryResponse struct {
	Payments   []*payment.Payment `json:"payments"`
	NextCursor string             `json:"next_cursor,omitempty"`
	Total      int                `json:"total"`
	HasMore    bool               `json:"has_more"`
}

// PaymentSummary provides aggregated statistics
type PaymentSummary struct {
	TotalAmount   int64            `json:"total_amount"`
	TotalCount    int              `json:"total_count"`
	ByStatus      map[string]int   `json:"by_status"`
	ByCurrency    map[string]int64 `json:"by_currency"`
	AverageAmount int64            `json:"average_amount"`
}

// Handler processes query requests
type Handler struct {
	db *dynamorm.DB
}

// NewHandler creates a new query handler
func NewHandler() (*Handler, error) {
	// Initialize DynamoDB connection
	db, err := dynamorm.New(
		dynamorm.WithLambdaOptimization(),
		dynamorm.WithConnectionPool(5), // Smaller pool for query operations
		dynamorm.WithRegion("us-east-1"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DynamoDB: %w", err)
	}

	// Register models
	db.Model(&payment.Payment{})
	db.Model(&payment.Customer{})
	db.Model(&payment.Transaction{})

	return &Handler{db: db}, nil
}

// HandleRequest processes query requests
func (h *Handler) HandleRequest(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Extract merchant ID from JWT
	merchantID, err := extractMerchantID(request.Headers)
	if err != nil {
		return errorResponse(http.StatusUnauthorized, "Invalid authentication"), nil
	}

	// Parse query parameters
	query := parseQueryParams(request.QueryStringParameters)

	// Handle different endpoints
	switch request.Path {
	case "/payments":
		return h.queryPayments(ctx, merchantID, query)
	case "/payments/summary":
		return h.getPaymentSummary(ctx, merchantID, query)
	case "/payments/export":
		return h.exportPayments(ctx, merchantID, query)
	default:
		if strings.HasPrefix(request.Path, "/payments/") {
			// Get single payment
			paymentID := strings.TrimPrefix(request.Path, "/payments/")
			return h.getPayment(ctx, merchantID, paymentID)
		}
		return errorResponse(http.StatusNotFound, "Endpoint not found"), nil
	}
}

// queryPayments returns paginated payment results
func (h *Handler) queryPayments(ctx context.Context, merchantID string, req *QueryRequest) (events.APIGatewayProxyResponse, error) {
	// Build query
	query := h.db.Model(&payment.Payment{}).
		Index("gsi-merchant").
		Where("MerchantID", "=", merchantID)

	// Apply filters
	if req.Status != "" {
		query = query.Where("Status", "=", req.Status)
	}

	if req.CustomerID != "" {
		query = query.Where("CustomerID", "=", req.CustomerID)
	}

	if req.MinAmount > 0 {
		query = query.Where("Amount", ">=", req.MinAmount)
	}

	if req.MaxAmount > 0 {
		query = query.Where("Amount", "<=", req.MaxAmount)
	}

	// Apply date filters
	if req.StartDate != "" {
		startTime, err := time.Parse("2006-01-02", req.StartDate)
		if err == nil {
			query = query.Where("CreatedAt", ">=", startTime)
		}
	}

	if req.EndDate != "" {
		endTime, err := time.Parse("2006-01-02", req.EndDate)
		if err == nil {
			// Add 1 day to include the entire end date
			endTime = endTime.Add(24 * time.Hour)
			query = query.Where("CreatedAt", "<", endTime)
		}
	}

	// Set limit
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query = query.Limit(limit)

	// Apply cursor if provided
	if req.Cursor != "" {
		cursor, err := decodeCursor(req.Cursor)
		if err == nil {
			query = query.Cursor(cursor)
		}
	}

	// Execute query
	var payments []*payment.Payment
	nextCursor, err := query.All(&payments)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to query payments"), nil
	}

	// Encode next cursor
	var encodedCursor string
	if nextCursor != "" {
		encodedCursor = encodeCursor(nextCursor)
	}

	// Build response
	response := QueryResponse{
		Payments:   payments,
		NextCursor: encodedCursor,
		Total:      len(payments),
		HasMore:    nextCursor != "",
	}

	return successResponse(http.StatusOK, response), nil
}

// getPayment returns a single payment
func (h *Handler) getPayment(ctx context.Context, merchantID, paymentID string) (events.APIGatewayProxyResponse, error) {
	var payment payment.Payment
	err := h.db.Model(&payment.Payment{}).
		Where("ID", "=", paymentID).
		Where("MerchantID", "=", merchantID). // Ensure merchant owns this payment
		First(&payment)

	if err != nil {
		if err == dynamorm.ErrNotFound {
			return errorResponse(http.StatusNotFound, "Payment not found"), nil
		}
		return errorResponse(http.StatusInternalServerError, "Failed to retrieve payment"), nil
	}

	// Get related transactions
	var transactions []*payment.Transaction
	err = h.db.Model(&payment.Transaction{}).
		Index("gsi-payment").
		Where("PaymentID", "=", paymentID).
		All(&transactions)

	// Build detailed response
	response := map[string]interface{}{
		"payment":      payment,
		"transactions": transactions,
	}

	return successResponse(http.StatusOK, response), nil
}

// getPaymentSummary returns aggregated statistics
func (h *Handler) getPaymentSummary(ctx context.Context, merchantID string, req *QueryRequest) (events.APIGatewayProxyResponse, error) {
	// For large datasets, this would typically use DynamoDB Streams + aggregation
	// For demo purposes, we'll do a simplified version

	var payments []*payment.Payment
	query := h.db.Model(&payment.Payment{}).
		Index("gsi-merchant").
		Where("MerchantID", "=", merchantID)

	// Apply date filters for summary
	if req.StartDate != "" {
		startTime, _ := time.Parse("2006-01-02", req.StartDate)
		query = query.Where("CreatedAt", ">=", startTime)
	}

	if req.EndDate != "" {
		endTime, _ := time.Parse("2006-01-02", req.EndDate)
		endTime = endTime.Add(24 * time.Hour)
		query = query.Where("CreatedAt", "<", endTime)
	}

	// Scan all payments (in production, use aggregation tables)
	err := query.Scan(&payments)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, "Failed to calculate summary"), nil
	}

	// Calculate summary
	summary := &PaymentSummary{
		ByStatus:   make(map[string]int),
		ByCurrency: make(map[string]int64),
	}

	for _, p := range payments {
		summary.TotalAmount += p.Amount
		summary.TotalCount++
		summary.ByStatus[p.Status]++
		summary.ByCurrency[p.Currency] += p.Amount
	}

	if summary.TotalCount > 0 {
		summary.AverageAmount = summary.TotalAmount / int64(summary.TotalCount)
	}

	return successResponse(http.StatusOK, summary), nil
}

// exportPayments generates a CSV export URL
func (h *Handler) exportPayments(ctx context.Context, merchantID string, req *QueryRequest) (events.APIGatewayProxyResponse, error) {
	// In production, this would:
	// 1. Create an async export job
	// 2. Process in the background
	// 3. Upload to S3
	// 4. Return a pre-signed URL

	exportID := fmt.Sprintf("export-%s-%d", merchantID, time.Now().Unix())

	response := map[string]interface{}{
		"export_id": exportID,
		"status":    "processing",
		"message":   "Export job created. You will receive a notification when complete.",
	}

	// TODO: Trigger async export Lambda

	return successResponse(http.StatusAccepted, response), nil
}

// Helper functions

func extractMerchantID(headers map[string]string) (string, error) {
	auth := headers["Authorization"]
	if auth == "" {
		auth = headers["authorization"]
	}

	if !strings.HasPrefix(auth, "Bearer ") {
		return "", fmt.Errorf("invalid authorization header")
	}

	// TODO: Validate JWT and extract merchant ID
	return "merchant-123", nil
}

func parseQueryParams(params map[string]string) *QueryRequest {
	req := &QueryRequest{}

	req.Status = params["status"]
	req.StartDate = params["start_date"]
	req.EndDate = params["end_date"]
	req.CustomerID = params["customer_id"]
	req.Cursor = params["cursor"]

	if limit, err := strconv.Atoi(params["limit"]); err == nil {
		req.Limit = limit
	}

	if minAmount, err := strconv.ParseInt(params["min_amount"], 10, 64); err == nil {
		req.MinAmount = minAmount
	}

	if maxAmount, err := strconv.ParseInt(params["max_amount"], 10, 64); err == nil {
		req.MaxAmount = maxAmount
	}

	return req
}

func encodeCursor(cursor string) string {
	return base64.URLEncoding.EncodeToString([]byte(cursor))
}

func decodeCursor(encoded string) (string, error) {
	decoded, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
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

func main() {
	handler, err := NewHandler()
	if err != nil {
		panic(fmt.Sprintf("Failed to initialize handler: %v", err))
	}

	lambda.Start(handler.HandleRequest)
}

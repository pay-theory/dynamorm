package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/google/uuid"

	"github.com/example/dynamorm"
	"github.com/example/dynamorm/examples/payment"
	"github.com/example/dynamorm/examples/payment/utils"
)

// ProcessPaymentRequest represents the payment request payload
type ProcessPaymentRequest struct {
	IdempotencyKey string            `json:"idempotency_key"`
	Amount         int64             `json:"amount"`
	Currency       string            `json:"currency"`
	PaymentMethod  string            `json:"payment_method"`
	CustomerID     string            `json:"customer_id,omitempty"`
	Description    string            `json:"description,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// Handler processes payment requests
type Handler struct {
	db          *dynamorm.DB
	idempotency *utils.IdempotencyMiddleware
}

// NewHandler creates a new payment handler
func NewHandler() (*Handler, error) {
	// Initialize DynamoDB connection with Lambda optimizations
	db, err := dynamorm.New(
		dynamorm.WithLambdaOptimization(),
		dynamorm.WithConnectionPool(10),
		dynamorm.WithRegion("us-east-1"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize DynamoDB: %w", err)
	}

	// Register models
	db.Model(&payment.Payment{})
	db.Model(&payment.IdempotencyRecord{})
	db.Model(&payment.Transaction{})
	db.Model(&payment.AuditEntry{})

	// Initialize idempotency middleware
	idempotency := utils.NewIdempotencyMiddleware(db, 24*time.Hour)

	return &Handler{
		db:          db,
		idempotency: idempotency,
	}, nil
}

// HandleRequest processes the payment request
func (h *Handler) HandleRequest(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Extract merchant ID from JWT claims
	merchantID, err := extractMerchantID(request.Headers)
	if err != nil {
		return errorResponse(http.StatusUnauthorized, "Invalid authentication"), nil
	}

	// Parse request body
	var req ProcessPaymentRequest
	if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
		return errorResponse(http.StatusBadRequest, "Invalid request body"), nil
	}

	// Validate request
	if err := validatePaymentRequest(&req); err != nil {
		return errorResponse(http.StatusBadRequest, err.Error()), nil
	}

	// Create idempotency key if not provided
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = generateIdempotencyKey(merchantID, &req)
	}

	// Process with idempotency
	result, err := h.idempotency.Process(ctx, merchantID, req.IdempotencyKey, func() (interface{}, error) {
		return h.processPayment(ctx, merchantID, &req)
	})

	if err != nil {
		if err == utils.ErrDuplicateRequest {
			// Return cached response
			if cached, ok := result.(*payment.Payment); ok {
				return successResponse(http.StatusOK, cached), nil
			}
		}
		return errorResponse(http.StatusInternalServerError, "Payment processing failed"), nil
	}

	// Return success response
	if payment, ok := result.(*payment.Payment); ok {
		return successResponse(http.StatusCreated, payment), nil
	}

	return errorResponse(http.StatusInternalServerError, "Unexpected error"), nil
}

// processPayment handles the actual payment processing
func (h *Handler) processPayment(ctx context.Context, merchantID string, req *ProcessPaymentRequest) (*payment.Payment, error) {
	// Create payment record
	payment := &payment.Payment{
		ID:             uuid.New().String(),
		IdempotencyKey: req.IdempotencyKey,
		MerchantID:     merchantID,
		Amount:         req.Amount,
		Currency:       req.Currency,
		Status:         payment.PaymentStatusPending,
		PaymentMethod:  req.PaymentMethod,
		CustomerID:     req.CustomerID,
		Description:    req.Description,
		Metadata:       req.Metadata,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Version:        1,
	}

	// Start transaction
	tx := h.db.Transaction()
	defer tx.Rollback()

	// Create payment record
	if err := tx.Model(payment).Create(); err != nil {
		return nil, fmt.Errorf("failed to create payment: %w", err)
	}

	// Create initial transaction
	transaction := &payment.Transaction{
		ID:          uuid.New().String(),
		PaymentID:   payment.ID,
		Type:        payment.TransactionTypeCapture,
		Amount:      payment.Amount,
		Status:      payment.PaymentStatusProcessing,
		ProcessedAt: time.Now(),
		AuditTrail: []payment.AuditEntry{
			{
				Timestamp: time.Now(),
				Action:    "payment_initiated",
				Changes: map[string]interface{}{
					"amount":   payment.Amount,
					"currency": payment.Currency,
				},
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Version:   1,
	}

	if err := tx.Model(transaction).Create(); err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	// TODO: Call payment processor API here
	// For now, we'll simulate success
	payment.Status = payment.PaymentStatusSucceeded
	transaction.Status = payment.PaymentStatusSucceeded
	transaction.ResponseCode = "00"
	transaction.ResponseText = "Approved"

	// Update payment status
	if err := tx.Model(payment).
		Where("ID", "=", payment.ID).
		Update(map[string]interface{}{
			"Status":    payment.Status,
			"UpdatedAt": time.Now(),
		}); err != nil {
		return nil, fmt.Errorf("failed to update payment: %w", err)
	}

	// Update transaction status
	if err := tx.Model(transaction).
		Where("ID", "=", transaction.ID).
		Update(map[string]interface{}{
			"Status":       transaction.Status,
			"ResponseCode": transaction.ResponseCode,
			"ResponseText": transaction.ResponseText,
			"UpdatedAt":    time.Now(),
		}); err != nil {
		return nil, fmt.Errorf("failed to update transaction: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// TODO: Send webhook notification asynchronously

	return payment, nil
}

// Helper functions

func extractMerchantID(headers map[string]string) (string, error) {
	// Extract from Authorization header
	auth := headers["Authorization"]
	if auth == "" {
		auth = headers["authorization"]
	}

	if !strings.HasPrefix(auth, "Bearer ") {
		return "", fmt.Errorf("invalid authorization header")
	}

	// TODO: Validate JWT and extract merchant ID
	// For now, return a mock merchant ID
	return "merchant-123", nil
}

func validatePaymentRequest(req *ProcessPaymentRequest) error {
	if req.Amount <= 0 {
		return fmt.Errorf("amount must be greater than 0")
	}
	if req.Currency == "" {
		return fmt.Errorf("currency is required")
	}
	if req.PaymentMethod == "" {
		return fmt.Errorf("payment_method is required")
	}
	return nil
}

func generateIdempotencyKey(merchantID string, req *ProcessPaymentRequest) string {
	data := fmt.Sprintf("%s:%d:%s:%s:%d",
		merchantID,
		req.Amount,
		req.Currency,
		req.PaymentMethod,
		time.Now().Unix(),
	)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}

func successResponse(statusCode int, data interface{}) events.APIGatewayProxyResponse {
	body, _ := json.Marshal(map[string]interface{}{
		"success": true,
		"data":    data,
	})

	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Headers: map[string]string{
			"Content-Type": "application/json",
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
			"Content-Type": "application/json",
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

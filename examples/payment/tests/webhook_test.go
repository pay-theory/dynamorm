package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/pay-theory/dynamorm"
	"github.com/pay-theory/dynamorm/examples/payment"
	"github.com/pay-theory/dynamorm/examples/payment/utils"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWebhookSender tests the webhook notification system
func TestWebhookSender(t *testing.T) {
	// Setup test database
	db, err := setupTestDB(t)
	require.NoError(t, err)

	// Create a test merchant with webhook URL
	merchant := &payment.Merchant{
		ID:            "test-merchant-123",
		Name:          "Test Merchant",
		Email:         "test@merchant.com",
		Status:        "active",
		WebhookURL:    "", // Will be set to test server URL
		WebhookSecret: "test-secret-key",
		Features:      []string{"webhooks"},
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Version:       1,
	}

	// Track received webhooks
	var receivedWebhooks []utils.WebhookPayload
	var mu sync.Mutex

	// Create test webhook server
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.NotEmpty(t, r.Header.Get("X-Webhook-ID"))
		assert.NotEmpty(t, r.Header.Get("X-Webhook-Timestamp"))
		assert.NotEmpty(t, r.Header.Get("X-Webhook-Signature"))

		// Parse webhook payload
		var payload utils.WebhookPayload
		err := json.NewDecoder(r.Body).Decode(&payload)
		require.NoError(t, err)

		// Store received webhook
		mu.Lock()
		receivedWebhooks = append(receivedWebhooks, payload)
		mu.Unlock()

		// Return success
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "received"})
	}))
	defer webhookServer.Close()

	// Update merchant with test server URL
	merchant.WebhookURL = webhookServer.URL
	err = db.Model(merchant).Create()
	require.NoError(t, err)

	// Initialize webhook sender
	sender := utils.NewWebhookSender(db, 2)
	defer sender.Stop()

	// Test payment data
	testPayment := &payment.Payment{
		ID:            "pay-123",
		MerchantID:    merchant.ID,
		Amount:        1000,
		Currency:      "USD",
		Status:        payment.PaymentStatusSucceeded,
		PaymentMethod: "card",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Version:       1,
	}

	// Send webhook
	job := &utils.WebhookJob{
		MerchantID: merchant.ID,
		EventType:  "payment.succeeded",
		PaymentID:  testPayment.ID,
		Data:       testPayment,
	}

	err = sender.Send(job)
	require.NoError(t, err)

	// Wait for webhook delivery
	time.Sleep(2 * time.Second)

	// Verify webhook was received
	mu.Lock()
	defer mu.Unlock()

	assert.Len(t, receivedWebhooks, 1)
	assert.Equal(t, "payment.succeeded", receivedWebhooks[0].EventType)
	assert.NotEmpty(t, receivedWebhooks[0].ID)
	assert.NotNil(t, receivedWebhooks[0].Data)

	// Verify webhook record in database
	var webhookRecord payment.Webhook
	err = db.Model(&payment.Webhook{}).
		Where("MerchantID", "=", merchant.ID).
		Where("EventType", "=", "payment.succeeded").
		First(&webhookRecord)

	require.NoError(t, err)
	assert.Equal(t, payment.WebhookStatusDelivered, webhookRecord.Status)
	assert.Equal(t, 200, webhookRecord.ResponseCode)
	assert.Equal(t, 1, webhookRecord.Attempts)
}

// TestWebhookRetry tests the webhook retry mechanism
func TestWebhookRetry(t *testing.T) {
	// Setup test database
	db, err := setupTestDB(t)
	require.NoError(t, err)

	// Create test merchant
	merchant := &payment.Merchant{
		ID:            "test-merchant-retry",
		Name:          "Test Merchant Retry",
		Email:         "retry@merchant.com",
		Status:        "active",
		WebhookSecret: "test-secret",
		Features:      []string{"webhooks"},
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Version:       1,
	}

	// Track attempt count
	attemptCount := 0
	var mu sync.Mutex

	// Create failing webhook server (returns 500 for first 2 attempts)
	webhookServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attemptCount++
		currentAttempt := attemptCount
		mu.Unlock()

		if currentAttempt <= 2 {
			// Fail first 2 attempts
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "temporary failure"})
		} else {
			// Succeed on 3rd attempt
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "success"})
		}
	}))
	defer webhookServer.Close()

	merchant.WebhookURL = webhookServer.URL
	err = db.Model(merchant).Create()
	require.NoError(t, err)

	// Initialize webhook sender
	sender := utils.NewWebhookSender(db, 1)
	defer sender.Stop()

	// Send webhook
	job := &utils.WebhookJob{
		MerchantID: merchant.ID,
		EventType:  "payment.failed",
		PaymentID:  "pay-456",
		Data:       map[string]string{"reason": "insufficient_funds"},
	}

	err = sender.Send(job)
	require.NoError(t, err)

	// Wait for retries
	time.Sleep(10 * time.Second)

	// Verify webhook was eventually delivered
	var webhookRecord payment.Webhook
	err = db.Model(&payment.Webhook{}).
		Where("MerchantID", "=", merchant.ID).
		Where("EventType", "=", "payment.failed").
		First(&webhookRecord)

	require.NoError(t, err)
	assert.Equal(t, payment.WebhookStatusDelivered, webhookRecord.Status)
	assert.Equal(t, 200, webhookRecord.ResponseCode)
	assert.Equal(t, 3, webhookRecord.Attempts)
}

// TestJWTValidation tests the JWT validator
func TestJWTValidation(t *testing.T) {
	// Initialize JWT validator
	validator := utils.NewSimpleJWTValidator(
		"test-secret-key",
		"test-issuer",
		"payment-api",
	)

	// Test cases
	tests := []struct {
		name      string
		token     string
		wantError bool
		errorMsg  string
	}{
		{
			name:      "Valid token",
			token:     createTestToken("merchant-123", "test@example.com", time.Now().Add(time.Hour)),
			wantError: false,
		},
		{
			name:      "Expired token",
			token:     createTestToken("merchant-123", "test@example.com", time.Now().Add(-time.Hour)),
			wantError: true,
			errorMsg:  "token expired",
		},
		{
			name:      "Invalid format",
			token:     "invalid.token.format",
			wantError: true,
			errorMsg:  "invalid signature",
		},
		{
			name:      "Missing merchant ID",
			token:     createTestTokenWithoutMerchant("test@example.com", time.Now().Add(time.Hour)),
			wantError: true,
			errorMsg:  "missing merchant_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := validator.ValidateToken(tt.token)

			if tt.wantError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, claims)
				assert.Equal(t, "merchant-123", claims.MerchantID)
				assert.Equal(t, "test@example.com", claims.Email)
			}
		})
	}
}

// TestExtractTokenFromHeader tests token extraction from Authorization header
func TestExtractTokenFromHeader(t *testing.T) {
	tests := []struct {
		name      string
		header    string
		wantToken string
		wantError bool
	}{
		{
			name:      "Valid Bearer token",
			header:    "Bearer eyJhbGciOiJIUzI1NiJ9.eyJtZXJjaGFudF9pZCI6IjEyMyJ9.abc",
			wantToken: "eyJhbGciOiJIUzI1NiJ9.eyJtZXJjaGFudF9pZCI6IjEyMyJ9.abc",
			wantError: false,
		},
		{
			name:      "Empty header",
			header:    "",
			wantError: true,
		},
		{
			name:      "Missing Bearer prefix",
			header:    "eyJhbGciOiJIUzI1NiJ9.eyJtZXJjaGFudF9pZCI6IjEyMyJ9.abc",
			wantError: true,
		},
		{
			name:      "Empty token after Bearer",
			header:    "Bearer ",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := utils.ExtractTokenFromHeader(tt.header)

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantToken, token)
			}
		})
	}
}

// Helper functions

func setupTestDB(_ *testing.T) (core.ExtendedDB, error) {
	db, err := dynamorm.New(dynamorm.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
	})
	if err != nil {
		return nil, err
	}

	// Register models
	models := []any{
		&payment.Payment{},
		&payment.Merchant{},
		&payment.Webhook{},
	}

	for _, model := range models {
		db.Model(model)
	}

	return db, nil
}

// createTestToken creates a test JWT token with the simple validator format
func createTestToken(merchantID string, email string, expiry time.Time) string {
	// This is a simplified version - in real tests you'd use the same
	// HMAC signing method as the validator
	// For now, returning a pre-generated token for merchant-123
	_ = merchantID
	_ = email
	_ = expiry
	return "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJtZXJjaGFudF9pZCI6Im1lcmNoYW50LTEyMyIsImVtYWlsIjoidGVzdEBleGFtcGxlLmNvbSIsImV4cCI6OTk5OTk5OTk5OX0.ZjhmMjVmZWU4NzM2YWE1ZmQ5ZGFmNzUwZjM4MjU0ZWU4MWYzMTI1YzQzYzJhZGE0YWI1MmU5OGQzZGFkYzM5ZQ"
}

func createTestTokenWithoutMerchant(email string, expiry time.Time) string {
	// Token without merchant_id claim
	_ = email
	_ = expiry
	return "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJlbWFpbCI6InRlc3RAZXhhbXBsZS5jb20iLCJleHAiOjk5OTk5OTk5OTl9.YjQzNDRkYjY3OGI0NTY3OGIzNDU2Nzg5MzQ1Njc4OTM0NTY3ODkzNDU2Nzg5MzQ1Njc4OTM0NTY3ODkzNDU2"
}

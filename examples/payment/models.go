package payment

import (
	"time"
)

// Payment represents a payment transaction with idempotency support
type Payment struct {
	UpdatedAt      time.Time         `dynamorm:"updated_at" json:"updated_at"`
	CreatedAt      time.Time         `dynamorm:"created_at" json:"created_at"`
	Metadata       map[string]string `dynamorm:"json" json:"metadata,omitempty"`
	PaymentMethod  string            `json:"payment_method"`
	Currency       string            `json:"currency"`
	Status         string            `dynamorm:"index:gsi-merchant,sk,prefix:status" json:"status"`
	ID             string            `dynamorm:"pk" json:"id"`
	CustomerID     string            `dynamorm:"index:gsi-customer" json:"customer_id,omitempty"`
	Description    string            `json:"description,omitempty"`
	MerchantID     string            `dynamorm:"index:gsi-merchant,pk" json:"merchant_id"`
	IdempotencyKey string            `dynamorm:"index:gsi-idempotency" json:"idempotency_key"`
	Amount         int64             `json:"amount"`
	Version        int               `dynamorm:"version" json:"version"`
}

// PaymentStatus constants
const (
	PaymentStatusPending    = "pending"
	PaymentStatusProcessing = "processing"
	PaymentStatusSucceeded  = "succeeded"
	PaymentStatusFailed     = "failed"
	PaymentStatusCanceled   = "canceled"
)

// Transaction represents a transaction on a payment (capture, refund, void)
type Transaction struct {
	CreatedAt    time.Time    `dynamorm:"created_at" json:"created_at"`
	UpdatedAt    time.Time    `dynamorm:"updated_at" json:"updated_at"`
	ProcessedAt  time.Time    `json:"processed_at"`
	PaymentID    string       `dynamorm:"index:gsi-payment" json:"payment_id"`
	Type         string       `json:"type"`
	Status       string       `json:"status"`
	ProcessorID  string       `json:"processor_id,omitempty"`
	ResponseCode string       `json:"response_code,omitempty"`
	ResponseText string       `json:"response_text,omitempty"`
	ID           string       `dynamorm:"pk" json:"id"`
	AuditTrail   []AuditEntry `dynamorm:"json" json:"audit_trail"`
	Amount       int64        `json:"amount"`
	Version      int          `dynamorm:"version" json:"version"`
}

// TransactionType constants
const (
	TransactionTypeCapture = "capture"
	TransactionTypeRefund  = "refund"
	TransactionTypeVoid    = "void"
)

// Customer represents a customer with PCI-compliant payment methods
type Customer struct {
	CreatedAt      time.Time         `dynamorm:"created_at" json:"created_at"`
	UpdatedAt      time.Time         `dynamorm:"updated_at" json:"updated_at"`
	Metadata       map[string]string `dynamorm:"json" json:"metadata,omitempty"`
	ID             string            `dynamorm:"pk" json:"id"`
	MerchantID     string            `dynamorm:"index:gsi-merchant,pk" json:"merchant_id"`
	Email          string            `dynamorm:"index:gsi-email,pk,encrypted" json:"email"`
	Name           string            `dynamorm:"encrypted" json:"name"`
	Phone          string            `dynamorm:"encrypted" json:"phone,omitempty"`
	DefaultMethod  string            `json:"default_method,omitempty"`
	PaymentMethods []PaymentMethod   `dynamorm:"json,encrypted:pci" json:"payment_methods"`
	Version        int               `dynamorm:"version" json:"version"`
}

// PaymentMethod represents a customer's payment method
type PaymentMethod struct {
	CreatedAt   time.Time `json:"created_at"`
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Last4       string    `json:"last4"`
	Brand       string    `json:"brand,omitempty"`
	BankName    string    `json:"bank_name,omitempty"`
	AccountType string    `json:"account_type,omitempty"`
	Token       string    `json:"-"`
	ExpiryMonth int       `json:"expiry_month,omitempty"`
	ExpiryYear  int       `json:"expiry_year,omitempty"`
	IsDefault   bool      `json:"is_default"`
}

// Merchant represents a merchant account
type Merchant struct {
	CreatedAt       time.Time      `dynamorm:"created_at" json:"created_at"`
	UpdatedAt       time.Time      `dynamorm:"updated_at" json:"updated_at"`
	ProcessorConfig map[string]any `dynamorm:"json,encrypted" json:"-"`
	ID              string         `dynamorm:"pk" json:"id"`
	Name            string         `json:"name"`
	Email           string         `dynamorm:"index:gsi-email,pk" json:"email"`
	Status          string         `json:"status"`
	WebhookURL      string         `json:"webhook_url,omitempty"`
	WebhookSecret   string         `dynamorm:"encrypted" json:"-"`
	Features        []string       `dynamorm:"set" json:"features"`
	RateLimits      RateLimits     `dynamorm:"json" json:"rate_limits"`
	Version         int            `dynamorm:"version" json:"version"`
}

// RateLimits defines rate limiting configuration
type RateLimits struct {
	PaymentsPerMinute int   `json:"payments_per_minute"`
	PaymentsPerDay    int   `json:"payments_per_day"`
	MaxPaymentAmount  int64 `json:"max_payment_amount"`
}

// AuditEntry represents an entry in the audit trail
type AuditEntry struct {
	Timestamp time.Time      `json:"timestamp"`
	Action    string         `json:"action"`
	UserID    string         `json:"user_id,omitempty"`
	IPAddress string         `json:"ip_address,omitempty"`
	Changes   map[string]any `json:"changes,omitempty"`
	Reason    string         `json:"reason,omitempty"`
}

// IdempotencyRecord tracks idempotent requests
type IdempotencyRecord struct {
	CreatedAt   time.Time `dynamorm:"created_at" json:"created_at"`
	Key         string    `dynamorm:"pk" json:"key"`
	MerchantID  string    `dynamorm:"index:gsi-merchant,pk" json:"merchant_id"`
	RequestHash string    `json:"request_hash"`
	Response    string    `dynamorm:"json" json:"response"`
	StatusCode  int       `json:"status_code"`
	ExpiresAt   int64     `dynamorm:"ttl" json:"expires_at"`
}

// Settlement represents a batch settlement
type Settlement struct {
	ProcessedAt      time.Time          `json:"processed_at,omitempty"`
	CreatedAt        time.Time          `dynamorm:"created_at" json:"created_at"`
	UpdatedAt        time.Time          `dynamorm:"updated_at" json:"updated_at"`
	ID               string             `dynamorm:"pk" json:"id"`
	MerchantID       string             `dynamorm:"index:gsi-merchant,pk" json:"merchant_id"`
	Date             string             `dynamorm:"index:gsi-merchant,sk" json:"date"`
	Status           string             `json:"status"`
	BatchID          string             `json:"batch_id"`
	Transactions     []SettlementDetail `dynamorm:"json" json:"transactions"`
	TotalAmount      int64              `json:"total_amount"`
	TransactionCount int                `json:"transaction_count"`
}

// SettlementDetail represents a transaction in a settlement
type SettlementDetail struct {
	PaymentID     string `json:"payment_id"`
	TransactionID string `json:"transaction_id"`
	Amount        int64  `json:"amount"`
	Fee           int64  `json:"fee"`
	NetAmount     int64  `json:"net_amount"`
}

// Webhook represents a webhook delivery attempt
type Webhook struct {
	CreatedAt    time.Time      `dynamorm:"created_at" json:"created_at"`
	NextRetry    time.Time      `dynamorm:"index:gsi-retry" json:"next_retry,omitempty"`
	LastAttempt  time.Time      `json:"last_attempt,omitempty"`
	Payload      map[string]any `dynamorm:"json" json:"payload"`
	Status       string         `json:"status"`
	URL          string         `json:"url"`
	PaymentID    string         `json:"payment_id,omitempty"`
	EventType    string         `dynamorm:"index:gsi-merchant,sk,prefix:event" json:"event_type"`
	ID           string         `dynamorm:"pk" json:"id"`
	ResponseBody string         `json:"response_body,omitempty"`
	MerchantID   string         `dynamorm:"index:gsi-merchant,pk" json:"merchant_id"`
	Attempts     int            `json:"attempts"`
	ResponseCode int            `json:"response_code,omitempty"`
	ExpiresAt    int64          `dynamorm:"ttl" json:"expires_at"`
}

// WebhookStatus constants
const (
	WebhookStatusPending   = "pending"
	WebhookStatusDelivered = "delivered"
	WebhookStatusFailed    = "failed"
	WebhookStatusExpired   = "expired"
)

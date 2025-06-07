package models

import (
	"time"
)

// Product represents a product in the catalog
type Product struct {
	ID           string            `dynamorm:"pk" json:"id"`
	SKU          string            `dynamorm:"index:gsi-sku,unique" json:"sku"`
	CategoryID   string            `dynamorm:"index:gsi-category,pk" json:"category_id"`
	Name         string            `dynamorm:"index:gsi-category,sk" json:"name"`
	Description  string            `json:"description"`
	Price        int               `json:"price"`                   // Price in cents
	ComparePrice int               `json:"compare_price,omitempty"` // Original price for sales
	Cost         int               `json:"cost,omitempty"`          // Cost to business
	Stock        int               `json:"stock"`
	Reserved     int               `json:"reserved"` // Reserved for pending orders
	Status       string            `json:"status"`   // active, inactive, discontinued
	Images       []ProductImage    `dynamorm:"json" json:"images"`
	Variants     []ProductVariant  `dynamorm:"json" json:"variants"`
	Tags         []string          `dynamorm:"set" json:"tags"`
	Attributes   map[string]string `dynamorm:"json" json:"attributes,omitempty"`
	Weight       int               `json:"weight,omitempty"` // Weight in grams
	Featured     bool              `json:"featured"`
	CreatedAt    time.Time         `dynamorm:"created_at" json:"created_at"`
	UpdatedAt    time.Time         `dynamorm:"updated_at" json:"updated_at"`
	Version      int               `dynamorm:"version" json:"version"`
}

// ProductStatus constants
const (
	ProductStatusActive       = "active"
	ProductStatusInactive     = "inactive"
	ProductStatusDiscontinued = "discontinued"
)

// ProductImage represents a product image
type ProductImage struct {
	URL      string `json:"url"`
	Alt      string `json:"alt"`
	Position int    `json:"position"`
}

// ProductVariant represents a product variant (size, color, etc)
type ProductVariant struct {
	ID         string            `json:"id"`
	SKU        string            `json:"sku"`
	Name       string            `json:"name"`
	Price      int               `json:"price,omitempty"` // Override price
	Stock      int               `json:"stock"`
	Attributes map[string]string `json:"attributes"` // size: "L", color: "Blue"
	Weight     int               `json:"weight,omitempty"`
	Barcode    string            `json:"barcode,omitempty"`
}

// Category represents a product category
type Category struct {
	ID          string    `dynamorm:"pk" json:"id"`
	Slug        string    `dynamorm:"index:gsi-slug,unique" json:"slug"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	ParentID    string    `json:"parent_id,omitempty"`
	Image       string    `json:"image,omitempty"`
	Position    int       `json:"position"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `dynamorm:"created_at" json:"created_at"`
	UpdatedAt   time.Time `dynamorm:"updated_at" json:"updated_at"`
}

// Cart represents a shopping cart
type Cart struct {
	ID         string     `dynamorm:"pk" json:"id"`
	SessionID  string     `dynamorm:"index:gsi-session,unique" json:"session_id"`
	CustomerID string     `dynamorm:"index:gsi-customer" json:"customer_id,omitempty"`
	Items      []CartItem `dynamorm:"json" json:"items"`
	Subtotal   int        `json:"subtotal"` // Sum of item prices
	Discount   int        `json:"discount,omitempty"`
	Tax        int        `json:"tax,omitempty"`
	Total      int        `json:"total"`
	Currency   string     `json:"currency"`
	ExpiresAt  time.Time  `dynamorm:"ttl" json:"expires_at"` // TTL for abandoned carts
	CreatedAt  time.Time  `dynamorm:"created_at" json:"created_at"`
	UpdatedAt  time.Time  `dynamorm:"updated_at" json:"updated_at"`
}

// CartItem represents an item in the cart
type CartItem struct {
	ProductID  string            `json:"product_id"`
	VariantID  string            `json:"variant_id,omitempty"`
	SKU        string            `json:"sku"`
	Name       string            `json:"name"`
	Price      int               `json:"price"`
	Quantity   int               `json:"quantity"`
	Subtotal   int               `json:"subtotal"`
	Image      string            `json:"image,omitempty"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// Customer represents a customer
type Customer struct {
	ID                string           `dynamorm:"pk" json:"id"`
	Email             string           `dynamorm:"index:gsi-email,unique" json:"email"`
	Phone             string           `dynamorm:"index:gsi-phone" json:"phone,omitempty"`
	FirstName         string           `json:"first_name"`
	LastName          string           `json:"last_name"`
	PasswordHash      string           `json:"-"`
	EmailVerified     bool             `json:"email_verified"`
	PhoneVerified     bool             `json:"phone_verified"`
	DefaultAddressID  string           `json:"default_address_id,omitempty"`
	Addresses         []Address        `dynamorm:"json" json:"addresses"`
	PaymentMethods    []PaymentMethod  `dynamorm:"json" json:"payment_methods"`
	Tags              []string         `dynamorm:"set" json:"tags"`
	OrderCount        int              `json:"order_count"`
	TotalSpent        int              `json:"total_spent"`
	AverageOrderValue int              `json:"average_order_value"`
	LastOrderDate     time.Time        `json:"last_order_date,omitempty"`
	AcceptsMarketing  bool             `json:"accepts_marketing"`
	MarketingConsent  MarketingConsent `dynamorm:"json" json:"marketing_consent"`
	CreatedAt         time.Time        `dynamorm:"created_at" json:"created_at"`
	UpdatedAt         time.Time        `dynamorm:"updated_at" json:"updated_at"`
	Version           int              `dynamorm:"version" json:"version"`
}

// Address represents a customer address
type Address struct {
	ID         string `json:"id"`
	Type       string `json:"type"` // billing, shipping
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	Company    string `json:"company,omitempty"`
	Address1   string `json:"address1"`
	Address2   string `json:"address2,omitempty"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
	Phone      string `json:"phone,omitempty"`
	IsDefault  bool   `json:"is_default"`
}

// PaymentMethod represents a saved payment method
type PaymentMethod struct {
	ID        string `json:"id"`
	Type      string `json:"type"` // card, paypal, etc
	Last4     string `json:"last4,omitempty"`
	Brand     string `json:"brand,omitempty"`
	ExpMonth  int    `json:"exp_month,omitempty"`
	ExpYear   int    `json:"exp_year,omitempty"`
	IsDefault bool   `json:"is_default"`
	Token     string `json:"-"` // Encrypted payment token
}

// MarketingConsent tracks marketing preferences
type MarketingConsent struct {
	Email       bool      `json:"email"`
	SMS         bool      `json:"sms"`
	ConsentDate time.Time `json:"consent_date"`
	IP          string    `json:"ip,omitempty"`
}

// Order represents a customer order
type Order struct {
	ID              string      `dynamorm:"pk" json:"id"`
	OrderNumber     string      `dynamorm:"index:gsi-order-number,unique" json:"order_number"`
	CustomerID      string      `dynamorm:"index:gsi-customer,pk" json:"customer_id"`
	OrderDate       time.Time   `dynamorm:"index:gsi-customer,sk" json:"order_date"`
	Status          string      `dynamorm:"index:gsi-status-date,pk" json:"status"`
	StatusDate      time.Time   `dynamorm:"index:gsi-status-date,sk" json:"status_date"`
	Email           string      `json:"email"`
	Phone           string      `json:"phone,omitempty"`
	Items           []OrderItem `dynamorm:"json" json:"items"`
	ShippingAddress Address     `dynamorm:"json" json:"shipping_address"`
	BillingAddress  Address     `dynamorm:"json" json:"billing_address"`
	Subtotal        int         `json:"subtotal"`
	ShippingCost    int         `json:"shipping_cost"`
	Tax             int         `json:"tax"`
	Discount        int         `json:"discount,omitempty"`
	Total           int         `json:"total"`
	Currency        string      `json:"currency"`
	PaymentStatus   string      `json:"payment_status"`
	PaymentMethod   string      `json:"payment_method"`
	PaymentID       string      `json:"payment_id,omitempty"`
	ShippingMethod  string      `json:"shipping_method"`
	TrackingNumber  string      `json:"tracking_number,omitempty"`
	Notes           string      `json:"notes,omitempty"`
	Tags            []string    `dynamorm:"set" json:"tags"`
	RefundAmount    int         `json:"refund_amount,omitempty"`
	RefundReason    string      `json:"refund_reason,omitempty"`
	CancelReason    string      `json:"cancel_reason,omitempty"`
	FulfillmentDate time.Time   `json:"fulfillment_date,omitempty"`
	CreatedAt       time.Time   `dynamorm:"created_at" json:"created_at"`
	UpdatedAt       time.Time   `dynamorm:"updated_at" json:"updated_at"`
	Version         int         `dynamorm:"version" json:"version"`
}

// OrderStatus constants
const (
	OrderStatusPending    = "pending"
	OrderStatusProcessing = "processing"
	OrderStatusShipped    = "shipped"
	OrderStatusDelivered  = "delivered"
	OrderStatusCancelled  = "cancelled"
	OrderStatusRefunded   = "refunded"
)

// PaymentStatus constants
const (
	PaymentStatusPending   = "pending"
	PaymentStatusPaid      = "paid"
	PaymentStatusFailed    = "failed"
	PaymentStatusRefunded  = "refunded"
	PaymentStatusPartially = "partially_refunded"
)

// OrderItem represents an item in an order
type OrderItem struct {
	ProductID         string            `json:"product_id"`
	VariantID         string            `json:"variant_id,omitempty"`
	SKU               string            `json:"sku"`
	Name              string            `json:"name"`
	Price             int               `json:"price"`
	Quantity          int               `json:"quantity"`
	Subtotal          int               `json:"subtotal"`
	Image             string            `json:"image,omitempty"`
	Attributes        map[string]string `json:"attributes,omitempty"`
	FulfillmentStatus string            `json:"fulfillment_status,omitempty"`
	RefundedQuantity  int               `json:"refunded_quantity,omitempty"`
}

// Inventory represents product inventory
type Inventory struct {
	ID              string    `dynamorm:"pk,composite:product_id,location_id" json:"id"`
	ProductID       string    `dynamorm:"extract:product_id" json:"product_id"`
	LocationID      string    `dynamorm:"extract:location_id" json:"location_id"`
	VariantID       string    `json:"variant_id,omitempty"`
	Available       int       `json:"available"`
	Reserved        int       `json:"reserved"`
	Incoming        int       `json:"incoming"`
	ReorderPoint    int       `json:"reorder_point"`
	ReorderQuantity int       `json:"reorder_quantity"`
	LastRestocked   time.Time `json:"last_restocked,omitempty"`
	UpdatedAt       time.Time `dynamorm:"updated_at" json:"updated_at"`
	Version         int       `dynamorm:"version" json:"version"`
}

// InventoryMovement tracks inventory changes
type InventoryMovement struct {
	ID            string    `dynamorm:"pk" json:"id"`
	ProductID     string    `dynamorm:"index:gsi-product,pk" json:"product_id"`
	Timestamp     time.Time `dynamorm:"index:gsi-product,sk" json:"timestamp"`
	LocationID    string    `json:"location_id"`
	VariantID     string    `json:"variant_id,omitempty"`
	Type          string    `json:"type"`           // sale, return, adjustment, transfer
	Quantity      int       `json:"quantity"`       // Positive or negative
	ReferenceType string    `json:"reference_type"` // order, return, adjustment
	ReferenceID   string    `json:"reference_id"`
	Notes         string    `json:"notes,omitempty"`
	UserID        string    `json:"user_id"`
	CreatedAt     time.Time `dynamorm:"created_at" json:"created_at"`
}

// Discount represents a discount code or promotion
type Discount struct {
	ID                   string    `dynamorm:"pk" json:"id"`
	Code                 string    `dynamorm:"index:gsi-code,unique" json:"code"`
	Type                 string    `json:"type"`  // percentage, fixed_amount, free_shipping
	Value                int       `json:"value"` // Percentage or cents
	MinimumAmount        int       `json:"minimum_amount,omitempty"`
	UsageLimit           int       `json:"usage_limit,omitempty"`
	UsageCount           int       `json:"usage_count"`
	CustomerLimit        int       `json:"customer_limit,omitempty"` // Per customer
	StartDate            time.Time `json:"start_date"`
	EndDate              time.Time `json:"end_date"`
	Active               bool      `json:"active"`
	ApplicableProducts   []string  `dynamorm:"set" json:"applicable_products,omitempty"`
	ApplicableCategories []string  `dynamorm:"set" json:"applicable_categories,omitempty"`
	ExcludedProducts     []string  `dynamorm:"set" json:"excluded_products,omitempty"`
	CustomerTags         []string  `dynamorm:"set" json:"customer_tags,omitempty"`
	CreatedAt            time.Time `dynamorm:"created_at" json:"created_at"`
	UpdatedAt            time.Time `dynamorm:"updated_at" json:"updated_at"`
}

// Review represents a product review
type Review struct {
	ID           string    `dynamorm:"pk" json:"id"`
	ProductID    string    `dynamorm:"index:gsi-product,pk" json:"product_id"`
	Rating       int       `dynamorm:"index:gsi-product,sk,prefix:rating" json:"rating"`
	CustomerID   string    `dynamorm:"index:gsi-customer" json:"customer_id"`
	OrderID      string    `json:"order_id"`
	Title        string    `json:"title"`
	Content      string    `json:"content"`
	Verified     bool      `json:"verified"` // Verified purchase
	Helpful      int       `json:"helpful_count"`
	Images       []string  `dynamorm:"set" json:"images,omitempty"`
	Status       string    `json:"status"`             // pending, approved, rejected
	Response     string    `json:"response,omitempty"` // Merchant response
	ResponseDate time.Time `json:"response_date,omitempty"`
	CreatedAt    time.Time `dynamorm:"created_at" json:"created_at"`
	UpdatedAt    time.Time `dynamorm:"updated_at" json:"updated_at"`
}

// Wishlist represents a customer's wishlist
type Wishlist struct {
	ID         string         `dynamorm:"pk" json:"id"`
	CustomerID string         `dynamorm:"index:gsi-customer,unique" json:"customer_id"`
	Name       string         `json:"name"`
	Items      []WishlistItem `dynamorm:"json" json:"items"`
	IsPublic   bool           `json:"is_public"`
	ShareToken string         `json:"share_token,omitempty"`
	CreatedAt  time.Time      `dynamorm:"created_at" json:"created_at"`
	UpdatedAt  time.Time      `dynamorm:"updated_at" json:"updated_at"`
}

// WishlistItem represents an item in a wishlist
type WishlistItem struct {
	ProductID string    `json:"product_id"`
	VariantID string    `json:"variant_id,omitempty"`
	AddedAt   time.Time `json:"added_at"`
	Priority  int       `json:"priority,omitempty"`
	Notes     string    `json:"notes,omitempty"`
}

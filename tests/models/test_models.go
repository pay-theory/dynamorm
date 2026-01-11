package models

import "time"

// TestUser is a test model for user data
type TestUser struct {
	CreatedAt time.Time `dynamorm:"index:gsi-email,sk"`
	ID        string    `dynamorm:"pk"`
	Email     string    `dynamorm:"sk,index:gsi-email,pk"`
	Status    string    `dynamorm:""`
	Name      string    `dynamorm:""`
	Tags      []string  `dynamorm:""`
	Age       int       `dynamorm:""`
}

// TestProduct is a test model for product data
type TestProduct struct {
	CreatedAt   time.Time `dynamorm:""`
	SKU         string    `dynamorm:"pk"`
	Category    string    `dynamorm:"sk,index:gsi-category,pk"`
	Name        string    `dynamorm:""`
	Description string    `dynamorm:""`
	Price       float64   `dynamorm:"index:gsi-category,sk"`
	InStock     bool      `dynamorm:""`
}

// TestOrder is a test model for complex queries
type TestOrder struct {
	CreatedAt  time.Time   `dynamorm:"index:gsi-customer,sk"`
	UpdatedAt  time.Time   `dynamorm:""`
	OrderID    string      `dynamorm:"pk"`
	CustomerID string      `dynamorm:"sk,index:gsi-customer,pk"`
	Status     string      `dynamorm:"index:gsi-status,pk"`
	Items      []OrderItem `dynamorm:""`
	Total      float64     `dynamorm:"index:gsi-status,sk"`
}

// OrderItem represents an item in an order
type OrderItem struct {
	ProductSKU string  `dynamorm:""`
	Quantity   int     `dynamorm:""`
	Price      float64 `dynamorm:""`
}

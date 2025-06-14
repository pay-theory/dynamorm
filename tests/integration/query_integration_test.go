package integration

import (
	"fmt"
	"testing"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/tests/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedQueryTestData seeds test data for query tests
func seedQueryTestData(t *testing.T, testCtx *TestContext) {
	// Seed users
	users := []models.TestUser{
		{ID: "user-1", Email: "john@example.com", CreatedAt: time.Now().Add(-48 * time.Hour), Age: 25, Status: "active", Tags: []string{"premium", "verified"}, Name: "John Doe"},
		{ID: "user-2", Email: "jane@example.com", CreatedAt: time.Now().Add(-24 * time.Hour), Age: 30, Status: "active", Tags: []string{"verified"}, Name: "Jane Smith"},
		{ID: "user-3", Email: "admin@example.com", CreatedAt: time.Now().Add(-12 * time.Hour), Age: 35, Status: "admin", Tags: []string{"admin", "verified"}, Name: "Admin User"},
		{ID: "user-4", Email: "inactive@example.com", CreatedAt: time.Now().Add(-72 * time.Hour), Age: 28, Status: "inactive", Tags: []string{}, Name: "Inactive User"},
	}

	for _, user := range users {
		err := testCtx.DB.Model(&user).Create()
		require.NoError(t, err)
	}

	// Seed products
	products := []models.TestProduct{
		{SKU: "ELEC-001", Category: "electronics", Price: 299.99, Name: "Laptop", Description: "High-performance laptop", InStock: true, CreatedAt: time.Now()},
		{SKU: "ELEC-002", Category: "electronics", Price: 599.99, Name: "Smartphone", Description: "Latest smartphone", InStock: true, CreatedAt: time.Now()},
		{SKU: "BOOK-001", Category: "books", Price: 19.99, Name: "Programming Book", Description: "Learn programming", InStock: true, CreatedAt: time.Now()},
		{SKU: "BOOK-002", Category: "books", Price: 14.99, Name: "Novel", Description: "Bestselling novel", InStock: false, CreatedAt: time.Now()},
	}

	for _, product := range products {
		err := testCtx.DB.Model(&product).Create()
		require.NoError(t, err)
	}

	// Seed orders
	orders := []models.TestOrder{
		{
			OrderID: "ORD-001", CustomerID: "user-1", Status: "completed", Total: 319.98,
			Items: []models.OrderItem{
				{ProductSKU: "ELEC-001", Quantity: 1, Price: 299.99},
				{ProductSKU: "BOOK-001", Quantity: 1, Price: 19.99},
			},
			CreatedAt: time.Now().Add(-24 * time.Hour),
			UpdatedAt: time.Now(),
		},
		{
			OrderID: "ORD-002", CustomerID: "user-2", Status: "pending", Total: 614.98,
			Items: []models.OrderItem{
				{ProductSKU: "ELEC-002", Quantity: 1, Price: 599.99},
				{ProductSKU: "BOOK-002", Quantity: 1, Price: 14.99},
			},
			CreatedAt: time.Now().Add(-1 * time.Hour),
			UpdatedAt: time.Now(),
		},
	}

	for _, order := range orders {
		err := testCtx.DB.Model(&order).Create()
		require.NoError(t, err)
	}
}

func TestComplexQueryWithIndexSelection(t *testing.T) {
	testCtx := InitTestDB(t)

	// Create tables
	testCtx.CreateTableIfNotExists(t, &models.TestUser{})
	testCtx.CreateTableIfNotExists(t, &models.TestProduct{})
	testCtx.CreateTableIfNotExists(t, &models.TestOrder{})

	// Seed test data
	seedQueryTestData(t, testCtx)

	// Test that queries automatically select the right index
	var user models.TestUser

	// This should use gsi-email index
	err := testCtx.DB.Model(&models.TestUser{}).
		Where("Email", "=", "john@example.com").
		First(&user)

	assert.NoError(t, err)
	assert.Equal(t, "john@example.com", user.Email)

	// Test with category index on products
	var products []models.TestProduct
	err = testCtx.DB.Model(&models.TestProduct{}).
		Where("Category", "=", "electronics").
		Where("Price", "<", 500.00).
		All(&products)

	assert.NoError(t, err)
	assert.Len(t, products, 1)
	assert.Equal(t, "ELEC-001", products[0].SKU)
}

func TestBatchOperationsWithLimits(t *testing.T) {
	testCtx := InitTestDB(t)

	// Create tables
	testCtx.CreateTableIfNotExists(t, &models.TestUser{})

	// Create many items to test batch limits
	for i := 0; i < 30; i++ {
		user := models.TestUser{
			ID:        fmt.Sprintf("batch-user-%d", i),
			Email:     fmt.Sprintf("batch%d@example.com", i),
			CreatedAt: time.Now(),
			Age:       20 + i,
			Status:    "active",
			Name:      fmt.Sprintf("Batch User %d", i),
		}
		err := testCtx.DB.Model(&user).Create()
		assert.NoError(t, err)
	}

	// Query with limit
	var users []models.TestUser
	err := testCtx.DB.Model(&models.TestUser{}).
		Where("Status", "=", "active").
		Limit(10).
		All(&users)

	assert.NoError(t, err)
	assert.LessOrEqual(t, len(users), 10)
}

func TestPaginationAcrossMultiplePages(t *testing.T) {
	testCtx := InitTestDB(t)

	// Create tables
	testCtx.CreateTableIfNotExists(t, &models.TestProduct{})

	// Create 50 items for pagination testing
	for i := 0; i < 50; i++ {
		product := models.TestProduct{
			SKU:         fmt.Sprintf("PAGE-%03d", i),
			Category:    "pagination-test",
			Price:       float64(i) * 10.0,
			Name:        fmt.Sprintf("Page Product %d", i),
			Description: "Test product for pagination",
			InStock:     true,
			CreatedAt:   time.Now(),
		}
		err := testCtx.DB.Model(&product).Create()
		assert.NoError(t, err)
	}

	// Test pagination using limit and offset for now
	// TODO: Implement cursor-based pagination in Priority 2

	// First page
	var firstPage []models.TestProduct
	err := testCtx.DB.Model(&models.TestProduct{}).
		Where("Category", "=", "pagination-test").
		Limit(10).
		OrderBy("Price", "asc").
		All(&firstPage)

	assert.NoError(t, err)
	assert.Len(t, firstPage, 10)

	// Second page using offset
	var secondPage []models.TestProduct
	err = testCtx.DB.Model(&models.TestProduct{}).
		Where("Category", "=", "pagination-test").
		Limit(10).
		Offset(10).
		OrderBy("Price", "asc").
		All(&secondPage)

	assert.NoError(t, err)
	assert.Len(t, secondPage, 10)

	// Verify no duplicates between pages
	firstPageSKUs := make(map[string]bool)
	for _, p := range firstPage {
		firstPageSKUs[p.SKU] = true
	}

	for _, p := range secondPage {
		assert.False(t, firstPageSKUs[p.SKU], "Found duplicate SKU across pages: %s", p.SKU)
	}
}

func TestComplexFilters(t *testing.T) {
	testCtx := InitTestDB(t)

	// Create tables
	testCtx.CreateTableIfNotExists(t, &models.TestUser{})
	testCtx.CreateTableIfNotExists(t, &models.TestProduct{})
	testCtx.CreateTableIfNotExists(t, &models.TestOrder{})

	// Seed test data
	seedQueryTestData(t, testCtx)

	// Test multiple filter conditions
	var users []models.TestUser
	err := testCtx.DB.Model(&models.TestUser{}).
		Where("Status", "=", "active").
		Filter("Age", ">", 20).
		Filter("Age", "<", 35).
		All(&users)

	assert.NoError(t, err)
	assert.NotEmpty(t, users)
	for _, user := range users {
		assert.Greater(t, user.Age, 20)
		assert.Less(t, user.Age, 35)
		assert.Equal(t, "active", user.Status)
	}
}

func TestINOperator(t *testing.T) {
	testCtx := InitTestDB(t)

	// Create tables
	testCtx.CreateTableIfNotExists(t, &models.TestUser{})
	testCtx.CreateTableIfNotExists(t, &models.TestProduct{})
	testCtx.CreateTableIfNotExists(t, &models.TestOrder{})

	// Seed test data
	seedQueryTestData(t, testCtx)

	var users []models.TestUser
	err := testCtx.DB.Model(&models.TestUser{}).
		Where("Status", "in", []string{"active", "admin"}).
		All(&users)

	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(users), 3) // We have at least 3 users with these statuses

	for _, user := range users {
		assert.Contains(t, []string{"active", "admin"}, user.Status)
	}
}

func TestContainsOperator(t *testing.T) {
	testCtx := InitTestDB(t)

	// Create tables
	testCtx.CreateTableIfNotExists(t, &models.TestUser{})
	testCtx.CreateTableIfNotExists(t, &models.TestProduct{})
	testCtx.CreateTableIfNotExists(t, &models.TestOrder{})

	// Seed test data
	seedQueryTestData(t, testCtx)

	// Note: The contains operator might need special handling
	// For now, let's use a Where clause with contains
	var users []models.TestUser
	err := testCtx.DB.Model(&models.TestUser{}).
		Where("Tags", "contains", "verified").
		All(&users)

	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(users), 3) // We have at least 3 verified users

	for _, user := range users {
		assert.Contains(t, user.Tags, "verified")
	}
}

func TestQueryProjections(t *testing.T) {
	testCtx := InitTestDB(t)

	// Create tables
	testCtx.CreateTableIfNotExists(t, &models.TestUser{})
	testCtx.CreateTableIfNotExists(t, &models.TestProduct{})
	testCtx.CreateTableIfNotExists(t, &models.TestOrder{})

	// Seed test data
	seedQueryTestData(t, testCtx)

	var users []models.TestUser
	err := testCtx.DB.Model(&models.TestUser{}).
		Where("Status", "=", "active").
		Select("ID", "Email", "Name").
		All(&users)

	assert.NoError(t, err)
	assert.NotEmpty(t, users)

	// Verify selected fields are populated
	for _, user := range users {
		assert.NotEmpty(t, user.ID)
		assert.NotEmpty(t, user.Email)
		assert.NotEmpty(t, user.Name)
		// Age should be zero value since not selected
		assert.Zero(t, user.Age)
	}
}

func TestTransactionQueries(t *testing.T) {
	testCtx := InitTestDB(t)

	// Create tables
	testCtx.CreateTableIfNotExists(t, &models.TestUser{})
	testCtx.CreateTableIfNotExists(t, &models.TestProduct{})
	testCtx.CreateTableIfNotExists(t, &models.TestOrder{})

	// Seed test data
	seedQueryTestData(t, testCtx)

	// Test query within transaction
	err := testCtx.DB.Transaction(func(tx *core.Tx) error {
		var user models.TestUser
		err := tx.Model(&models.TestUser{}).
			Where("ID", "=", "user-1").
			First(&user)
		if err != nil {
			return err
		}

		// Update within same transaction
		user.Status = "premium"
		return tx.Model(&user).Update("Status")
	})

	assert.NoError(t, err)

	// Verify update
	var updated models.TestUser
	err = testCtx.DB.Model(&models.TestUser{}).
		Where("ID", "=", "user-1").
		First(&updated)
	assert.NoError(t, err)
	assert.Equal(t, "premium", updated.Status)
}

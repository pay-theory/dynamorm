package integration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/pay-theory/dynamorm"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/tests/models"
	"github.com/stretchr/testify/suite"
)

type QueryIntegrationSuite struct {
	suite.Suite
	db     *dynamorm.DB
	client *dynamodb.Client
	tables []string
}

func (s *QueryIntegrationSuite) SetupSuite() {
	// Initialize AWS config for DynamoDB Local
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion("us-east-1"),
		config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{
					URL:           "http://localhost:8000",
					SigningRegion: "us-east-1",
				}, nil
			})),
	)
	s.Require().NoError(err)

	// Initialize DynamoDB client
	s.client = dynamodb.NewFromConfig(cfg)

	// Initialize DynamORM
	s.db, err = dynamorm.New(dynamorm.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
	})
	s.Require().NoError(err)

	// Create test tables
	s.createTestTables()

	// Seed test data
	s.seedTestData()
}

func (s *QueryIntegrationSuite) TearDownSuite() {
	// Clean up tables
	for _, table := range s.tables {
		_, _ = s.client.DeleteTable(context.TODO(), &dynamodb.DeleteTableInput{
			TableName: aws.String(table),
		})
	}

	// Close DB connection
	if s.db != nil {
		s.db.Close()
	}
}

func (s *QueryIntegrationSuite) createTestTables() {
	// Create tables for each test model
	err := s.db.AutoMigrate(
		&models.TestUser{},
		&models.TestProduct{},
		&models.TestOrder{},
	)
	s.Require().NoError(err)

	s.tables = []string{"TestUsers", "TestProducts", "TestOrders"}

	// Wait for tables to be active
	for _, table := range s.tables {
		s.waitForTable(table)
	}
}

func (s *QueryIntegrationSuite) waitForTable(tableName string) {
	ctx := context.TODO()
	for i := 0; i < 30; i++ {
		desc, err := s.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
			TableName: aws.String(tableName),
		})
		if err == nil && desc.Table.TableStatus == "ACTIVE" {
			return
		}
		time.Sleep(1 * time.Second)
	}
	s.Fail("Table not active: " + tableName)
}

func (s *QueryIntegrationSuite) seedTestData() {
	// Seed users
	users := []models.TestUser{
		{ID: "user-1", Email: "john@example.com", CreatedAt: time.Now().Add(-48 * time.Hour), Age: 25, Status: "active", Tags: []string{"premium", "verified"}, Name: "John Doe"},
		{ID: "user-2", Email: "jane@example.com", CreatedAt: time.Now().Add(-24 * time.Hour), Age: 30, Status: "active", Tags: []string{"verified"}, Name: "Jane Smith"},
		{ID: "user-3", Email: "admin@example.com", CreatedAt: time.Now().Add(-12 * time.Hour), Age: 35, Status: "admin", Tags: []string{"admin", "verified"}, Name: "Admin User"},
		{ID: "user-4", Email: "inactive@example.com", CreatedAt: time.Now().Add(-72 * time.Hour), Age: 28, Status: "inactive", Tags: []string{}, Name: "Inactive User"},
	}

	for _, user := range users {
		err := s.db.Model(&user).Create()
		s.Require().NoError(err)
	}

	// Seed products
	products := []models.TestProduct{
		{SKU: "ELEC-001", Category: "electronics", Price: 299.99, Name: "Laptop", Description: "High-performance laptop", InStock: true, CreatedAt: time.Now()},
		{SKU: "ELEC-002", Category: "electronics", Price: 599.99, Name: "Smartphone", Description: "Latest smartphone", InStock: true, CreatedAt: time.Now()},
		{SKU: "BOOK-001", Category: "books", Price: 19.99, Name: "Programming Book", Description: "Learn programming", InStock: true, CreatedAt: time.Now()},
		{SKU: "BOOK-002", Category: "books", Price: 14.99, Name: "Novel", Description: "Bestselling novel", InStock: false, CreatedAt: time.Now()},
	}

	for _, product := range products {
		err := s.db.Model(&product).Create()
		s.Require().NoError(err)
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
		err := s.db.Model(&order).Create()
		s.Require().NoError(err)
	}
}

// Test Cases

func (s *QueryIntegrationSuite) TestComplexQueryWithIndexSelection() {
	// Test that queries automatically select the right index
	var users []models.TestUser

	// This should use gsi-email index
	err := s.db.Model(&models.TestUser{}).
		Where("Email", "=", "john@example.com").
		First(&users)

	s.NoError(err)
	s.Len(users, 1)
	s.Equal("john@example.com", users[0].Email)

	// Test with category index on products
	var products []models.TestProduct
	err = s.db.Model(&models.TestProduct{}).
		Where("Category", "=", "electronics").
		Where("Price", "<", 500.00).
		All(&products)

	s.NoError(err)
	s.Len(products, 1)
	s.Equal("ELEC-001", products[0].SKU)
}

func (s *QueryIntegrationSuite) TestBatchOperationsWithLimits() {
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
		err := s.db.Model(&user).Create()
		s.NoError(err)
	}

	// Query with limit
	var users []models.TestUser
	err := s.db.Model(&models.TestUser{}).
		Where("Status", "=", "active").
		Limit(10).
		All(&users)

	s.NoError(err)
	s.LessOrEqual(len(users), 10)
}

func (s *QueryIntegrationSuite) TestPaginationAcrossMultiplePages() {
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
		err := s.db.Model(&product).Create()
		s.NoError(err)
	}

	// Test pagination using limit and offset for now
	// TODO: Implement cursor-based pagination in Priority 2

	// First page
	var firstPage []models.TestProduct
	err := s.db.Model(&models.TestProduct{}).
		Where("Category", "=", "pagination-test").
		Limit(10).
		OrderBy("Price", "asc").
		All(&firstPage)

	s.NoError(err)
	s.Len(firstPage, 10)

	// Second page using offset
	var secondPage []models.TestProduct
	err = s.db.Model(&models.TestProduct{}).
		Where("Category", "=", "pagination-test").
		Limit(10).
		Offset(10).
		OrderBy("Price", "asc").
		All(&secondPage)

	s.NoError(err)
	s.Len(secondPage, 10)

	// Verify no duplicates between pages
	firstPageSKUs := make(map[string]bool)
	for _, p := range firstPage {
		firstPageSKUs[p.SKU] = true
	}

	for _, p := range secondPage {
		s.False(firstPageSKUs[p.SKU], "Found duplicate SKU across pages: %s", p.SKU)
	}
}

func (s *QueryIntegrationSuite) TestComplexFilters() {
	// Test multiple filter conditions
	var users []models.TestUser
	err := s.db.Model(&models.TestUser{}).
		Where("Status", "=", "active").
		Filter("Age > :minAge AND Age < :maxAge",
			core.Param{Name: "minAge", Value: 20},
			core.Param{Name: "maxAge", Value: 35}).
		All(&users)

	s.NoError(err)
	s.NotEmpty(users)
	for _, user := range users {
		s.Greater(user.Age, 20)
		s.Less(user.Age, 35)
		s.Equal("active", user.Status)
	}
}

func (s *QueryIntegrationSuite) TestINOperator() {
	var users []models.TestUser
	err := s.db.Model(&models.TestUser{}).
		Where("Status", "in", []string{"active", "admin"}).
		All(&users)

	s.NoError(err)
	s.GreaterOrEqual(len(users), 3) // We have at least 3 users with these statuses

	for _, user := range users {
		s.Contains([]string{"active", "admin"}, user.Status)
	}
}

func (s *QueryIntegrationSuite) TestContainsOperator() {
	var users []models.TestUser
	err := s.db.Model(&models.TestUser{}).
		Filter("contains(Tags, :tag)", core.Param{Name: "tag", Value: "verified"}).
		All(&users)

	s.NoError(err)
	s.GreaterOrEqual(len(users), 3) // We have at least 3 verified users

	for _, user := range users {
		s.Contains(user.Tags, "verified")
	}
}

func (s *QueryIntegrationSuite) TestProjections() {
	var users []models.TestUser
	err := s.db.Model(&models.TestUser{}).
		Where("Status", "=", "active").
		Select("ID", "Email", "Name").
		All(&users)

	s.NoError(err)
	s.NotEmpty(users)

	// Verify selected fields are populated
	for _, user := range users {
		s.NotEmpty(user.ID)
		s.NotEmpty(user.Email)
		s.NotEmpty(user.Name)
		// Age should be zero value since not selected
		s.Zero(user.Age)
	}
}

func (s *QueryIntegrationSuite) TestTransactionQueries() {
	// Test query within transaction
	err := s.db.Transaction(func(tx *core.Tx) error {
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

	s.NoError(err)

	// Verify update
	var updated models.TestUser
	err = s.db.Model(&models.TestUser{}).
		Where("ID", "=", "user-1").
		First(&updated)
	s.NoError(err)
	s.Equal("premium", updated.Status)
}

func TestQueryIntegrationSuite(t *testing.T) {
	suite.Run(t, new(QueryIntegrationSuite))
}

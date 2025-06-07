package integration

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm"
	"github.com/pay-theory/dynamorm/pkg/schema"
	"github.com/pay-theory/dynamorm/pkg/transaction"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// User model for testing
type User struct {
	ID        string `dynamorm:"pk"`
	Name      string
	Email     string `dynamorm:"index:email-index,pk"`
	Balance   float64
	Status    string
	Version   int       `dynamorm:"version"`
	CreatedAt time.Time `dynamorm:"created_at"`
	UpdatedAt time.Time `dynamorm:"updated_at"`
}

// Product model for testing with composite key
type Product struct {
	ProductID  string `dynamorm:"pk"`
	CategoryID string `dynamorm:"sk"`
	Name       string
	Price      float64
	Stock      int
	LastSold   time.Time `dynamorm:"lsi:last-sold-index,sk"`
}

func TestCompleteWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Initialize DB
	db, err := dynamorm.New(dynamorm.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
	})
	require.NoError(t, err)

	// Clean up any existing tables
	_ = db.DeleteTable(&User{})
	_ = db.DeleteTable(&Product{})

	t.Run("CreateTables", func(t *testing.T) {
		// Create user table with custom options
		err = db.CreateTable(&User{},
			schema.WithBillingMode(types.BillingModePayPerRequest),
		)
		require.NoError(t, err)

		// Create product table
		err = db.CreateTable(&Product{})
		require.NoError(t, err)

		// Verify tables exist
		desc, err := db.DescribeTable(&User{})
		assert.NoError(t, err)
		assert.Equal(t, types.TableStatusActive, desc.TableStatus)

		desc, err = db.DescribeTable(&Product{})
		assert.NoError(t, err)
		assert.Equal(t, types.TableStatusActive, desc.TableStatus)
	})

	t.Run("BasicCRUDOperations", func(t *testing.T) {
		// Create a user
		user := &User{
			ID:      "user-1",
			Name:    "Alice",
			Email:   "alice@example.com",
			Balance: 100.0,
			Status:  "active",
		}

		err = db.Model(user).Create()
		require.NoError(t, err)

		// Read the user
		var fetchedUser User
		err = db.Model(&User{ID: "user-1"}).First(&fetchedUser)
		require.NoError(t, err)
		assert.Equal(t, "Alice", fetchedUser.Name)
		assert.Equal(t, float64(100), fetchedUser.Balance)

		// Update the user
		fetchedUser.Balance = 125.0
		err = db.Model(&fetchedUser).Update()
		require.NoError(t, err)

		// Verify update
		var updatedUser User
		err = db.Model(&User{ID: "user-1"}).First(&updatedUser)
		require.NoError(t, err)
		assert.Equal(t, float64(125), updatedUser.Balance)
		assert.Equal(t, 1, updatedUser.Version) // Version should be incremented

		// Create products
		products := []Product{
			{ProductID: "prod-1", CategoryID: "electronics", Name: "Laptop", Price: 999.99, Stock: 10},
			{ProductID: "prod-2", CategoryID: "electronics", Name: "Mouse", Price: 29.99, Stock: 100},
			{ProductID: "prod-3", CategoryID: "books", Name: "Go Programming", Price: 39.99, Stock: 50},
		}

		for _, p := range products {
			err = db.Model(&p).Create()
			require.NoError(t, err)
		}

		// Query products by category
		var electronics []Product
		err = db.Model(&Product{CategoryID: "electronics"}).
			Where("CategoryID", "=", "electronics").
			All(&electronics)
		require.NoError(t, err)
		assert.Len(t, electronics, 2)
	})

	t.Run("TransactionSupport", func(t *testing.T) {
		// Create two users for fund transfer
		user1 := &User{ID: "tx-user-1", Name: "Bob", Balance: 200.0}
		user2 := &User{ID: "tx-user-2", Name: "Charlie", Balance: 50.0}

		err = db.Model(user1).Create()
		require.NoError(t, err)
		err = db.Model(user2).Create()
		require.NoError(t, err)

		// Perform atomic fund transfer
		transferAmount := 25.0
		err = db.TransactionFunc(func(tx *transaction.Transaction) error {
			// Fetch current balances
			var u1, u2 User
			err := db.Model(&User{ID: "tx-user-1"}).First(&u1)
			if err != nil {
				return err
			}
			err = db.Model(&User{ID: "tx-user-2"}).First(&u2)
			if err != nil {
				return err
			}

			// Update balances
			u1.Balance -= transferAmount
			u2.Balance += transferAmount

			// Add updates to transaction
			if err := tx.Update(&u1); err != nil {
				return err
			}
			return tx.Update(&u2)
		})
		require.NoError(t, err)

		// Verify balances after transaction
		var afterUser1, afterUser2 User
		err = db.Model(&User{ID: "tx-user-1"}).First(&afterUser1)
		require.NoError(t, err)
		err = db.Model(&User{ID: "tx-user-2"}).First(&afterUser2)
		require.NoError(t, err)

		assert.Equal(t, 175.0, afterUser1.Balance)
		assert.Equal(t, 75.0, afterUser2.Balance)
	})

	t.Run("TransactionWithNewItems", func(t *testing.T) {
		// Create order and update inventory atomically
		err = db.TransactionFunc(func(tx *transaction.Transaction) error {
			// Create a new order
			order := &User{
				ID:      "order-1",
				Name:    "Order for prod-1",
				Balance: 999.99,
				Status:  "pending",
			}
			if err := tx.Create(order); err != nil {
				return err
			}

			// Update product stock
			var product Product
			err := db.Model(&Product{ProductID: "prod-1", CategoryID: "electronics"}).First(&product)
			if err != nil {
				return err
			}

			product.Stock -= 1
			product.LastSold = time.Now()

			return tx.Update(&product)
		})
		require.NoError(t, err)

		// Verify order was created
		var order User
		err = db.Model(&User{ID: "order-1"}).First(&order)
		require.NoError(t, err)
		assert.Equal(t, "pending", order.Status)

		// Verify stock was updated
		var product Product
		err = db.Model(&Product{ProductID: "prod-1", CategoryID: "electronics"}).First(&product)
		require.NoError(t, err)
		assert.Equal(t, 9, product.Stock)
	})

	t.Run("ConditionalTransactionFailure", func(t *testing.T) {
		// Try to create a user that already exists
		err = db.TransactionFunc(func(tx *transaction.Transaction) error {
			duplicate := &User{
				ID:   "user-1",
				Name: "Duplicate User",
			}
			return tx.Create(duplicate)
		})
		// Should fail due to conditional check
		assert.Error(t, err)
	})

	t.Run("QueryWithIndex", func(t *testing.T) {
		// Query by email using GSI
		var userByEmail User
		err = db.Model(&User{}).
			Index("email-index").
			Where("Email", "=", "alice@example.com").
			First(&userByEmail)
		require.NoError(t, err)
		assert.Equal(t, "Alice", userByEmail.Name)
	})

	t.Run("AutoMigrate", func(t *testing.T) {
		// Test AutoMigrate with existing tables
		err = db.AutoMigrate(&User{}, &Product{})
		assert.NoError(t, err) // Should not error on existing tables
	})

	t.Run("TableUpdateOptions", func(t *testing.T) {
		// This would normally update table settings, but DynamoDB Local
		// may have limitations on certain updates
		// Commenting out for local testing but this shows the API

		// err = db.UpdateTable(&User{},
		//     schema.WithStreamSpecification(types.StreamSpecification{
		//         StreamEnabled:  aws.Bool(true),
		//         StreamViewType: types.StreamViewTypeNewAndOldImages,
		//     }),
		// )
		// assert.NoError(t, err)
	})

	// Cleanup
	t.Cleanup(func() {
		_ = db.DeleteTable(&User{})
		_ = db.DeleteTable(&Product{})
	})
}

func TestEnsureTable(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db, err := dynamorm.New(dynamorm.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
	})
	require.NoError(t, err)

	// Clean up
	_ = db.DeleteTable(&User{})

	// EnsureTable should create if not exists
	err = db.EnsureTable(&User{})
	require.NoError(t, err)

	// Second call should not error
	err = db.EnsureTable(&User{})
	require.NoError(t, err)

	// Verify table exists
	desc, err := db.DescribeTable(&User{})
	assert.NoError(t, err)
	assert.NotNil(t, desc)

	// Cleanup
	_ = db.DeleteTable(&User{})
}

func TestBatchOperationsWithTransaction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	db, err := dynamorm.New(dynamorm.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
	})
	require.NoError(t, err)

	// Ensure table exists
	_ = db.DeleteTable(&User{})
	err = db.CreateTable(&User{})
	require.NoError(t, err)

	// Create multiple users in a transaction
	err = db.TransactionFunc(func(tx *transaction.Transaction) error {
		users := []User{
			{ID: "batch-1", Name: "User 1", Balance: 100},
			{ID: "batch-2", Name: "User 2", Balance: 200},
			{ID: "batch-3", Name: "User 3", Balance: 300},
			{ID: "batch-4", Name: "User 4", Balance: 400},
			{ID: "batch-5", Name: "User 5", Balance: 500},
		}

		for _, u := range users {
			if err := tx.Create(&u); err != nil {
				return err
			}
		}
		return nil
	})
	require.NoError(t, err)

	// Verify all users were created
	var allUsers []User
	err = db.Model(&User{}).Scan(&allUsers)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(allUsers), 5)

	// Cleanup
	_ = db.DeleteTable(&User{})
}

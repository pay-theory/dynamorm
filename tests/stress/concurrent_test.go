package stress

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/pay-theory/dynamorm"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/session"
	"github.com/pay-theory/dynamorm/tests"
	"github.com/pay-theory/dynamorm/tests/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConcurrentQueries tests system behavior under heavy concurrent load
func TestConcurrentQueries(t *testing.T) {
	db, err := setupStressDB(t)
	require.NoError(t, err)

	// Clean up any existing items
	for i := 0; i < 100; i++ {
		_ = db.Model(&models.TestUser{}).
			Where("ID", "=", fmt.Sprintf("concurrent-user-%d", i)).
			Delete()
	}

	// Create test data
	users := make([]*models.TestUser, 100)
	timestamp := time.Now()
	for i := 0; i < 100; i++ {
		users[i] = &models.TestUser{
			ID:        fmt.Sprintf("concurrent-user-%d", i),
			Email:     fmt.Sprintf("user%d@example.com", i),
			CreatedAt: timestamp.Add(time.Duration(i) * time.Minute),
			Age:       20 + i%50,
			Status:    "active",
			Tags:      []string{"test", fmt.Sprintf("group%d", i%5)},
			Name:      fmt.Sprintf("User %d", i),
		}
		assertSuccessfulCreation(t, db, users[i])
	}

	// Use our new test utility instead of testing.Short()
	tests.RequireDynamoDBLocal(t)

	// Number of concurrent goroutines
	concurrency := 100
	iterations := 10

	var wg sync.WaitGroup
	errors := make(chan error, concurrency*iterations)

	// Track memory usage
	startMem := getMemStats()

	start := time.Now()

	// Launch concurrent queries
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for j := 0; j < iterations; j++ {
				// Mix of different query types
				switch j % 5 {
				case 0:
					// Simple query
					var user models.TestUser
					err := db.Model(&models.TestUser{}).
						Where("ID", "=", fmt.Sprintf("concurrent-user-%d", workerID%100)).
						First(&user)
					if err != nil {
						errors <- fmt.Errorf("worker %d iteration %d: simple query failed: %w", workerID, j, err)
					}

				case 1:
					// Query with filter
					var users []models.TestUser
					err := db.Model(&models.TestUser{}).
						Where("Status", "=", "active").
						Filter("Age", ">", 25).
						Limit(10).
						All(&users)
					if err != nil {
						errors <- fmt.Errorf("worker %d iteration %d: filtered query failed: %w", workerID, j, err)
					}

				case 2:
					// Index query
					var users []models.TestUser
					err := db.Model(&models.TestUser{}).
						Index("gsi-email").
						Where("Email", "=", fmt.Sprintf("user%d@example.com", workerID%100)).
						All(&users)
					if err != nil {
						errors <- fmt.Errorf("worker %d iteration %d: index query failed: %w", workerID, j, err)
					}

				case 3:
					// Create operation
					user := models.TestUser{
						ID:        fmt.Sprintf("stress-temp-%d-%d-%d", workerID, j, time.Now().UnixNano()),
						Email:     fmt.Sprintf("temp%d-%d@example.com", workerID, j),
						CreatedAt: time.Now(),
						Status:    "active",
						Age:       25,
						Name:      fmt.Sprintf("Temp User %d-%d", workerID, j),
					}
					err := db.Model(&user).Create()
					if err != nil {
						errors <- fmt.Errorf("worker %d iteration %d: create failed: %w", workerID, j, err)
					}

				case 4:
					// Update operation
					err := db.Model(&models.TestUser{}).
						Where("ID", "=", fmt.Sprintf("concurrent-user-%d", workerID%100)).
						Update("Status")
					if err != nil {
						errors <- fmt.Errorf("worker %d iteration %d: update failed: %w", workerID, j, err)
					}
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errors)

	duration := time.Since(start)
	endMem := getMemStats()

	// Check for errors
	var errorCount int
	for err := range errors {
		t.Logf("Concurrent operation error: %v", err)
		errorCount++
	}

	// Verify results
	assert.Equal(t, 0, errorCount, "Expected no errors during concurrent operations")

	// Log performance metrics
	totalOps := concurrency * iterations
	opsPerSec := float64(totalOps) / duration.Seconds()
	memIncrease := endMem - startMem

	t.Logf("Concurrent test results:")
	t.Logf("- Total operations: %d", totalOps)
	t.Logf("- Duration: %v", duration)
	t.Logf("- Operations/sec: %.2f", opsPerSec)
	t.Logf("- Memory increase: %d MB", memIncrease/(1024*1024))

	// Verify memory usage is reasonable (less than 100MB increase)
	assert.Less(t, memIncrease, uint64(100*1024*1024), "Memory usage increased by more than 100MB")
}

// TestLargeItemHandling tests handling of items near DynamoDB limits
func TestLargeItemHandling(t *testing.T) {
	tests.RequireDynamoDBLocal(t)

	db, err := setupStressDB(t)
	require.NoError(t, err)

	t.Run("Large String Attributes", func(t *testing.T) {
		// Create a large string (300KB)
		largeString := generateLargeString(300 * 1024)

		// Using a custom type with Description field
		type LargeUser struct {
			ID          string    `dynamorm:"pk"`
			Email       string    `dynamorm:"index:gsi-email"`
			CreatedAt   time.Time `dynamorm:"sk"`
			Age         int       `dynamorm:""`
			Status      string    `dynamorm:""`
			Tags        []string  `dynamorm:""`
			Name        string    `dynamorm:""`
			Description string    `dynamorm:""`
		}

		// Create table for LargeUser
		err := db.AutoMigrate(&LargeUser{})
		require.NoError(t, err)

		// Clean up any existing item
		_ = db.Model(&LargeUser{}).
			Where("ID", "=", "large-string-user").
			Delete()

		// Use a fixed timestamp for both create and query
		timestamp := time.Now()

		user := LargeUser{
			ID:          "large-string-user",
			Email:       "large@example.com",
			CreatedAt:   timestamp,
			Status:      "active",
			Name:        "Large User",
			Description: largeString,
		}

		// Create item
		err = db.Model(&user).Create()
		assert.NoError(t, err)

		// Query it back
		var retrieved LargeUser
		err = db.Model(&LargeUser{}).
			Where("ID", "=", "large-string-user").
			Where("CreatedAt", "=", timestamp).
			First(&retrieved)
		assert.NoError(t, err)
		assert.Equal(t, len(largeString), len(retrieved.Description))
	})

	t.Run("Many Attributes", func(t *testing.T) {
		// Create item with 100+ attributes (using a map)
		type FlexibleItem struct {
			ID         string            `dynamorm:"pk"`
			Attributes map[string]string `dynamorm:""`
		}

		// Create table for FlexibleItem
		err := db.AutoMigrate(&FlexibleItem{})
		require.NoError(t, err)

		// Clean up any existing item
		_ = db.Model(&FlexibleItem{}).
			Where("ID", "=", "many-attributes-item").
			Delete()

		item := FlexibleItem{
			ID:         "many-attributes-item",
			Attributes: make(map[string]string),
		}

		// Add 100 attributes
		for i := 0; i < 100; i++ {
			item.Attributes[fmt.Sprintf("attr%d", i)] = fmt.Sprintf("value%d", i)
		}

		// Create item
		err = db.Model(&item).Create()
		assert.NoError(t, err)

		// Query it back
		var retrieved FlexibleItem
		err = db.Model(&FlexibleItem{}).
			Where("ID", "=", "many-attributes-item").
			First(&retrieved)
		assert.NoError(t, err)
		assert.Len(t, retrieved.Attributes, 100)
	})

	t.Run("Large Lists", func(t *testing.T) {
		// Create item with large list
		// Use a unique ID to avoid conflicts
		uniqueID := fmt.Sprintf("large-list-user-%d", time.Now().UnixNano())
		timestamp := time.Now()
		user := models.TestUser{
			ID:        uniqueID,
			Email:     "largelist@example.com",
			CreatedAt: timestamp,
			Status:    "active",
			Tags:      generateLargeList(1000), // 1000 tags
			Name:      "List User",
		}

		// Measure performance
		start := time.Now()
		err := db.Model(&user).Create()
		createTime := time.Since(start)
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		// Debug: print what we're querying for
		t.Logf("Querying for ID=%s, CreatedAt=%v", uniqueID, timestamp)

		// Query it back using Scan to bypass the GetItem issue
		start = time.Now()
		var retrieved models.TestUser
		var users []models.TestUser
		err = db.Model(&models.TestUser{}).
			Where("ID", "=", uniqueID).
			All(&users)
		if err != nil {
			t.Fatalf("Failed to query users: %v", err)
		}
		queryTime := time.Since(start)

		if len(users) > 0 {
			retrieved = users[0]
		} else {
			t.Fatalf("No users found with ID=%s", uniqueID)
		}

		assert.Len(t, retrieved.Tags, 1000)

		t.Logf("Large list performance - Create: %v, Query: %v", createTime, queryTime)

		// Verify performance doesn't degrade significantly
		assert.Less(t, createTime, 100*time.Millisecond, "Create took too long")
		assert.Less(t, queryTime, 50*time.Millisecond, "Query took too long")
	})
}

// TestMemoryStability tests for memory leaks under sustained load
func TestMemoryStability(t *testing.T) {
	// Skip if running quick tests
	if os.Getenv("SKIP_MEMORY_TEST") == "true" {
		t.Skip("Skipping memory stability test (SKIP_MEMORY_TEST=true)")
	}

	// Use our new test utility instead of testing.Short()
	tests.RequireDynamoDBLocal(t)

	db, err := setupStressDB(t)
	require.NoError(t, err)

	// Clean up any existing items
	for i := 0; i < 100; i++ {
		_ = db.Model(&models.TestUser{}).
			Where("ID", "=", fmt.Sprintf("mem-test-user-%d", i)).
			Delete()
	}

	// Create test data
	for i := 0; i < 100; i++ {
		user := &models.TestUser{
			ID:        fmt.Sprintf("mem-test-user-%d", i),
			Email:     fmt.Sprintf("memtest%d@example.com", i),
			CreatedAt: time.Now(),
			Status:    "active",
		}
		assertSuccessfulCreation(t, db, user)
	}

	// Run sustained load for 1 minute
	duration := 1 * time.Minute
	done := make(chan bool)
	errors := make(chan error, 1000)

	// Track memory samples
	memorySamples := []uint64{}
	sampleTicker := time.NewTicker(5 * time.Second)
	defer sampleTicker.Stop()

	go func() {
		for {
			select {
			case <-sampleTicker.C:
				memorySamples = append(memorySamples, getMemStats())
			case <-done:
				return
			}
		}
	}()

	// Start load generation
	start := time.Now()
	var opsCount int64

	for time.Since(start) < duration {
		// Random operation
		switch opsCount % 3 {
		case 0:
			// Query
			var user models.TestUser
			err := db.Model(&models.TestUser{}).
				Where("ID", "=", fmt.Sprintf("mem-test-user-%d", opsCount%100)).
				First(&user)
			if err != nil {
				errors <- err
			}

		case 1:
			// Scan with filter
			var users []models.TestUser
			err := db.Model(&models.TestUser{}).
				Filter("Age", ">", 20).
				Limit(20).
				Scan(&users)
			if err != nil {
				errors <- err
			}

		case 2:
			// Create
			user := models.TestUser{
				ID:        fmt.Sprintf("stability-user-%d-%d", opsCount, time.Now().UnixNano()),
				Email:     fmt.Sprintf("stability%d@example.com", opsCount),
				CreatedAt: time.Now(),
				Status:    "active",
				Age:       int(opsCount%50) + 20,
				Name:      fmt.Sprintf("Stability User %d", opsCount),
			}
			err := db.Model(&user).Create()
			if err != nil {
				errors <- err
			}
		}

		opsCount++

		// Small delay to prevent overwhelming
		if opsCount%100 == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	close(done)
	close(errors)

	// Check errors
	var errorCount int
	for err := range errors {
		t.Logf("Operation error: %v", err)
		errorCount++
	}

	// Analyze memory usage
	if len(memorySamples) > 2 {
		firstSample := memorySamples[0]
		lastSample := memorySamples[len(memorySamples)-1]
		avgSample := calculateAverage(memorySamples)

		memGrowth := float64(lastSample-firstSample) / float64(firstSample) * 100

		t.Logf("Memory stability results:")
		t.Logf("- Total operations: %d", opsCount)
		t.Logf("- Error count: %d", errorCount)
		t.Logf("- Initial memory: %d MB", firstSample/(1024*1024))
		t.Logf("- Final memory: %d MB", lastSample/(1024*1024))
		t.Logf("- Average memory: %d MB", avgSample/(1024*1024))
		t.Logf("- Memory growth: %.2f%%", memGrowth)

		// Verify memory growth is reasonable (less than 20%)
		assert.Less(t, memGrowth, 20.0, "Memory grew by more than 20%")
	}
}

// Helper functions

func setupStressDB(t *testing.T) (core.ExtendedDB, error) {
	// Fixed initialization with session.Config
	sessionConfig := session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
		AWSConfigOptions: []func(*config.LoadOptions) error{
			config.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider("dummy", "dummy", ""),
			),
			config.WithRegion("us-east-1"),
		},
	}

	db, err := dynamorm.New(sessionConfig)
	require.NoError(t, err)

	// Create test tables
	err = db.AutoMigrate(&models.TestUser{}, &models.TestProduct{})
	if err != nil {
		return nil, err
	}

	// Wait for tables to be active
	ctx := context.TODO()
	tables := []string{"TestUsers", "TestProducts"}

	// Get the DynamoDB client from the db session
	// We need to create a client from config since we can't access db's internal client
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion("us-east-1"),
		config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{
					URL:           "http://localhost:8000",
					SigningRegion: "us-east-1",
				}, nil
			})),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
	)
	if err != nil {
		return nil, err
	}

	client := dynamodb.NewFromConfig(cfg)

	for _, table := range tables {
		for i := 0; i < 30; i++ {
			desc, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
				TableName: aws.String(table),
			})
			if err == nil && desc.Table.TableStatus == "ACTIVE" {
				break
			}
			time.Sleep(1 * time.Second)
		}
	}

	return db, nil
}

func generateLargeString(size int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, size)
	for i := range b {
		b[i] = chars[i%len(chars)]
	}
	return string(b)
}

func generateLargeList(size int) []string {
	list := make([]string, size)
	for i := 0; i < size; i++ {
		list[i] = fmt.Sprintf("tag-%d", i)
	}
	return list
}

func getMemStats() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc
}

func calculateAverage(samples []uint64) uint64 {
	if len(samples) == 0 {
		return 0
	}
	var sum uint64
	for _, s := range samples {
		sum += s
	}
	return sum / uint64(len(samples))
}

func assertSuccessfulCreation(t *testing.T, db core.ExtendedDB, user *models.TestUser) {
	err := db.Model(user).Create()
	require.NoError(t, err)
}

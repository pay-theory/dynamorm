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
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/pay-theory/dynamorm"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/tests"
	"github.com/pay-theory/dynamorm/tests/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConcurrentQueries tests system behavior under heavy concurrent load
func TestConcurrentQueries(t *testing.T) {
	// Use our new test utility instead of testing.Short()
	tests.RequireDynamoDBLocal(t)

	db, err := setupStressDB(t)
	require.NoError(t, err)

	// Seed test data
	seedStressData(t, db, 100)

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
						Where("ID", "=", fmt.Sprintf("stress-user-%d", workerID%100)).
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
						Where("Email", "=", fmt.Sprintf("stress%d@example.com", workerID%100)).
						All(&users)
					if err != nil {
						errors <- fmt.Errorf("worker %d iteration %d: index query failed: %w", workerID, j, err)
					}

				case 3:
					// Create operation
					user := models.TestUser{
						ID:        fmt.Sprintf("stress-temp-%d-%d", workerID, j),
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
						Where("ID", "=", fmt.Sprintf("stress-user-%d", workerID%100)).
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
			models.TestUser
			Description string `dynamorm:""`
		}

		user := LargeUser{
			TestUser: models.TestUser{
				ID:        "large-string-user",
				Email:     "large@example.com",
				CreatedAt: time.Now(),
				Status:    "active",
				Name:      "Large User",
			},
			Description: largeString,
		}

		// Create item
		err := db.Model(&user).Create()
		assert.NoError(t, err)

		// Query it back
		var retrieved LargeUser
		err = db.Model(&LargeUser{}).
			Where("ID", "=", "large-string-user").
			First(&retrieved)
		assert.NoError(t, err)
		assert.Equal(t, len(largeString), len(retrieved.Description))
	})

	t.Run("Many Attributes", func(t *testing.T) {
		// Create item with 100+ attributes (using a map)
		type FlexibleItem struct {
			ID         string         `dynamorm:"pk"`
			Attributes map[string]any `dynamorm:""`
		}

		item := FlexibleItem{
			ID:         "many-attributes-item",
			Attributes: make(map[string]any),
		}

		// Add 100 attributes
		for i := 0; i < 100; i++ {
			item.Attributes[fmt.Sprintf("attr%d", i)] = fmt.Sprintf("value%d", i)
		}

		// Create item
		err := db.Model(&item).Create()
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
		user := models.TestUser{
			ID:        "large-list-user",
			Email:     "largelist@example.com",
			CreatedAt: time.Now(),
			Status:    "active",
			Tags:      generateLargeList(1000), // 1000 tags
			Name:      "List User",
		}

		// Measure performance
		start := time.Now()
		err := db.Model(&user).Create()
		createTime := time.Since(start)
		assert.NoError(t, err)

		// Query it back
		start = time.Now()
		var retrieved models.TestUser
		err = db.Model(&models.TestUser{}).
			Where("ID", "=", "large-list-user").
			First(&retrieved)
		queryTime := time.Since(start)
		assert.NoError(t, err)
		assert.Len(t, retrieved.Tags, 1000)

		t.Logf("Large list performance - Create: %v, Query: %v", createTime, queryTime)

		// Verify performance doesn't degrade significantly
		assert.Less(t, createTime, 100*time.Millisecond, "Create took too long")
		assert.Less(t, queryTime, 50*time.Millisecond, "Query took too long")
	})
}

// TestMemoryStability tests for memory leaks under sustained load
func TestMemoryStability(t *testing.T) {
	tests.RequireDynamoDBLocal(t)

	// This is a longer test, so allow skipping it specifically
	if os.Getenv("SKIP_MEMORY_TEST") == "true" {
		t.Skip("Skipping memory stability test (SKIP_MEMORY_TEST=true)")
	}

	db, err := setupStressDB(t)
	require.NoError(t, err)

	// Seed initial data
	seedStressData(t, db, 1000)

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
		switch opsCount % 4 {
		case 0:
			// Query
			var user models.TestUser
			err := db.Model(&models.TestUser{}).
				Where("ID", "=", fmt.Sprintf("stress-user-%d", opsCount%1000)).
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
				ID:        fmt.Sprintf("stability-user-%d", opsCount),
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

		case 3:
			// Delete
			err := db.Model(&models.TestUser{}).
				Where("ID", "=", fmt.Sprintf("stability-user-%d", opsCount-10)).
				Delete()
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
	// Initialize AWS config
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion("us-east-1"),
		config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...any) (aws.Endpoint, error) {
				return aws.Endpoint{
					URL:           "http://localhost:8000",
					SigningRegion: "us-east-1",
				}, nil
			})),
	)
	if err != nil {
		return nil, err
	}

	// Initialize DynamoDB client
	client := dynamodb.NewFromConfig(cfg)

	// Initialize DynamORM
	db, err := dynamorm.New(dynamorm.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
	})
	if err != nil {
		return nil, err
	}

	// Create test tables
	err = db.AutoMigrate(&models.TestUser{}, &models.TestProduct{})
	if err != nil {
		return nil, err
	}

	// Wait for tables to be active
	ctx := context.TODO()
	tables := []string{"TestUsers", "TestProducts"}
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

func seedStressData(t *testing.T, db core.ExtendedDB, count int) {
	for i := 0; i < count; i++ {
		user := models.TestUser{
			ID:        fmt.Sprintf("stress-user-%d", i),
			Email:     fmt.Sprintf("stress%d@example.com", i),
			CreatedAt: time.Now().Add(-time.Duration(i) * time.Hour),
			Status:    "active",
			Age:       20 + (i % 50),
			Tags:      []string{"stress", fmt.Sprintf("group%d", i%10)},
			Name:      fmt.Sprintf("Stress User %d", i),
		}
		err := db.Model(&user).Create()
		require.NoError(t, err)
	}
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

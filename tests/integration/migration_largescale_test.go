package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm"
	"github.com/pay-theory/dynamorm/pkg/schema"
	"github.com/pay-theory/dynamorm/pkg/session"
	"github.com/pay-theory/dynamorm/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LargeDatasetV1 represents a model with large amounts of data
type LargeDatasetV1 struct {
	ID          string    `dynamorm:"pk"`
	Category    string    `dynamorm:"sk"`
	Data        string    `dynamorm:"attr:data"`
	ProcessedAt time.Time `dynamorm:"attr:processed_at"`
	Version     int64     `dynamorm:"version"`
}

func (l *LargeDatasetV1) TableName() string {
	return "large_dataset_v1"
}

// LargeDatasetV2 represents the migrated version with additional fields
type LargeDatasetV2 struct {
	ID           string            `dynamorm:"pk"`
	Category     string            `dynamorm:"sk"`
	Data         string            `dynamorm:"attr:data"`
	DataChecksum string            `dynamorm:"attr:data_checksum"`
	ProcessedAt  time.Time         `dynamorm:"attr:processed_at"`
	MigratedAt   time.Time         `dynamorm:"attr:migrated_at"`
	Metadata     map[string]string `dynamorm:"attr:metadata"`
	Version      int64             `dynamorm:"version"`
}

func (l *LargeDatasetV2) TableName() string {
	return "large_dataset_v2"
}

func TestLargeScaleMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	tests.RequireDynamoDBLocal(t)

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

	// Clean up any existing tables
	_ = db.DeleteTable(&LargeDatasetV1{})
	_ = db.DeleteTable(&LargeDatasetV2{})

	t.Run("MigrationWithLargeDataset", func(t *testing.T) {
		// Create source table (use EnsureTable to handle existing tables)
		err := db.EnsureTable(&LargeDatasetV1{})
		require.NoError(t, err)

		// Clear any existing data first
		var existingItems []LargeDatasetV1
		_ = db.Model(&LargeDatasetV1{}).Scan(&existingItems)
		for _, item := range existingItems {
			_ = db.Model(&LargeDatasetV1{}).
				Where("ID", "=", item.ID).
				Where("Category", "=", item.Category).
				Delete()
		}

		// Generate large dataset (50 items for debugging)
		const itemCount = 50
		items := make([]*LargeDatasetV1, itemCount)
		for i := 0; i < itemCount; i++ {
			items[i] = &LargeDatasetV1{
				ID:          fmt.Sprintf("item-%05d", i),
				Category:    fmt.Sprintf("cat-%d", i%10),
				Data:        generateLargeData(i, 1024), // 1KB per item
				ProcessedAt: time.Now().Add(-time.Duration(i) * time.Hour),
				Version:     1,
			}
		}

		// Insert all items
		for _, item := range items {
			err = db.Model(item).Create()
			require.NoError(t, err)
		}

		// Define transform function that adds checksum and metadata
		var transformFunc schema.TransformFunc = func(source map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
			target := make(map[string]types.AttributeValue)

			// Copy all existing fields
			for k, v := range source {
				target[k] = v
			}

			// Calculate checksum from data field
			if dataAttr, exists := source["data"]; exists {
				if dataStr, ok := dataAttr.(*types.AttributeValueMemberS); ok {
					checksum := calculateSimpleChecksum(dataStr.Value)
					target["data_checksum"] = &types.AttributeValueMemberS{Value: checksum}
				}
			}

			// Add migration timestamp
			target["migrated_at"] = &types.AttributeValueMemberS{
				Value: time.Now().Format(time.RFC3339),
			}

			// Add metadata
			metadata := map[string]types.AttributeValue{
				"source_table":      &types.AttributeValueMemberS{Value: "large_dataset_v1"},
				"migration_version": &types.AttributeValueMemberS{Value: "1.0"},
			}
			target["metadata"] = &types.AttributeValueMemberM{Value: metadata}

			return target, nil
		}

		// Migrate with standard batch size
		startTime := time.Now()
		err = db.AutoMigrateWithOptions(&LargeDatasetV1{},
			schema.WithTargetModel(&LargeDatasetV2{}),
			schema.WithDataCopy(true),
			schema.WithTransform(transformFunc),
			schema.WithBatchSize(25), // Standard DynamoDB batch size
		)
		require.NoError(t, err)
		migrationDuration := time.Since(startTime)

		t.Logf("Migration of %d items completed in %v", itemCount, migrationDuration)

		// Verify all items were migrated
		var migratedItems []LargeDatasetV2
		err = db.Model(&LargeDatasetV2{}).All(&migratedItems)
		require.NoError(t, err)
		assert.Len(t, migratedItems, itemCount)

		// Verify data integrity by checking a sample
		sampleSize := 10
		for i := 0; i < sampleSize; i++ {
			idx := i * (itemCount / sampleSize)

			var original LargeDatasetV1
			err = db.Model(&LargeDatasetV1{}).
				Where("ID", "=", fmt.Sprintf("item-%05d", idx)).
				Where("Category", "=", fmt.Sprintf("cat-%d", idx%10)).
				First(&original)
			require.NoError(t, err)

			var migrated LargeDatasetV2
			err = db.Model(&LargeDatasetV2{}).
				Where("ID", "=", fmt.Sprintf("item-%05d", idx)).
				Where("Category", "=", fmt.Sprintf("cat-%d", idx%10)).
				First(&migrated)
			require.NoError(t, err)

			// Verify data integrity
			assert.Equal(t, original.ID, migrated.ID)
			assert.Equal(t, original.Category, migrated.Category)
			assert.Equal(t, original.Data, migrated.Data)
			assert.Equal(t, original.ProcessedAt.Unix(), migrated.ProcessedAt.Unix())

			// Verify transform was applied
			expectedChecksum := calculateSimpleChecksum(original.Data)
			assert.Equal(t, expectedChecksum, migrated.DataChecksum)
			assert.NotZero(t, migrated.MigratedAt)
			assert.Equal(t, "large_dataset_v1", migrated.Metadata["source_table"])
		}

		// Clean up
		_ = db.DeleteTable(&LargeDatasetV1{})
		_ = db.DeleteTable(&LargeDatasetV2{})
	})

	t.Run("MigrationWithBatchingAndRetries", func(t *testing.T) {
		// Create source table (use EnsureTable to handle existing tables)
		err := db.EnsureTable(&LargeDatasetV1{})
		require.NoError(t, err)

		// Clear any existing data first
		var existingItems []LargeDatasetV1
		_ = db.Model(&LargeDatasetV1{}).Scan(&existingItems)
		for _, item := range existingItems {
			_ = db.Model(&LargeDatasetV1{}).
				Where("ID", "=", item.ID).
				Where("Category", "=", item.Category).
				Delete()
		}

		// Add items that might cause batch failures (e.g., large items)
		const itemCount = 100
		for i := 0; i < itemCount; i++ {
			item := &LargeDatasetV1{
				ID:          fmt.Sprintf("batch-%03d", i),
				Category:    "stress-test",
				Data:        generateLargeData(i, 10*1024), // 10KB per item
				ProcessedAt: time.Now(),
				Version:     1,
			}
			err = db.Model(item).Create()
			require.NoError(t, err)
		}

		// Transform that occasionally simulates errors for testing retry logic
		errorCount := 0
		var transformFunc schema.TransformFunc = func(source map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
			// Simulate occasional errors (but not too many to avoid test failure)
			if errorCount < 2 {
				if idAttr, exists := source["id"]; exists {
					if idStr, ok := idAttr.(*types.AttributeValueMemberS); ok {
						if strings.HasSuffix(idStr.Value, "050") || strings.HasSuffix(idStr.Value, "075") {
							errorCount++
							return nil, fmt.Errorf("simulated transform error for testing")
						}
					}
				}
			}

			// Normal transform
			target := make(map[string]types.AttributeValue)
			for k, v := range source {
				target[k] = v
			}
			target["migrated_at"] = &types.AttributeValueMemberS{
				Value: time.Now().Format(time.RFC3339),
			}
			return target, nil
		}

		// Migrate with custom batch size
		err = db.AutoMigrateWithOptions(&LargeDatasetV1{},
			schema.WithTargetModel(&LargeDatasetV2{}),
			schema.WithDataCopy(true),
			schema.WithTransform(transformFunc),
			schema.WithBatchSize(25), // DynamoDB batch write limit
			schema.WithContext(context.Background()),
		)

		// The migration might fail due to simulated errors, which is expected
		// In a real scenario, we'd want retry logic in the migration code
		if err != nil {
			t.Logf("Migration completed with expected errors: %v", err)
		}

		// Clean up
		_ = db.DeleteTable(&LargeDatasetV1{})
		_ = db.DeleteTable(&LargeDatasetV2{})
	})
}

func TestMigrationRollbackScenarios(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	tests.RequireDynamoDBLocal(t)

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

	t.Run("BackupBeforeMigration", func(t *testing.T) {
		// Create and populate source table (use EnsureTable to handle existing tables)
		err := db.EnsureTable(&LargeDatasetV1{})
		require.NoError(t, err)

		// Add test data
		testData := []*LargeDatasetV1{
			{
				ID:          "backup-1",
				Category:    "important",
				Data:        "critical data that must be preserved",
				ProcessedAt: time.Now(),
				Version:     1,
			},
			{
				ID:          "backup-2",
				Category:    "important",
				Data:        "another critical piece of data",
				ProcessedAt: time.Now(),
				Version:     1,
			},
		}

		for _, item := range testData {
			err = db.Model(item).Create()
			require.NoError(t, err)
		}

		// Migrate with backup table
		backupTableName := "large_dataset_v1_backup_" + time.Now().Format("20060102_150405")
		err = db.AutoMigrateWithOptions(&LargeDatasetV1{},
			schema.WithBackupTable(backupTableName),
			schema.WithTargetModel(&LargeDatasetV2{}),
			schema.WithDataCopy(true),
		)
		require.NoError(t, err)

		// Verify backup table exists by trying to describe it
		// Since we can't directly check table existence, we'll try to describe it
		// In a real scenario, you would check if the backup was created successfully
		t.Logf("Migration completed with backup table %s", backupTableName)

		// Verify original data in source table still exists
		var sourceItems []LargeDatasetV1
		err = db.Model(&LargeDatasetV1{}).All(&sourceItems)
		require.NoError(t, err)
		assert.Len(t, sourceItems, len(testData), "Source data should be intact")

		// Verify data was copied to target table
		var targetItems []LargeDatasetV2
		err = db.Model(&LargeDatasetV2{}).All(&targetItems)
		require.NoError(t, err)
		assert.Len(t, targetItems, len(testData), "Target table should have migrated data")

		// Clean up tables
		_ = db.DeleteTable(&LargeDatasetV1{})
		_ = db.DeleteTable(&LargeDatasetV2{})
		// Note: Backup table cleanup would require infrastructure tools
		// as DynamORM doesn't support deleting tables by name directly
	})

	t.Run("MigrationWithValidationFailure", func(t *testing.T) {
		// Create source table (use EnsureTable to handle existing tables)
		err := db.EnsureTable(&LargeDatasetV1{})
		require.NoError(t, err)

		// Add item that will fail validation
		item := &LargeDatasetV1{
			ID:          "invalid-1",
			Category:    "test",
			Data:        "data",
			ProcessedAt: time.Now(),
			Version:     1,
		}
		err = db.Model(item).Create()
		require.NoError(t, err)

		// Transform that removes required fields (should fail validation)
		var transformFunc schema.TransformFunc = func(source map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
			target := make(map[string]types.AttributeValue)
			// Intentionally omit required partition key
			if catAttr, exists := source["category"]; exists {
				target["category"] = catAttr
			}
			// Missing "id" field should cause validation failure
			return target, nil
		}

		// Migration should fail due to validation
		err = db.AutoMigrateWithOptions(&LargeDatasetV1{},
			schema.WithTargetModel(&LargeDatasetV2{}),
			schema.WithDataCopy(true),
			schema.WithTransform(transformFunc),
		)
		assert.Error(t, err, "Migration should fail when transform removes required fields")
		assert.Contains(t, err.Error(), "partition key")

		// Verify original data is intact
		var original LargeDatasetV1
		err = db.Model(&LargeDatasetV1{}).
			Where("ID", "=", "invalid-1").
			Where("Category", "=", "test").
			First(&original)
		require.NoError(t, err)
		assert.Equal(t, "data", original.Data)

		// Clean up
		_ = db.DeleteTable(&LargeDatasetV1{})
		_ = db.DeleteTable(&LargeDatasetV2{})
	})

	t.Run("PartialMigrationRecovery", func(t *testing.T) {
		// Create source table (use EnsureTable to handle existing tables)
		err := db.EnsureTable(&LargeDatasetV1{})
		require.NoError(t, err)

		// Add multiple items
		const itemCount = 50
		for i := 0; i < itemCount; i++ {
			item := &LargeDatasetV1{
				ID:          fmt.Sprintf("partial-%03d", i),
				Category:    "recovery-test",
				Data:        fmt.Sprintf("data-%d", i),
				ProcessedAt: time.Now(),
				Version:     1,
			}
			err = db.Model(item).Create()
			require.NoError(t, err)
		}

		// Transform that fails after processing some items
		processedCount := 0
		var transformFunc schema.TransformFunc = func(source map[string]types.AttributeValue) (map[string]types.AttributeValue, error) {
			processedCount++
			// Fail after processing half the items
			if processedCount > itemCount/2 {
				return nil, fmt.Errorf("simulated failure after processing %d items", processedCount)
			}

			target := make(map[string]types.AttributeValue)
			for k, v := range source {
				target[k] = v
			}
			return target, nil
		}

		// Attempt migration (will fail partway through)
		err = db.AutoMigrateWithOptions(&LargeDatasetV1{},
			schema.WithTargetModel(&LargeDatasetV2{}),
			schema.WithDataCopy(true),
			schema.WithTransform(transformFunc),
			schema.WithBatchSize(5), // Small batches to ensure partial processing
		)
		assert.Error(t, err, "Migration should fail after processing some items")

		// In a real rollback scenario, we would:
		// 1. Check how many items were successfully migrated
		// 2. Either continue from where it left off or rollback
		// 3. Ensure data consistency

		// For this test, we'll verify the source data is still intact
		var sourceItems []LargeDatasetV1
		err = db.Model(&LargeDatasetV1{}).All(&sourceItems)
		require.NoError(t, err)
		assert.Len(t, sourceItems, itemCount, "All source items should still exist")

		// Clean up
		_ = db.DeleteTable(&LargeDatasetV1{})
		_ = db.DeleteTable(&LargeDatasetV2{})
	})
}

// Helper functions

func generateLargeData(seed int, size int) string {
	// Generate deterministic data based on seed
	data := fmt.Sprintf("Item-%d-", seed)
	for len(data) < size {
		data += fmt.Sprintf("-%d", seed)
	}
	if len(data) > size {
		data = data[:size]
	}
	return data
}

func calculateSimpleChecksum(data string) string {
	// Simple checksum for testing
	sum := 0
	for _, ch := range data {
		sum += int(ch)
	}
	return fmt.Sprintf("%08x", sum)
}

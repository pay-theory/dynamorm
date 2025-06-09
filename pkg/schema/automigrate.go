package schema

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/pkg/model"
)

// AutoMigrateOptions holds configuration for AutoMigrate operations
type AutoMigrateOptions struct {
	// BackupTable specifies a table name to backup data to before migration
	BackupTable string

	// DataCopy enables copying data from source to target table
	DataCopy bool

	// TargetModel specifies a different model to migrate data to
	TargetModel any

	// Transform is a function to transform data during copy
	Transform interface{}

	// BatchSize for data copy operations
	BatchSize int

	// Context for the operation
	Context context.Context
}

// AutoMigrateOption is a function that configures AutoMigrateOptions
type AutoMigrateOption func(*AutoMigrateOptions)

// WithBackupTable sets a backup table name
func WithBackupTable(tableName string) AutoMigrateOption {
	return func(opts *AutoMigrateOptions) {
		opts.BackupTable = tableName
	}
}

// WithDataCopy enables data copying
func WithDataCopy(enable bool) AutoMigrateOption {
	return func(opts *AutoMigrateOptions) {
		opts.DataCopy = enable
	}
}

// WithTargetModel sets a different target model for migration
func WithTargetModel(model any) AutoMigrateOption {
	return func(opts *AutoMigrateOptions) {
		opts.TargetModel = model
	}
}

// WithTransform sets a transformation function for data migration
func WithTransform(transform interface{}) AutoMigrateOption {
	return func(opts *AutoMigrateOptions) {
		opts.Transform = transform
	}
}

// WithBatchSize sets the batch size for data copy operations
func WithBatchSize(size int) AutoMigrateOption {
	return func(opts *AutoMigrateOptions) {
		opts.BatchSize = size
	}
}

// WithContext sets the context for the operation
func WithContext(ctx context.Context) AutoMigrateOption {
	return func(opts *AutoMigrateOptions) {
		opts.Context = ctx
	}
}

// AutoMigrateWithOptions performs an enhanced auto-migration with data copy support
func (m *Manager) AutoMigrateWithOptions(sourceModel any, options ...AutoMigrateOption) error {
	// Apply options
	opts := &AutoMigrateOptions{
		BatchSize: 25, // Default batch size
		Context:   context.Background(),
	}
	for _, opt := range options {
		opt(opts)
	}

	// Register source model
	if err := m.registry.Register(sourceModel); err != nil {
		return fmt.Errorf("failed to register source model: %w", err)
	}

	sourceMetadata, err := m.registry.GetMetadata(sourceModel)
	if err != nil {
		return fmt.Errorf("failed to get source metadata: %w", err)
	}

	// Determine target model
	targetModel := sourceModel
	if opts.TargetModel != nil {
		targetModel = opts.TargetModel
		if err := m.registry.Register(targetModel); err != nil {
			return fmt.Errorf("failed to register target model: %w", err)
		}
	}

	targetMetadata, err := m.registry.GetMetadata(targetModel)
	if err != nil {
		return fmt.Errorf("failed to get target metadata: %w", err)
	}

	// Handle backup if requested
	if opts.BackupTable != "" {
		if err := m.createBackup(opts.Context, sourceMetadata.TableName, opts.BackupTable); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Create target table if it doesn't exist
	exists, err := m.TableExists(targetMetadata.TableName)
	if err != nil {
		return fmt.Errorf("failed to check target table existence: %w", err)
	}

	if !exists {
		if err := m.CreateTable(targetModel); err != nil {
			return fmt.Errorf("failed to create target table: %w", err)
		}
	}

	// Copy data if requested
	if opts.DataCopy && sourceMetadata.TableName != targetMetadata.TableName {
		if err := m.copyData(opts, sourceMetadata, targetMetadata); err != nil {
			return fmt.Errorf("failed to copy data: %w", err)
		}
	}

	return nil
}

// createBackup creates a point-in-time backup of the table
func (m *Manager) createBackup(ctx context.Context, sourceTable, backupName string) error {
	// Check if source table exists
	exists, err := m.TableExists(sourceTable)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("source table %s does not exist", sourceTable)
	}

	// Create backup using DynamoDB's backup feature
	backupRequest := &dynamodb.CreateBackupInput{
		TableName:  &sourceTable,
		BackupName: &backupName,
	}

	_, err = m.session.Client().CreateBackup(ctx, backupRequest)
	if err != nil {
		// If backup fails, try table copy instead
		return m.copyTable(ctx, sourceTable, backupName)
	}

	return nil
}

// copyTable creates a copy of a table
func (m *Manager) copyTable(ctx context.Context, sourceTable, targetTable string) error {
	// Get source table description
	desc, err := m.session.Client().DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: &sourceTable,
	})
	if err != nil {
		return fmt.Errorf("failed to describe source table: %w", err)
	}

	// Create target table with same schema
	createInput := &dynamodb.CreateTableInput{
		TableName:            &targetTable,
		KeySchema:            desc.Table.KeySchema,
		AttributeDefinitions: desc.Table.AttributeDefinitions,
		BillingMode:          desc.Table.BillingModeSummary.BillingMode,
	}

	// Convert GlobalSecondaryIndexDescriptions to GlobalSecondaryIndexes
	if len(desc.Table.GlobalSecondaryIndexes) > 0 {
		gsis := make([]types.GlobalSecondaryIndex, len(desc.Table.GlobalSecondaryIndexes))
		for i, gsi := range desc.Table.GlobalSecondaryIndexes {
			gsis[i] = types.GlobalSecondaryIndex{
				IndexName:  gsi.IndexName,
				KeySchema:  gsi.KeySchema,
				Projection: gsi.Projection,
			}
			if gsi.ProvisionedThroughput != nil {
				gsis[i].ProvisionedThroughput = &types.ProvisionedThroughput{
					ReadCapacityUnits:  gsi.ProvisionedThroughput.ReadCapacityUnits,
					WriteCapacityUnits: gsi.ProvisionedThroughput.WriteCapacityUnits,
				}
			}
		}
		createInput.GlobalSecondaryIndexes = gsis
	}

	// Convert LocalSecondaryIndexDescriptions to LocalSecondaryIndexes
	if len(desc.Table.LocalSecondaryIndexes) > 0 {
		lsis := make([]types.LocalSecondaryIndex, len(desc.Table.LocalSecondaryIndexes))
		for i, lsi := range desc.Table.LocalSecondaryIndexes {
			lsis[i] = types.LocalSecondaryIndex{
				IndexName:  lsi.IndexName,
				KeySchema:  lsi.KeySchema,
				Projection: lsi.Projection,
			}
		}
		createInput.LocalSecondaryIndexes = lsis
	}

	if desc.Table.ProvisionedThroughput != nil {
		createInput.ProvisionedThroughput = &types.ProvisionedThroughput{
			ReadCapacityUnits:  desc.Table.ProvisionedThroughput.ReadCapacityUnits,
			WriteCapacityUnits: desc.Table.ProvisionedThroughput.WriteCapacityUnits,
		}
	}

	_, err = m.session.Client().CreateTable(ctx, createInput)
	if err != nil {
		return fmt.Errorf("failed to create target table: %w", err)
	}

	// Wait for table to be active
	waiter := dynamodb.NewTableExistsWaiter(m.session.Client())
	if err := waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: &targetTable,
	}, 5*time.Minute); err != nil {
		return fmt.Errorf("timeout waiting for table creation: %w", err)
	}

	// Copy data
	return m.copyTableData(ctx, sourceTable, targetTable, 25)
}

// copyData copies data from source to target table with optional transformation
func (m *Manager) copyData(opts *AutoMigrateOptions, sourceMetadata, targetMetadata *model.Metadata) error {
	ctx := opts.Context

	// Scan source table
	var lastEvaluatedKey map[string]types.AttributeValue
	for {
		scanInput := &dynamodb.ScanInput{
			TableName: &sourceMetadata.TableName,
			Limit:     int32Ptr(int32(opts.BatchSize)),
		}
		if lastEvaluatedKey != nil {
			scanInput.ExclusiveStartKey = lastEvaluatedKey
		}

		result, err := m.session.Client().Scan(ctx, scanInput)
		if err != nil {
			return fmt.Errorf("failed to scan source table: %w", err)
		}

		// Process items
		if len(result.Items) > 0 {
			if err := m.processItems(ctx, result.Items, opts, sourceMetadata, targetMetadata); err != nil {
				return fmt.Errorf("failed to process items: %w", err)
			}
		}

		// Check if more items
		lastEvaluatedKey = result.LastEvaluatedKey
		if lastEvaluatedKey == nil {
			break
		}
	}

	return nil
}

// processItems processes and writes items to the target table
func (m *Manager) processItems(ctx context.Context, items []map[string]types.AttributeValue,
	opts *AutoMigrateOptions, sourceMetadata, targetMetadata *model.Metadata) error {

	// Prepare batch write requests
	writeRequests := make([]types.WriteRequest, 0, len(items))

	for _, item := range items {
		// Apply transformation if provided
		transformedItem := item
		if opts.Transform != nil {
			var err error
			transformedItem, err = m.applyTransform(item, opts.Transform, sourceMetadata, targetMetadata)
			if err != nil {
				return fmt.Errorf("failed to transform item: %w", err)
			}
		}

		writeRequests = append(writeRequests, types.WriteRequest{
			PutRequest: &types.PutRequest{
				Item: transformedItem,
			},
		})
	}

	// Batch write to target table
	if len(writeRequests) > 0 {
		batchInput := &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				targetMetadata.TableName: writeRequests,
			},
		}

		_, err := m.session.Client().BatchWriteItem(ctx, batchInput)
		if err != nil {
			return fmt.Errorf("failed to write items to target table: %w", err)
		}
	}

	return nil
}

// applyTransform applies a transformation function to an item
func (m *Manager) applyTransform(item map[string]types.AttributeValue, transform interface{},
	sourceMetadata, targetMetadata *model.Metadata) (map[string]types.AttributeValue, error) {

	// This is a simplified implementation
	// In a full implementation, we would:
	// 1. Unmarshal the item to the source model type
	// 2. Call the transform function
	// 3. Marshal the result to AttributeValue map

	// For now, return the item unchanged
	// The actual implementation would use the converter to properly handle the transformation
	return item, nil
}

// copyTableData copies all data from source to target table
func (m *Manager) copyTableData(ctx context.Context, sourceTable, targetTable string, batchSize int) error {
	var lastEvaluatedKey map[string]types.AttributeValue

	for {
		// Scan source table
		scanInput := &dynamodb.ScanInput{
			TableName: &sourceTable,
			Limit:     int32Ptr(int32(batchSize)),
		}
		if lastEvaluatedKey != nil {
			scanInput.ExclusiveStartKey = lastEvaluatedKey
		}

		result, err := m.session.Client().Scan(ctx, scanInput)
		if err != nil {
			return fmt.Errorf("failed to scan source table: %w", err)
		}

		// Batch write to target table
		if len(result.Items) > 0 {
			writeRequests := make([]types.WriteRequest, len(result.Items))
			for i, item := range result.Items {
				writeRequests[i] = types.WriteRequest{
					PutRequest: &types.PutRequest{
						Item: item,
					},
				}
			}

			batchInput := &dynamodb.BatchWriteItemInput{
				RequestItems: map[string][]types.WriteRequest{
					targetTable: writeRequests,
				},
			}

			_, err = m.session.Client().BatchWriteItem(ctx, batchInput)
			if err != nil {
				return fmt.Errorf("failed to write batch: %w", err)
			}
		}

		// Check if more items
		lastEvaluatedKey = result.LastEvaluatedKey
		if lastEvaluatedKey == nil {
			break
		}
	}

	return nil
}

// Helper function
func int32Ptr(i int32) *int32 {
	return &i
}

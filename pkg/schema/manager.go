package schema

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/pkg/model"
	"github.com/pay-theory/dynamorm/pkg/session"
)

// Manager handles DynamoDB table schema operations
type Manager struct {
	session  *session.Session
	registry *model.Registry
}

// NewManager creates a new schema manager
func NewManager(session *session.Session, registry *model.Registry) *Manager {
	return &Manager{
		session:  session,
		registry: registry,
	}
}

// TableOption configures table creation options
type TableOption func(*dynamodb.CreateTableInput)

// WithBillingMode sets the billing mode for the table
func WithBillingMode(mode types.BillingMode) TableOption {
	return func(input *dynamodb.CreateTableInput) {
		input.BillingMode = mode
		// If provisioned, remove any existing throughput settings
		if mode == types.BillingModePayPerRequest {
			input.ProvisionedThroughput = nil
		}
	}
}

// WithThroughput sets provisioned throughput for the table
func WithThroughput(rcu, wcu int64) TableOption {
	return func(input *dynamodb.CreateTableInput) {
		input.BillingMode = types.BillingModeProvisioned
		input.ProvisionedThroughput = &types.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(rcu),
			WriteCapacityUnits: aws.Int64(wcu),
		}
	}
}

// WithStreamSpecification enables DynamoDB streams
func WithStreamSpecification(spec types.StreamSpecification) TableOption {
	return func(input *dynamodb.CreateTableInput) {
		input.StreamSpecification = &spec
	}
}

// WithSSESpecification enables server-side encryption
func WithSSESpecification(spec types.SSESpecification) TableOption {
	return func(input *dynamodb.CreateTableInput) {
		input.SSESpecification = &spec
	}
}

// CreateTable creates a DynamoDB table based on the model struct
func (m *Manager) CreateTable(model interface{}, opts ...TableOption) error {
	metadata, err := m.registry.GetMetadata(model)
	if err != nil {
		return fmt.Errorf("failed to get model metadata: %w", err)
	}

	input := &dynamodb.CreateTableInput{
		TableName:   aws.String(metadata.TableName),
		BillingMode: types.BillingModePayPerRequest, // Default to on-demand
	}

	// Build key schema
	input.KeySchema = m.buildKeySchema(metadata)

	// Build attribute definitions
	input.AttributeDefinitions = m.buildAttributeDefinitions(metadata)

	// Build GSI/LSI from unified indexes
	gsiList, lsiList := m.buildIndexes(metadata)
	if len(gsiList) > 0 {
		input.GlobalSecondaryIndexes = gsiList
	}
	if len(lsiList) > 0 {
		input.LocalSecondaryIndexes = lsiList
	}

	// Apply options
	for _, opt := range opts {
		opt(input)
	}

	// Create table
	ctx := context.Background()
	_, err = m.session.Client().CreateTable(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create table %s: %w", metadata.TableName, err)
	}

	// Wait for table to be active
	return m.waitForTableActive(metadata.TableName)
}

// buildKeySchema builds the primary key schema
func (m *Manager) buildKeySchema(metadata *model.Metadata) []types.KeySchemaElement {
	schema := []types.KeySchemaElement{
		{
			AttributeName: aws.String(metadata.PrimaryKey.PartitionKey.DBName),
			KeyType:       types.KeyTypeHash,
		},
	}

	if metadata.PrimaryKey.SortKey != nil {
		schema = append(schema, types.KeySchemaElement{
			AttributeName: aws.String(metadata.PrimaryKey.SortKey.DBName),
			KeyType:       types.KeyTypeRange,
		})
	}

	return schema
}

// buildAttributeDefinitions builds attribute definitions for all keys
func (m *Manager) buildAttributeDefinitions(metadata *model.Metadata) []types.AttributeDefinition {
	// Use a map to avoid duplicates
	attrs := make(map[string]types.ScalarAttributeType)

	// Primary key attributes
	attrs[metadata.PrimaryKey.PartitionKey.DBName] = m.getAttributeType(metadata.PrimaryKey.PartitionKey.Type.Kind())
	if metadata.PrimaryKey.SortKey != nil {
		attrs[metadata.PrimaryKey.SortKey.DBName] = m.getAttributeType(metadata.PrimaryKey.SortKey.Type.Kind())
	}

	// Index attributes
	for _, index := range metadata.Indexes {
		if index.PartitionKey != nil {
			attrs[index.PartitionKey.DBName] = m.getAttributeType(index.PartitionKey.Type.Kind())
		}
		if index.SortKey != nil {
			attrs[index.SortKey.DBName] = m.getAttributeType(index.SortKey.Type.Kind())
		}
	}

	// Convert map to slice
	definitions := make([]types.AttributeDefinition, 0, len(attrs))
	for name, attrType := range attrs {
		definitions = append(definitions, types.AttributeDefinition{
			AttributeName: aws.String(name),
			AttributeType: attrType,
		})
	}

	return definitions
}

// getAttributeType converts Go reflect.Kind to DynamoDB attribute type
func (m *Manager) getAttributeType(kind reflect.Kind) types.ScalarAttributeType {
	switch kind {
	case reflect.String:
		return types.ScalarAttributeTypeS
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return types.ScalarAttributeTypeN
	case reflect.Slice:
		return types.ScalarAttributeTypeB
	default:
		return types.ScalarAttributeTypeS
	}
}

// buildIndexes separates and builds GSI and LSI from metadata
func (m *Manager) buildIndexes(metadata *model.Metadata) ([]types.GlobalSecondaryIndex, []types.LocalSecondaryIndex) {
	var gsiList []types.GlobalSecondaryIndex
	var lsiList []types.LocalSecondaryIndex

	for _, index := range metadata.Indexes {
		if index.Type == model.GlobalSecondaryIndex {
			gsi := types.GlobalSecondaryIndex{
				IndexName: aws.String(index.Name),
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String(index.PartitionKey.DBName),
						KeyType:       types.KeyTypeHash,
					},
				},
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeAll, // Default to ALL
				},
			}

			if index.SortKey != nil {
				gsi.KeySchema = append(gsi.KeySchema, types.KeySchemaElement{
					AttributeName: aws.String(index.SortKey.DBName),
					KeyType:       types.KeyTypeRange,
				})
			}

			// Set projection type based on metadata
			if index.ProjectionType != "" {
				gsi.Projection.ProjectionType = types.ProjectionType(index.ProjectionType)

				// If INCLUDE projection, add non-key attributes
				if index.ProjectionType == "INCLUDE" && len(index.ProjectedFields) > 0 {
					gsi.Projection.NonKeyAttributes = index.ProjectedFields
				}
			}

			gsiList = append(gsiList, gsi)
		} else if index.Type == model.LocalSecondaryIndex {
			lsi := types.LocalSecondaryIndex{
				IndexName: aws.String(index.Name),
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String(metadata.PrimaryKey.PartitionKey.DBName),
						KeyType:       types.KeyTypeHash,
					},
				},
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeAll, // Default to ALL
				},
			}

			if index.SortKey != nil {
				lsi.KeySchema = append(lsi.KeySchema, types.KeySchemaElement{
					AttributeName: aws.String(index.SortKey.DBName),
					KeyType:       types.KeyTypeRange,
				})
			}

			// Set projection type based on metadata
			if index.ProjectionType != "" {
				lsi.Projection.ProjectionType = types.ProjectionType(index.ProjectionType)

				// If INCLUDE projection, add non-key attributes
				if index.ProjectionType == "INCLUDE" && len(index.ProjectedFields) > 0 {
					lsi.Projection.NonKeyAttributes = index.ProjectedFields
				}
			}

			lsiList = append(lsiList, lsi)
		}
	}

	return gsiList, lsiList
}

// waitForTableActive waits for a table to become active
func (m *Manager) waitForTableActive(tableName string) error {
	ctx := context.Background()
	waiter := dynamodb.NewTableExistsWaiter(m.session.Client())

	// Wait up to 5 minutes for table to be active
	err := waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}, 5*time.Minute)

	if err != nil {
		return fmt.Errorf("failed waiting for table %s to be active: %w", tableName, err)
	}

	return nil
}

// TableExists checks if a table exists
func (m *Manager) TableExists(tableName string) (bool, error) {
	ctx := context.Background()
	_, err := m.session.Client().DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	})

	if err != nil {
		var notFoundErr *types.ResourceNotFoundException
		if ok := errors.As(err, &notFoundErr); ok {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// DeleteTable deletes a DynamoDB table
func (m *Manager) DeleteTable(tableName string) error {
	ctx := context.Background()
	_, err := m.session.Client().DeleteTable(ctx, &dynamodb.DeleteTableInput{
		TableName: aws.String(tableName),
	})

	if err != nil {
		return fmt.Errorf("failed to delete table %s: %w", tableName, err)
	}

	// Wait for table to be deleted
	waiter := dynamodb.NewTableNotExistsWaiter(m.session.Client())
	return waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}, 5*time.Minute)
}

// DescribeTable returns table description
func (m *Manager) DescribeTable(model interface{}) (*types.TableDescription, error) {
	metadata, err := m.registry.GetMetadata(model)
	if err != nil {
		return nil, fmt.Errorf("failed to get model metadata: %w", err)
	}

	ctx := context.Background()
	output, err := m.session.Client().DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(metadata.TableName),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to describe table %s: %w", metadata.TableName, err)
	}

	return output.Table, nil
}

// UpdateTable updates table configuration (throughput, indexes, etc.)
func (m *Manager) UpdateTable(model interface{}, opts ...TableOption) error {
	metadata, err := m.registry.GetMetadata(model)
	if err != nil {
		return fmt.Errorf("failed to get model metadata: %w", err)
	}

	// Get current table description
	current, err := m.DescribeTable(model)
	if err != nil {
		return err
	}

	// Build update input
	input := &dynamodb.UpdateTableInput{
		TableName: aws.String(metadata.TableName),
	}

	// Apply options to determine what to update
	createInput := &dynamodb.CreateTableInput{}
	for _, opt := range opts {
		opt(createInput)
	}

	// Update billing mode if changed
	if createInput.BillingMode != "" && createInput.BillingMode != current.BillingModeSummary.BillingMode {
		input.BillingMode = createInput.BillingMode

		// If switching to provisioned, set throughput
		if createInput.BillingMode == types.BillingModeProvisioned && createInput.ProvisionedThroughput != nil {
			input.ProvisionedThroughput = createInput.ProvisionedThroughput
		}
	}

	// Update streams if changed
	if createInput.StreamSpecification != nil {
		input.StreamSpecification = createInput.StreamSpecification
	}

	// Update SSE if changed
	if createInput.SSESpecification != nil {
		input.SSESpecification = createInput.SSESpecification
	}

	// TODO: Handle GSI updates (create/delete indexes)

	ctx := context.Background()
	_, err = m.session.Client().UpdateTable(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to update table %s: %w", metadata.TableName, err)
	}

	// Wait for update to complete
	return m.waitForTableActive(metadata.TableName)
}

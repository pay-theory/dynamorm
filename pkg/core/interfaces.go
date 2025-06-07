// Package core defines the core interfaces and types for DynamORM
package core

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// DB represents the main database connection interface
type DB interface {
	// Model returns a new query builder for the given model
	Model(model interface{}) Query

	// Transaction executes a function within a database transaction
	Transaction(fn func(tx *Tx) error) error

	// Migrate runs all pending migrations
	Migrate() error

	// AutoMigrate creates or updates tables based on the given models
	AutoMigrate(models ...interface{}) error

	// Close closes the database connection
	Close() error

	// WithContext returns a new DB instance with the given context
	WithContext(ctx context.Context) DB
}

// Query represents a chainable query builder interface
type Query interface {
	// Where adds a condition to the query
	Where(field string, op string, value interface{}) Query

	// Index specifies which index to use for the query
	Index(indexName string) Query

	// Filter adds a filter expression to the query
	Filter(expression string, values ...interface{}) Query

	// OrderBy sets the sort order for the query
	OrderBy(field string, order string) Query

	// Limit sets the maximum number of items to return
	Limit(limit int) Query

	// Offset sets the starting position for the query
	Offset(offset int) Query

	// Select specifies which fields to retrieve
	Select(fields ...string) Query

	// First retrieves the first matching item
	First(dest interface{}) error

	// All retrieves all matching items
	All(dest interface{}) error

	// Count returns the number of matching items
	Count() (int64, error)

	// Create creates a new item
	Create() error

	// Update updates the matching items
	Update(fields ...string) error

	// Delete deletes the matching items
	Delete() error

	// Scan performs a table scan
	Scan(dest interface{}) error

	// BatchGet retrieves multiple items by their primary keys
	BatchGet(keys []interface{}, dest interface{}) error

	// BatchCreate creates multiple items
	BatchCreate(items interface{}) error

	// WithContext sets the context for the query
	WithContext(ctx context.Context) Query
}

// Tx represents a database transaction
type Tx struct {
	db DB
}

// Model returns a new query builder for the given model within the transaction
func (tx *Tx) Model(model interface{}) Query {
	return tx.db.Model(model)
}

// Create creates a new item within the transaction
func (tx *Tx) Create(model interface{}) error {
	return tx.db.Model(model).Create()
}

// Update updates an item within the transaction
func (tx *Tx) Update(model interface{}, fields ...string) error {
	return tx.db.Model(model).Update(fields...)
}

// Delete deletes an item within the transaction
func (tx *Tx) Delete(model interface{}) error {
	return tx.db.Model(model).Delete()
}

// Param represents a parameter for expressions
type Param struct {
	Name  string
	Value interface{}
}

// CompiledQuery represents a compiled query ready for execution
type CompiledQuery struct {
	Operation string // "Query", "Scan", "GetItem", etc.
	TableName string
	IndexName string

	// Expression components
	KeyConditionExpression string
	FilterExpression       string
	ProjectionExpression   string
	UpdateExpression       string
	ConditionExpression    string

	// Expression mappings
	ExpressionAttributeNames  map[string]string
	ExpressionAttributeValues map[string]types.AttributeValue

	// Other query parameters
	Limit             *int32
	ExclusiveStartKey map[string]types.AttributeValue
	ScanIndexForward  *bool
	Select            string // "ALL_ATTRIBUTES", "COUNT", etc.
	Offset            *int   // For pagination handling
}

// ModelMetadata provides metadata about a model
type ModelMetadata interface {
	TableName() string
	PrimaryKey() KeySchema
	Indexes() []IndexSchema
	AttributeMetadata(field string) *AttributeMetadata
}

// KeySchema represents a primary key or index key schema
type KeySchema struct {
	PartitionKey string
	SortKey      string // optional
}

// IndexSchema represents a GSI or LSI schema
type IndexSchema struct {
	Name            string
	Type            string // "GSI" or "LSI"
	PartitionKey    string
	SortKey         string
	ProjectionType  string
	ProjectedFields []string
}

// AttributeMetadata provides metadata about a model attribute
type AttributeMetadata struct {
	Name         string
	Type         string
	DynamoDBName string
	Tags         map[string]string
}

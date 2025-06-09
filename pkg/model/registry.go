// Package model provides model registration and metadata management for DynamORM
package model

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/pay-theory/dynamorm/pkg/errors"
)

// Registry manages registered models and their metadata
type Registry struct {
	mu     sync.RWMutex
	models map[reflect.Type]*Metadata
	tables map[string]*Metadata
}

// NewRegistry creates a new model registry
func NewRegistry() *Registry {
	return &Registry{
		models: make(map[reflect.Type]*Metadata),
		tables: make(map[string]*Metadata),
	}
}

// Register registers a model and parses its metadata
func (r *Registry) Register(model any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	modelType := reflect.TypeOf(model)
	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}

	if modelType.Kind() != reflect.Struct {
		return fmt.Errorf("%w: model must be a struct", errors.ErrInvalidModel)
	}

	// Check if already registered
	if _, exists := r.models[modelType]; exists {
		return nil // Already registered
	}

	// Parse metadata
	metadata, err := parseMetadata(modelType)
	if err != nil {
		return err
	}

	// Register model
	r.models[modelType] = metadata
	r.tables[metadata.TableName] = metadata

	return nil
}

// GetMetadata retrieves metadata for a model
func (r *Registry) GetMetadata(model any) (*Metadata, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	modelType := reflect.TypeOf(model)
	if modelType.Kind() == reflect.Ptr {
		modelType = modelType.Elem()
	}

	metadata, exists := r.models[modelType]
	if !exists {
		return nil, fmt.Errorf("%w: model not registered: %s", errors.ErrInvalidModel, modelType.Name())
	}

	return metadata, nil
}

// GetMetadataByTable retrieves metadata by table name
func (r *Registry) GetMetadataByTable(tableName string) (*Metadata, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metadata, exists := r.tables[tableName]
	if !exists {
		return nil, fmt.Errorf("%w: table not found: %s", errors.ErrTableNotFound, tableName)
	}

	return metadata, nil
}

// Metadata holds all metadata for a model
type Metadata struct {
	Type           reflect.Type
	TableName      string
	PrimaryKey     *KeySchema
	Indexes        []IndexSchema
	Fields         map[string]*FieldMetadata
	FieldsByDBName map[string]*FieldMetadata
	VersionField   *FieldMetadata
	TTLField       *FieldMetadata
	CreatedAtField *FieldMetadata
	UpdatedAtField *FieldMetadata
}

// KeySchema represents a primary key or index key schema
type KeySchema struct {
	PartitionKey *FieldMetadata
	SortKey      *FieldMetadata
}

// IndexSchema represents a GSI or LSI schema
type IndexSchema struct {
	Name            string
	Type            IndexType
	PartitionKey    *FieldMetadata
	SortKey         *FieldMetadata
	ProjectionType  string
	ProjectedFields []string
	Sparse          bool
}

// IndexType represents the type of index
type IndexType string

const (
	GlobalSecondaryIndex IndexType = "GSI"
	LocalSecondaryIndex  IndexType = "LSI"
)

// FieldMetadata holds metadata for a single field
type FieldMetadata struct {
	Name        string               // Go field name
	Type        reflect.Type         // Go type
	DBName      string               // DynamoDB attribute name
	Index       int                  // Field index in struct
	Tags        map[string]string    // Parsed tags
	IsPK        bool                 // Is partition key
	IsSK        bool                 // Is sort key
	IsVersion   bool                 // Is version field
	IsTTL       bool                 // Is TTL field
	IsCreatedAt bool                 // Is created_at field
	IsUpdatedAt bool                 // Is updated_at field
	IsSet       bool                 // Should be stored as DynamoDB set
	OmitEmpty   bool                 // Omit if empty
	IndexInfo   map[string]IndexRole // Index participation
}

// IndexRole represents a field's role in an index
type IndexRole struct {
	IndexName string
	IsPK      bool
	IsSK      bool
}

// parseMetadata parses model metadata from struct tags
func parseMetadata(modelType reflect.Type) (*Metadata, error) {
	metadata := &Metadata{
		Type:           modelType,
		TableName:      getTableName(modelType),
		Fields:         make(map[string]*FieldMetadata),
		FieldsByDBName: make(map[string]*FieldMetadata),
		Indexes:        make([]IndexSchema, 0),
	}

	indexMap := make(map[string]*IndexSchema)

	// Parse fields
	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		fieldMeta, err := parseFieldMetadata(field, i)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", field.Name, err)
		}

		// Register field
		metadata.Fields[field.Name] = fieldMeta
		metadata.FieldsByDBName[fieldMeta.DBName] = fieldMeta

		// Handle primary key
		if fieldMeta.IsPK {
			if metadata.PrimaryKey == nil {
				metadata.PrimaryKey = &KeySchema{}
			}
			if metadata.PrimaryKey.PartitionKey != nil {
				return nil, fmt.Errorf("field %s: %w", field.Name, errors.ErrDuplicatePrimaryKey)
			}
			metadata.PrimaryKey.PartitionKey = fieldMeta
		}

		if fieldMeta.IsSK {
			if metadata.PrimaryKey == nil {
				metadata.PrimaryKey = &KeySchema{}
			}
			if metadata.PrimaryKey.SortKey != nil {
				return nil, fmt.Errorf("field %s: duplicate sort key definition", field.Name)
			}
			metadata.PrimaryKey.SortKey = fieldMeta
		}

		// Handle special fields
		if fieldMeta.IsVersion {
			metadata.VersionField = fieldMeta
		}
		if fieldMeta.IsTTL {
			metadata.TTLField = fieldMeta
		}
		if fieldMeta.IsCreatedAt {
			metadata.CreatedAtField = fieldMeta
		}
		if fieldMeta.IsUpdatedAt {
			metadata.UpdatedAtField = fieldMeta
		}

		// Process indexes
		for indexName, role := range fieldMeta.IndexInfo {
			index, exists := indexMap[indexName]
			if !exists {
				index = &IndexSchema{
					Name: indexName,
					Type: determineIndexType(indexName),
				}
				indexMap[indexName] = index
			}

			if role.IsPK {
				if index.PartitionKey != nil {
					return nil, fmt.Errorf("field %s: duplicate partition key for index %s", field.Name, indexName)
				}
				index.PartitionKey = fieldMeta
			}
			if role.IsSK {
				if index.SortKey != nil {
					return nil, fmt.Errorf("field %s: duplicate sort key for index %s", field.Name, indexName)
				}
				index.SortKey = fieldMeta
			}
		}
	}

	// Validate primary key
	if metadata.PrimaryKey == nil || metadata.PrimaryKey.PartitionKey == nil {
		return nil, errors.ErrMissingPrimaryKey
	}

	// Convert index map to slice
	for _, index := range indexMap {
		// LSIs share the partition key with the main table
		if index.Type == LocalSecondaryIndex {
			index.PartitionKey = metadata.PrimaryKey.PartitionKey
		} else if index.PartitionKey == nil {
			// GSIs must have their own partition key
			return nil, fmt.Errorf("index %s: missing partition key", index.Name)
		}
		metadata.Indexes = append(metadata.Indexes, *index)
	}

	return metadata, nil
}

// parseFieldMetadata parses metadata for a single field
func parseFieldMetadata(field reflect.StructField, index int) (*FieldMetadata, error) {
	meta := &FieldMetadata{
		Name:      field.Name,
		Type:      field.Type,
		DBName:    field.Name,
		Index:     index,
		Tags:      make(map[string]string),
		IndexInfo: make(map[string]IndexRole),
	}

	// Parse dynamorm tag
	tag := field.Tag.Get("dynamorm")
	if tag == "" {
		return meta, nil
	}

	if tag == "-" {
		return nil, nil // Skip this field
	}

	// Parse tag components - need special handling for index tags
	parts := splitTags(tag)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Handle key:value tags
		if colonIdx := strings.Index(part, ":"); colonIdx > 0 {
			key := part[:colonIdx]
			value := part[colonIdx+1:]

			switch key {
			case "attr":
				meta.DBName = value
			case "index":
				if err := parseIndexTag(meta, value); err != nil {
					return nil, err
				}
			case "lsi":
				meta.IndexInfo[value] = IndexRole{
					IndexName: value,
					IsSK:      true,
				}
			case "project":
				meta.Tags["project"] = value
			default:
				meta.Tags[key] = value
			}
		} else {
			// Handle simple tags
			switch part {
			case "pk":
				meta.IsPK = true
			case "sk":
				meta.IsSK = true
			case "version":
				meta.IsVersion = true
			case "ttl":
				meta.IsTTL = true
			case "created_at":
				meta.IsCreatedAt = true
			case "updated_at":
				meta.IsUpdatedAt = true
			case "set":
				meta.IsSet = true
			case "omitempty":
				meta.OmitEmpty = true
			case "binary", "json", "encrypted":
				meta.Tags[part] = "true"
			default:
				return nil, fmt.Errorf("%w: unknown tag '%s'", errors.ErrInvalidTag, part)
			}
		}
	}

	// Validate field type for special tags
	if err := validateFieldType(meta); err != nil {
		return nil, err
	}

	return meta, nil
}

// parseIndexTag parses an index tag value
func parseIndexTag(meta *FieldMetadata, value string) error {
	parts := strings.Split(value, ",")
	indexName := strings.TrimSpace(parts[0])

	role := IndexRole{IndexName: indexName}

	// Default behavior: field is partition key if no role specified
	if len(parts) == 1 {
		role.IsPK = true
	} else {
		for i := 1; i < len(parts); i++ {
			part := strings.TrimSpace(parts[i])
			if part == "" {
				continue // Skip empty parts
			}
			switch part {
			case "pk":
				role.IsPK = true
			case "sk":
				role.IsSK = true
			case "sparse":
				meta.Tags["sparse:"+indexName] = "true"
			default:
				return fmt.Errorf("%w: unknown index tag modifier '%s'", errors.ErrInvalidTag, part)
			}
		}
	}

	meta.IndexInfo[indexName] = role
	return nil
}

// validateFieldType validates field type against tag requirements
func validateFieldType(meta *FieldMetadata) error {
	// Validate version field
	if meta.IsVersion {
		switch meta.Type.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			// Valid numeric types
		default:
			return fmt.Errorf("%w: version field must be numeric", errors.ErrInvalidTag)
		}
	}

	// Validate TTL field
	if meta.IsTTL {
		switch meta.Type.Kind() {
		case reflect.Int64, reflect.Uint64:
			// Valid TTL types
		default:
			return fmt.Errorf("%w: ttl field must be int64 or uint64", errors.ErrInvalidTag)
		}
	}

	// Validate set tag
	if meta.IsSet && meta.Type.Kind() != reflect.Slice {
		return fmt.Errorf("%w: set tag can only be used on slice types", errors.ErrInvalidTag)
	}

	// Validate created_at and updated_at
	if meta.IsCreatedAt || meta.IsUpdatedAt {
		if meta.Type.String() != "time.Time" {
			return fmt.Errorf("%w: created_at/updated_at fields must be time.Time", errors.ErrInvalidTag)
		}
	}

	return nil
}

// getTableName derives the table name from the model type
func getTableName(modelType reflect.Type) string {
	name := modelType.Name()
	// Convert to plural form (simple version)
	if strings.HasSuffix(name, "s") {
		return name + "es"
	}
	if strings.HasSuffix(name, "y") {
		return name[:len(name)-1] + "ies"
	}
	return name + "s"
}

// determineIndexType determines if an index is GSI or LSI based on naming convention
func determineIndexType(indexName string) IndexType {
	if strings.HasPrefix(indexName, "lsi-") || strings.HasPrefix(indexName, "lsi:") {
		return LocalSecondaryIndex
	}
	return GlobalSecondaryIndex
}

// splitTags splits struct tags while preserving index tag values that contain commas
func splitTags(tag string) []string {
	var parts []string
	var current strings.Builder
	inSpecialTag := false

	for i := 0; i < len(tag); i++ {
		ch := tag[i]

		if ch == ',' && !inSpecialTag {
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		} else {
			current.WriteByte(ch)

			// Check if we're in a special tag that might contain commas
			currentStr := current.String()
			if strings.HasPrefix(currentStr, "index:") || strings.HasPrefix(currentStr, "lsi:") {
				// We're in an index tag - need to check if we've reached the end
				if ch == ',' && i+1 < len(tag) {
					// Look ahead to see what follows the comma
					remaining := tag[i+1:]
					remaining = strings.TrimSpace(remaining)

					// Check if the next part is a standalone tag (not an index modifier)
					if isStandaloneTag(remaining) {
						// This comma ends the index tag
						parts = append(parts, current.String())
						current.Reset()
						inSpecialTag = false
					} else {
						// This comma is part of the index tag value
						inSpecialTag = true
					}
				} else {
					inSpecialTag = true
				}
			}
		}
	}

	// Add the last part
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// isStandaloneTag checks if the string starts with a standalone tag (not an index modifier)
func isStandaloneTag(s string) bool {
	// Check for simple tags
	simpleTags := []string{
		"pk", "sk", "version", "ttl", "created_at", "updated_at",
		"set", "omitempty", "binary", "json", "encrypted",
	}

	for _, tag := range simpleTags {
		if s == tag || strings.HasPrefix(s, tag+",") {
			// But pk/sk after index: are modifiers, not standalone tags
			if (tag == "pk" || tag == "sk") && !strings.Contains(s, ":") {
				// This could be a modifier for the previous index tag
				return false
			}
			return true
		}
	}

	// Check for key:value tags
	if strings.Contains(s, ":") {
		colonIdx := strings.Index(s, ":")
		key := s[:colonIdx]
		knownKeys := []string{"attr", "index", "lsi", "project"}
		for _, k := range knownKeys {
			if key == k {
				return true
			}
		}
	}

	return false
}

// getNextTagPart extracts the next tag part after current position
func getNextTagPart(s string) string {
	s = strings.TrimSpace(s)
	commaIdx := strings.Index(s, ",")
	colonIdx := strings.Index(s, ":")

	if commaIdx == -1 {
		return s
	}
	if colonIdx == -1 || colonIdx > commaIdx {
		return s[:commaIdx]
	}
	return s[:colonIdx]
}

// isKnownSimpleTag checks if a string is a known simple tag
func isKnownSimpleTag(s string) bool {
	knownTags := []string{
		"pk", "sk", "version", "ttl", "created_at", "updated_at",
		"set", "omitempty", "binary", "json", "encrypted",
	}
	s = strings.TrimSpace(s)
	for _, tag := range knownTags {
		if s == tag {
			return true
		}
	}
	// Also check for known key:value tags
	if strings.HasPrefix(s, "attr:") || strings.HasPrefix(s, "project:") {
		return true
	}
	return false
}

// Package model provides model registration and metadata management for DynamORM
package model

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/pay-theory/dynamorm/pkg/naming"
)

// Registry manages registered models and their metadata
type Registry struct {
	models map[reflect.Type]*Metadata
	tables map[string]*Metadata
	mu     sync.RWMutex
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
	Type             reflect.Type
	PrimaryKey       *KeySchema
	Fields           map[string]*FieldMetadata
	FieldsByDBName   map[string]*FieldMetadata
	VersionField     *FieldMetadata
	TTLField         *FieldMetadata
	CreatedAtField   *FieldMetadata
	UpdatedAtField   *FieldMetadata
	TableName        string
	Indexes          []IndexSchema
	NamingConvention naming.Convention
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
	Type        reflect.Type
	IndexInfo   map[string]IndexRole
	Tags        map[string]string
	DBName      string
	Name        string
	IndexPath   []int
	Index       int
	IsPK        bool
	IsVersion   bool
	IsTTL       bool
	IsCreatedAt bool
	IsUpdatedAt bool
	IsSet       bool
	OmitEmpty   bool
	IsSK        bool
}

// IndexRole represents a field's role in an index
type IndexRole struct {
	IndexName string
	IsPK      bool
	IsSK      bool
}

// parseMetadata parses model metadata from struct tags
func parseMetadata(modelType reflect.Type) (*Metadata, error) {
	// First check if the model has a TableName method
	tableName := ""

	// Check for TableName method on value receiver
	modelValue := reflect.New(modelType).Elem()
	if method := modelValue.MethodByName("TableName"); method.IsValid() {
		if method.Type().NumIn() == 0 && method.Type().NumOut() == 1 {
			results := method.Call(nil)
			if len(results) > 0 && results[0].Kind() == reflect.String {
				tableName = results[0].String()
			}
		}
	}

	// If not found on value, check pointer receiver
	if tableName == "" {
		modelPtr := reflect.New(modelType)
		if method := modelPtr.MethodByName("TableName"); method.IsValid() {
			if method.Type().NumIn() == 0 && method.Type().NumOut() == 1 {
				results := method.Call(nil)
				if len(results) > 0 && results[0].Kind() == reflect.String {
					tableName = results[0].String()
				}
			}
		}
	}

	// If no TableName method or it returned empty, use default
	if tableName == "" {
		tableName = getTableName(modelType)
	}

	// Detect naming convention from struct tags
	convention := detectNamingConvention(modelType)

	metadata := &Metadata{
		Type:             modelType,
		TableName:        tableName,
		NamingConvention: convention,
		Fields:           make(map[string]*FieldMetadata),
		FieldsByDBName:   make(map[string]*FieldMetadata),
		Indexes:          make([]IndexSchema, 0),
	}

	indexMap := make(map[string]*IndexSchema)

	// Parse fields recursively to handle embedded structs
	if err := parseFields(modelType, metadata, indexMap, []int{}); err != nil {
		return nil, err
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
			return nil, fmt.Errorf("missing partition key for index")
		}
		metadata.Indexes = append(metadata.Indexes, *index)
	}

	return metadata, nil
}

// parseFields recursively parses fields including embedded structs
func parseFields(modelType reflect.Type, metadata *Metadata, indexMap map[string]*IndexSchema, indexPath []int) error {
	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		currentPath := append(indexPath, i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Handle embedded structs
		if field.Anonymous && field.Type.Kind() == reflect.Struct {
			// Recursively parse embedded struct fields
			if err := parseFields(field.Type, metadata, indexMap, currentPath); err != nil {
				return err
			}
			continue
		}

		// Parse regular field
		fieldMeta, err := parseFieldMetadata(field, currentPath, metadata.NamingConvention)
		if err != nil {
			return fmt.Errorf("field validation failed: %w", err)
		}

		// Skip nil fields (e.g., fields with tag "-")
		if fieldMeta == nil {
			continue
		}

		// Register field with full path name for embedded fields
		fullFieldName := field.Name
		if len(indexPath) > 0 {
			// For embedded fields, we need a unique name
			// We'll use the field name directly since Go ensures uniqueness at each level
			fullFieldName = field.Name
		}
		metadata.Fields[fullFieldName] = fieldMeta
		metadata.FieldsByDBName[fieldMeta.DBName] = fieldMeta

		// Handle primary key
		if fieldMeta.IsPK {
			if metadata.PrimaryKey == nil {
				metadata.PrimaryKey = &KeySchema{}
			}
			if metadata.PrimaryKey.PartitionKey != nil {
				return fmt.Errorf("duplicate primary key definition: %w", errors.ErrDuplicatePrimaryKey)
			}
			metadata.PrimaryKey.PartitionKey = fieldMeta
		}

		if fieldMeta.IsSK {
			if metadata.PrimaryKey == nil {
				metadata.PrimaryKey = &KeySchema{}
			}
			if metadata.PrimaryKey.SortKey != nil {
				return fmt.Errorf("duplicate sort key definition")
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
				// Check if this is an LSI based on field tags
				var indexType IndexType
				if _, isLSI := fieldMeta.Tags["lsi:"+indexName]; isLSI {
					indexType = LocalSecondaryIndex
				} else {
					// Fall back to name-based detection for backward compatibility
					indexType = determineIndexType(indexName)
				}

				index = &IndexSchema{
					Name: indexName,
					Type: indexType,
				}
				indexMap[indexName] = index
			}

			if role.IsPK {
				if index.PartitionKey != nil {
					return fmt.Errorf("duplicate partition key for index %s", indexName)
				}
				index.PartitionKey = fieldMeta
			}
			if role.IsSK {
				if index.SortKey != nil {
					return fmt.Errorf("duplicate sort key for index %s", indexName)
				}
				index.SortKey = fieldMeta
			}
		}
	}

	return nil
}

// parseFieldMetadata parses metadata for a single field
func parseFieldMetadata(field reflect.StructField, indexPath []int, convention naming.Convention) (*FieldMetadata, error) {
	meta := &FieldMetadata{
		Name:      field.Name,
		Type:      field.Type,
		DBName:    naming.ConvertAttrName(field.Name, convention),
		Index:     indexPath[len(indexPath)-1], // Keep for backward compatibility
		IndexPath: indexPath,
		Tags:      make(map[string]string),
		IndexInfo: make(map[string]IndexRole),
	}

	// Implicit timestamp detection for conventional field names
	isTimeField := field.Type.Kind() == reflect.Struct &&
		field.Type.PkgPath() == "time" &&
		field.Type.Name() == "Time"
	if isTimeField && strings.EqualFold(field.Name, "CreatedAt") {
		meta.IsCreatedAt = true
	}
	if isTimeField && strings.EqualFold(field.Name, "UpdatedAt") {
		meta.IsUpdatedAt = true
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
			value := strings.TrimSpace(part[colonIdx+1:])

			switch key {
			case "attr":
				meta.DBName = value
			case "index":
				if err := parseIndexTag(meta, value); err != nil {
					return nil, err
				}
			case "lsi":
				// Parse LSI tag similar to index tag to support modifiers
				lsiParts := strings.Split(value, ",")
				indexName := strings.TrimSpace(lsiParts[0])

				role := IndexRole{IndexName: indexName}

				// LSI fields are sort keys by default
				if len(lsiParts) == 1 {
					role.IsSK = true
				} else {
					for i := 1; i < len(lsiParts); i++ {
						modifier := strings.TrimSpace(lsiParts[i])
						switch modifier {
						case "sk":
							role.IsSK = true
						default:
							return nil, fmt.Errorf("%w: unknown lsi tag modifier '%s'", errors.ErrInvalidTag, modifier)
						}
					}
				}

				meta.IndexInfo[indexName] = role
				// Mark this index as LSI explicitly
				meta.Tags["lsi:"+indexName] = "true"
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
				// Don't change the DBName, keep the field name as is
			case "sk":
				meta.IsSK = true
				// Don't change the DBName, keep the field name as is
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

	if err := naming.ValidateAttrName(meta.DBName, convention); err != nil {
		return nil, fmt.Errorf("%w: %v", errors.ErrInvalidTag, err)
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
	if strings.HasPrefix(indexName, "lsi-") || strings.HasPrefix(indexName, "lsi_") {
		return LocalSecondaryIndex
	}
	return GlobalSecondaryIndex
}

// splitTags splits struct tags while keeping index/LSI modifiers attached to the index tag
func splitTags(tag string) []string {
	var parts []string
	tokens := strings.Split(tag, ",")

	var current strings.Builder
	inIndexClause := false

	flushCurrent := func() {
		if current.Len() == 0 {
			return
		}
		parts = append(parts, current.String())
		current.Reset()
		inIndexClause = false
	}

	for _, raw := range tokens {
		part := strings.TrimSpace(raw)
		if part == "" {
			continue
		}

		if inIndexClause {
			if isIndexModifier(part) {
				current.WriteString(",")
				current.WriteString(part)
				continue
			}
			flushCurrent()
		}

		if strings.HasPrefix(part, "index:") || strings.HasPrefix(part, "lsi:") {
			inIndexClause = true
			current.WriteString(part)
			continue
		}

		parts = append(parts, part)
	}

	flushCurrent()

	return parts
}

// detectNamingConvention scans struct fields for a naming convention tag.
// It looks for a field (typically blank identifier _) with tag `dynamorm:"naming:snake_case"`.
// Returns CamelCase (default) if no naming tag is found.
func detectNamingConvention(modelType reflect.Type) naming.Convention {
	for i := 0; i < modelType.NumField(); i++ {
		field := modelType.Field(i)
		tag := field.Tag.Get("dynamorm")

		if tag == "" {
			continue
		}

		// Look for naming:snake_case or naming:camel_case
		parts := strings.Split(tag, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "naming:") {
				convention := strings.TrimPrefix(part, "naming:")
				switch convention {
				case "snake_case":
					return naming.SnakeCase
				case "camel_case", "camelCase":
					return naming.CamelCase
				}
			}
		}
	}

	// Default to CamelCase
	return naming.CamelCase
}

// isIndexModifier returns true if the token belongs to the current index/LSI clause
func isIndexModifier(token string) bool {
	switch token {
	case "pk", "sk", "sparse":
		return true
	default:
		return false
	}
}

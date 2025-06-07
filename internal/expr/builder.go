package expr

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	dynamormErrors "github.com/pay-theory/dynamorm/pkg/errors"
)

// Reserved words in DynamoDB that need to be escaped
var reservedWords = map[string]bool{
	"ABORT": true, "ABSOLUTE": true, "ACTION": true, "ADD": true, "AFTER": true,
	"AGENT": true, "AGGREGATE": true, "ALL": true, "ALLOCATE": true, "ALTER": true,
	"ANALYZE": true, "AND": true, "ANY": true, "ARCHIVE": true, "ARE": true,
	"ARRAY": true, "AS": true, "ASC": true, "ASCII": true, "ASENSITIVE": true,
	"ASSERTION": true, "ASYMMETRIC": true, "AT": true, "ATOMIC": true, "ATTACH": true,
	"ATTRIBUTE": true, "AUTH": true, "AUTHORIZATION": true, "AUTHORIZE": true, "AUTO": true,
	"AVG": true, "BACK": true, "BACKUP": true, "BASE": true, "BATCH": true,
	"BEFORE": true, "BEGIN": true, "BETWEEN": true, "BIGINT": true, "BINARY": true,
	"BIT": true, "BLOB": true, "BLOCK": true, "BOOLEAN": true, "BOTH": true,
	"BREADTH": true, "BUCKET": true, "BULK": true, "BY": true, "BYTE": true,
	"CALL": true, "CALLED": true, "CALLING": true, "CAPACITY": true, "CASCADE": true,
	"CASCADED": true, "CASE": true, "CAST": true, "CATALOG": true, "CHAR": true,
	"CHARACTER": true, "CHECK": true, "CLASS": true, "CLOB": true, "CLOSE": true,
	"CLUSTER": true, "CLUSTERED": true, "CLUSTERING": true, "CLUSTERS": true, "COALESCE": true,
	"COLLATE": true, "COLLATION": true, "COLLECTION": true, "COLUMN": true, "COLUMNS": true,
	"COMBINE": true, "COMMENT": true, "COMMIT": true, "COMPACT": true, "COMPILE": true,
	"COMPRESS": true, "CONDITION": true, "CONFLICT": true, "CONNECT": true, "CONNECTION": true,
	"CONSISTENCY": true, "CONSISTENT": true, "CONSTRAINT": true, "CONSTRAINTS": true, "CONSTRUCTOR": true,
	"CONSUMED": true, "CONTINUE": true, "CONVERT": true, "COPY": true, "CORRESPONDING": true,
	"COUNT": true, "COUNTER": true, "CREATE": true, "CROSS": true, "CUBE": true,
	"CURRENT": true, "CURSOR": true, "CYCLE": true, "DATA": true, "DATABASE": true,
	"DATE": true, "DATETIME": true, "DAY": true, "DEALLOCATE": true, "DEC": true,
	"DECIMAL": true, "DECLARE": true, "DEFAULT": true, "DEFERRABLE": true, "DEFERRED": true,
	"DEFINE": true, "DEFINED": true, "DEFINITION": true, "DELETE": true, "DELIMITED": true,
	"DEPTH": true, "DEREF": true, "DESC": true, "DESCRIBE": true, "DESCRIPTOR": true,
	"DETACH": true, "DETERMINISTIC": true, "DIAGNOSTICS": true, "DIRECTORIES": true, "DISABLE": true,
	"DISCONNECT": true, "DISTINCT": true, "DISTRIBUTE": true, "DO": true, "DOMAIN": true,
	"DOUBLE": true, "DROP": true, "DUMP": true, "DURATION": true, "DYNAMIC": true,
	"EACH": true, "ELEMENT": true, "ELSE": true, "ELSEIF": true, "EMPTY": true,
	"ENABLE": true, "END": true, "EQUAL": true, "EQUALS": true, "ERROR": true,
	"ESCAPE": true, "ESCAPED": true, "EVAL": true, "EVALUATE": true, "EXCEEDED": true,
	"EXCEPT": true, "EXCEPTION": true, "EXCEPTIONS": true, "EXCLUSIVE": true, "EXEC": true,
	"EXECUTE": true, "EXISTS": true, "EXIT": true, "EXPLAIN": true, "EXPLODE": true,
	"EXPORT": true, "EXPRESSION": true, "EXTENDED": true, "EXTERNAL": true, "EXTRACT": true,
	"FAIL": true, "FALSE": true, "FAMILY": true, "FETCH": true, "FIELDS": true,
	"FILE": true, "FILTER": true, "FILTERING": true, "FINAL": true, "FINISH": true,
	"FIRST": true, "FIXED": true, "FLATTERN": true, "FLOAT": true, "FOR": true,
	"FORCE": true, "FOREIGN": true, "FORMAT": true, "FORWARD": true, "FOUND": true,
	"FREE": true, "FROM": true, "FULL": true, "FUNCTION": true, "FUNCTIONS": true,
	"GENERAL": true, "GENERATE": true, "GET": true, "GLOB": true, "GLOBAL": true,
	"GO": true, "GOTO": true, "GRANT": true, "GREATER": true, "GROUP": true,
	"GROUPING": true, "HANDLER": true, "HASH": true, "HAVE": true, "HAVING": true,
	"HEAP": true, "HIDDEN": true, "HOLD": true, "HOUR": true, "IDENTIFIED": true,
	"IDENTITY": true, "IF": true, "IGNORE": true, "IMMEDIATE": true, "IMPORT": true,
	"IN": true, "INCLUDING": true, "INCLUSIVE": true, "INCREMENT": true, "INCREMENTAL": true,
	"INDEX": true, "INDEXED": true, "INDEXES": true, "INDICATOR": true, "INFINITE": true,
	"INITIALLY": true, "INLINE": true, "INNER": true, "INNTER": true, "INOUT": true,
	"INPUT": true, "INSENSITIVE": true, "INSERT": true, "INSTEAD": true, "INT": true,
	"INTEGER": true, "INTERSECT": true, "INTERVAL": true, "INTO": true, "INVALIDATE": true,
	"IS": true, "ISOLATION": true, "ITEM": true, "ITEMS": true, "ITERATE": true,
	"JOIN": true, "KEY": true, "KEYS": true, "LAG": true, "LANGUAGE": true,
	"LARGE": true, "LAST": true, "LATERAL": true, "LEAD": true, "LEADING": true,
	"LEAVE": true, "LEFT": true, "LENGTH": true, "LESS": true, "LEVEL": true,
	"LIKE": true, "LIMIT": true, "LIMITED": true, "LINES": true, "LIST": true,
	"LOAD": true, "LOCAL": true, "LOCALTIME": true, "LOCALTIMESTAMP": true, "LOCATION": true,
	"LOCATOR": true, "LOCK": true, "LOCKS": true, "LOG": true, "LOGED": true,
	"LONG": true, "LOOP": true, "LOWER": true, "MAP": true, "MATCH": true,
	"MATERIALIZED": true, "MAX": true, "MAXLEN": true, "MEMBER": true, "MERGE": true,
	"METHOD": true, "METRICS": true, "MIN": true, "MINUS": true, "MINUTE": true,
	"MISSING": true, "MOD": true, "MODE": true, "MODIFIES": true, "MODIFY": true,
	"MODULE": true, "MONTH": true, "MULTI": true, "MULTISET": true, "NAME": true,
	"NAMES": true, "NATIONAL": true, "NATURAL": true, "NCHAR": true, "NCLOB": true,
	"NEW": true, "NEXT": true, "NO": true, "NONE": true, "NOT": true,
	"NULL": true, "NULLIF": true, "NUMBER": true, "NUMERIC": true, "OBJECT": true,
	"OF": true, "OFFLINE": true, "OFFSET": true, "OLD": true, "ON": true,
	"ONLINE": true, "ONLY": true, "OPAQUE": true, "OPEN": true, "OPERATOR": true,
	"OPTION": true, "OR": true, "ORDER": true, "ORDINALITY": true, "OTHER": true,
	"OTHERS": true, "OUT": true, "OUTER": true, "OUTPUT": true, "OVER": true,
	"OVERLAPS": true, "OVERRIDE": true, "OWNER": true, "PAD": true, "PARALLEL": true,
	"PARAMETER": true, "PARAMETERS": true, "PARTIAL": true, "PARTITION": true, "PARTITIONED": true,
	"PARTITIONS": true, "PATH": true, "PERCENT": true, "PERCENTILE": true, "PERMISSION": true,
	"PERMISSIONS": true, "PIPE": true, "PIPELINED": true, "PLAN": true, "POOL": true,
	"POSITION": true, "PRECISION": true, "PREPARE": true, "PRESERVE": true, "PRIMARY": true,
	"PRIOR": true, "PRIVATE": true, "PRIVILEGES": true, "PROCEDURE": true, "PROCESSED": true,
	"PROJECT": true, "PROJECTION": true, "PROPERTY": true, "PROVISIONING": true, "PUBLIC": true,
	"PUT": true, "QUERY": true, "QUIT": true, "QUORUM": true, "RAISE": true,
	"RANDOM": true, "RANGE": true, "RANK": true, "RAW": true, "READ": true,
	"READS": true, "REAL": true, "REBUILD": true, "RECORD": true, "RECURSIVE": true,
	"REDUCE": true, "REF": true, "REFERENCE": true, "REFERENCES": true, "REFERENCING": true,
	"REGEXP": true, "REGION": true, "REINDEX": true, "RELATIVE": true, "RELEASE": true,
	"REMAINDER": true, "RENAME": true, "REPEAT": true, "REPLACE": true, "REQUEST": true,
	"RESET": true, "RESIGNAL": true, "RESOURCE": true, "RESPONSE": true, "RESTORE": true,
	"RESTRICT": true, "RESULT": true, "RETURN": true, "RETURNING": true, "RETURNS": true,
	"REVERSE": true, "REVOKE": true, "RIGHT": true, "ROLE": true, "ROLES": true,
	"ROLLBACK": true, "ROLLUP": true, "ROUTINE": true, "ROW": true, "ROWS": true,
	"RULE": true, "RULES": true, "SAMPLE": true, "SATISFIES": true, "SAVE": true,
	"SAVEPOINT": true, "SCAN": true, "SCHEMA": true, "SCOPE": true, "SCROLL": true,
	"SEARCH": true, "SECOND": true, "SECTION": true, "SEGMENT": true, "SEGMENTS": true,
	"SELECT": true, "SELF": true, "SEMI": true, "SENSITIVE": true, "SEPARATE": true,
	"SEQUENCE": true, "SERIALIZABLE": true, "SESSION": true, "SET": true, "SETS": true,
	"SHARD": true, "SHARE": true, "SHARED": true, "SHORT": true, "SHOW": true,
	"SIGNAL": true, "SIMILAR": true, "SIZE": true, "SKEWED": true, "SMALLINT": true,
	"SNAPSHOT": true, "SOME": true, "SOURCE": true, "SPACE": true, "SPACES": true,
	"SPARSE": true, "SPECIFIC": true, "SPECIFICTYPE": true, "SPLIT": true, "SQL": true,
	"SQLCODE": true, "SQLERROR": true, "SQLEXCEPTION": true, "SQLSTATE": true, "SQLWARNING": true,
	"START": true, "STATE": true, "STATIC": true, "STATUS": true, "STORAGE": true,
	"STORE": true, "STORED": true, "STREAM": true, "STRING": true, "STRUCT": true,
	"STYLE": true, "SUB": true, "SUBMULTISET": true, "SUBPARTITION": true, "SUBSTRING": true,
	"SUBTYPE": true, "SUM": true, "SUPER": true, "SYMMETRIC": true, "SYNONYM": true,
	"SYSTEM": true, "TABLE": true, "TABLESAMPLE": true, "TEMP": true, "TEMPORARY": true,
	"TERMINATED": true, "TEXT": true, "THAN": true, "THEN": true, "THROUGHPUT": true,
	"TIME": true, "TIMESTAMP": true, "TIMEZONE": true, "TINYINT": true, "TO": true,
	"TOKEN": true, "TOTAL": true, "TOUCH": true, "TRAILING": true, "TRANSACTION": true,
	"TRANSFORM": true, "TRANSLATE": true, "TRANSLATION": true, "TREAT": true, "TRIGGER": true,
	"TRIM": true, "TRUE": true, "TRUNCATE": true, "TTL": true, "TUPLE": true,
	"TYPE": true, "UNDER": true, "UNDO": true, "UNION": true, "UNIQUE": true,
	"UNIT": true, "UNKNOWN": true, "UNLOGGED": true, "UNNEST": true, "UNPROCESSED": true,
	"UNSIGNED": true, "UNTIL": true, "UPDATE": true, "UPPER": true, "URL": true,
	"USAGE": true, "USE": true, "USER": true, "USERS": true, "USING": true,
	"UUID": true, "VACUUM": true, "VALUE": true, "VALUED": true, "VALUES": true,
	"VARCHAR": true, "VARIABLE": true, "VARIANCE": true, "VARINT": true, "VARYING": true,
	"VIEW": true, "VIEWS": true, "VIRTUAL": true, "VOID": true, "WAIT": true,
	"WHEN": true, "WHENEVER": true, "WHERE": true, "WHILE": true, "WINDOW": true,
	"WITH": true, "WITHIN": true, "WITHOUT": true, "WORK": true, "WRAPPED": true,
	"WRITE": true, "YEAR": true, "ZONE": true,
}

// Builder compiles expressions for DynamoDB operations
type Builder struct {
	// Expression components
	keyConditions     []string
	filterConditions  []string
	updateExpressions map[string][]string // SET, ADD, REMOVE, DELETE
	conditions        []string
	projections       []string

	// Attribute mappings
	names  map[string]string
	values map[string]types.AttributeValue

	// Counters for placeholder generation
	nameCounter  int
	valueCounter int

	// Complex condition support
	currentGroup  []string
	groupStack    [][]string
	operatorStack []string
}

// NewBuilder creates a new expression builder
func NewBuilder() *Builder {
	return &Builder{
		names:             make(map[string]string),
		values:            make(map[string]types.AttributeValue),
		updateExpressions: make(map[string][]string),
		groupStack:        make([][]string, 0),
		operatorStack:     make([]string, 0),
	}
}

// BeginGroup starts a new expression group (for parentheses)
func (b *Builder) BeginGroup() *Builder {
	b.groupStack = append(b.groupStack, b.currentGroup)
	b.currentGroup = []string{}
	return b
}

// EndGroup ends the current expression group
func (b *Builder) EndGroup() *Builder {
	if len(b.groupStack) == 0 {
		return b // No group to end
	}

	// Build the group expression
	groupExpr := "(" + strings.Join(b.currentGroup, " AND ") + ")"

	// Pop the previous group
	b.currentGroup = b.groupStack[len(b.groupStack)-1]
	b.groupStack = b.groupStack[:len(b.groupStack)-1]

	// Add the group expression to the current context
	b.currentGroup = append(b.currentGroup, groupExpr)

	return b
}

// Or adds an OR operator (changes the default AND behavior)
func (b *Builder) Or() *Builder {
	if len(b.currentGroup) > 0 {
		// Mark that the next condition should be ORed
		b.operatorStack = append(b.operatorStack, "OR")
	}
	return b
}

// And explicitly adds an AND operator (this is the default)
func (b *Builder) And() *Builder {
	if len(b.currentGroup) > 0 {
		b.operatorStack = append(b.operatorStack, "AND")
	}
	return b
}

// AddKeyCondition adds a key condition expression
func (b *Builder) AddKeyCondition(field string, operator string, value interface{}) error {
	expr, err := b.buildCondition(field, operator, value)
	if err != nil {
		return err
	}
	b.keyConditions = append(b.keyConditions, expr)
	return nil
}

// AddFilterCondition adds a filter condition expression
func (b *Builder) AddFilterCondition(field string, operator string, value interface{}) error {
	expr, err := b.buildCondition(field, operator, value)
	if err != nil {
		return err
	}
	b.filterConditions = append(b.filterConditions, expr)
	return nil
}

// AddRawFilter adds a raw filter expression with parameters
func (b *Builder) AddRawFilter(expr string, params map[string]interface{}) error {
	// Replace parameter placeholders with actual value references
	processedExpr := expr
	for name, value := range params {
		placeholder := ":" + name
		if strings.Contains(expr, placeholder) {
			valueRef := b.addValue(value)
			processedExpr = strings.ReplaceAll(processedExpr, placeholder, valueRef)
		}
	}

	// Process attribute names
	processedExpr = b.processAttributeNames(processedExpr)

	b.filterConditions = append(b.filterConditions, processedExpr)
	return nil
}

// AddProjection adds fields to the projection expression
func (b *Builder) AddProjection(fields ...string) {
	for _, field := range fields {
		nameRef := b.addName(field)
		b.projections = append(b.projections, nameRef)
	}
}

// AddUpdateSet adds a SET update expression
func (b *Builder) AddUpdateSet(field string, value interface{}) {
	nameRef := b.addName(field)
	valueRef := b.addValue(value)
	expr := fmt.Sprintf("%s = %s", nameRef, valueRef)
	b.updateExpressions["SET"] = append(b.updateExpressions["SET"], expr)
}

// AddUpdateAdd adds an ADD update expression (for numeric increment)
func (b *Builder) AddUpdateAdd(field string, value interface{}) {
	nameRef := b.addName(field)
	valueRef := b.addValue(value)
	expr := fmt.Sprintf("%s %s", nameRef, valueRef)
	b.updateExpressions["ADD"] = append(b.updateExpressions["ADD"], expr)
}

// AddUpdateRemove adds a REMOVE update expression
func (b *Builder) AddUpdateRemove(field string) {
	nameRef := b.addName(field)
	b.updateExpressions["REMOVE"] = append(b.updateExpressions["REMOVE"], nameRef)
}

// AddConditionExpression adds a condition for conditional updates
func (b *Builder) AddConditionExpression(field string, operator string, value interface{}) error {
	expr, err := b.buildCondition(field, operator, value)
	if err != nil {
		return err
	}
	b.conditions = append(b.conditions, expr)
	return nil
}

// Build compiles all expressions and returns the final components
func (b *Builder) Build() ExpressionComponents {
	components := ExpressionComponents{
		ExpressionAttributeNames:  b.names,
		ExpressionAttributeValues: b.values,
	}

	// Build key condition expression
	if len(b.keyConditions) > 0 {
		components.KeyConditionExpression = strings.Join(b.keyConditions, " AND ")
	}

	// Build filter expression
	if len(b.filterConditions) > 0 {
		components.FilterExpression = strings.Join(b.filterConditions, " AND ")
	}

	// Build projection expression
	if len(b.projections) > 0 {
		components.ProjectionExpression = strings.Join(b.projections, ", ")
	}

	// Build update expression
	if len(b.updateExpressions) > 0 {
		var parts []string
		for action, exprs := range b.updateExpressions {
			if len(exprs) > 0 {
				parts = append(parts, fmt.Sprintf("%s %s", action, strings.Join(exprs, ", ")))
			}
		}
		components.UpdateExpression = strings.Join(parts, " ")
	}

	// Build condition expression
	if len(b.conditions) > 0 {
		components.ConditionExpression = strings.Join(b.conditions, " AND ")
	}

	return components
}

// buildCondition builds a single condition expression
func (b *Builder) buildCondition(field string, operator string, value interface{}) (string, error) {
	nameRef := b.addName(field)

	switch strings.ToUpper(operator) {
	case "=", "EQ":
		valueRef := b.addValue(value)
		return fmt.Sprintf("%s = %s", nameRef, valueRef), nil

	case "!=", "<>", "NE":
		valueRef := b.addValue(value)
		return fmt.Sprintf("%s <> %s", nameRef, valueRef), nil

	case "<", "LT":
		valueRef := b.addValue(value)
		return fmt.Sprintf("%s < %s", nameRef, valueRef), nil

	case "<=", "LE":
		valueRef := b.addValue(value)
		return fmt.Sprintf("%s <= %s", nameRef, valueRef), nil

	case ">", "GT":
		valueRef := b.addValue(value)
		return fmt.Sprintf("%s > %s", nameRef, valueRef), nil

	case ">=", "GE":
		valueRef := b.addValue(value)
		return fmt.Sprintf("%s >= %s", nameRef, valueRef), nil

	case "BETWEEN":
		// Value should be []interface{} with two elements
		values, ok := value.([]interface{})
		if !ok || len(values) != 2 {
			return "", errors.New("BETWEEN operator requires two values")
		}
		valueRef1 := b.addValue(values[0])
		valueRef2 := b.addValue(values[1])
		return fmt.Sprintf("%s BETWEEN %s AND %s", nameRef, valueRef1, valueRef2), nil

	case "IN":
		// Value should be a slice
		values, err := b.convertToSlice(value)
		if err != nil {
			return "", err
		}
		if len(values) > 100 {
			return "", errors.New("IN operator supports maximum 100 values")
		}
		var valueRefs []string
		for _, v := range values {
			valueRefs = append(valueRefs, b.addValue(v))
		}
		return fmt.Sprintf("%s IN (%s)", nameRef, strings.Join(valueRefs, ", ")), nil

	case "BEGINS_WITH":
		valueRef := b.addValue(value)
		return fmt.Sprintf("begins_with(%s, %s)", nameRef, valueRef), nil

	case "CONTAINS":
		valueRef := b.addValue(value)
		return fmt.Sprintf("contains(%s, %s)", nameRef, valueRef), nil

	case "EXISTS":
		return fmt.Sprintf("attribute_exists(%s)", nameRef), nil

	case "NOT_EXISTS":
		return fmt.Sprintf("attribute_not_exists(%s)", nameRef), nil

	default:
		return "", fmt.Errorf("%w: %s", dynamormErrors.ErrInvalidOperator, operator)
	}
}

// addName adds an attribute name and returns its placeholder
func (b *Builder) addName(name string) string {
	// Check if already added
	for placeholder, attrName := range b.names {
		if attrName == name {
			return placeholder
		}
	}

	// Check if it's a reserved word
	if b.isReservedWord(name) {
		// Always use placeholder for reserved words
		b.nameCounter++
		placeholder := fmt.Sprintf("#%s", name)
		b.names[placeholder] = name
		return placeholder
	}

	// For nested attributes, process each part
	if strings.Contains(name, ".") {
		parts := strings.Split(name, ".")
		processedParts := make([]string, len(parts))

		for i, part := range parts {
			if b.isReservedWord(part) {
				b.nameCounter++
				placeholder := fmt.Sprintf("#%s", part)
				b.names[placeholder] = part
				processedParts[i] = placeholder
			} else {
				// Check if we need a placeholder for consistency
				b.nameCounter++
				placeholder := fmt.Sprintf("#n%d", b.nameCounter)
				b.names[placeholder] = part
				processedParts[i] = placeholder
			}
		}

		return strings.Join(processedParts, ".")
	}

	// Generate new placeholder
	b.nameCounter++
	placeholder := fmt.Sprintf("#n%d", b.nameCounter)
	b.names[placeholder] = name
	return placeholder
}

// isReservedWord checks if a word is reserved in DynamoDB
func (b *Builder) isReservedWord(word string) bool {
	return reservedWords[strings.ToUpper(word)]
}

// addValue adds an attribute value and returns its placeholder
func (b *Builder) addValue(value interface{}) string {
	b.valueCounter++
	placeholder := fmt.Sprintf(":v%d", b.valueCounter)

	// Convert value to AttributeValue
	av, err := ConvertToAttributeValue(value)
	if err != nil {
		// For now, store as string
		av = &types.AttributeValueMemberS{Value: fmt.Sprintf("%v", value)}
	}

	b.values[placeholder] = av
	return placeholder
}

// processAttributeNames replaces attribute names with placeholders in raw expressions
func (b *Builder) processAttributeNames(expr string) string {
	// Split expression into tokens
	// This is a simple tokenizer - in production, use a proper parser
	tokens := strings.Fields(expr)

	for i, token := range tokens {
		// Skip operators and functions
		if strings.Contains("()=<>!,", token) {
			continue
		}

		// Check if it looks like an attribute name (not a value placeholder)
		if !strings.HasPrefix(token, ":") && !strings.HasPrefix(token, "#") {
			// Check if it's a function name
			if i+1 < len(tokens) && strings.HasPrefix(tokens[i+1], "(") {
				continue
			}

			// Process as attribute name
			if b.isReservedWord(token) || strings.Contains(token, ".") {
				placeholder := b.addName(token)
				tokens[i] = placeholder
			}
		}
	}

	return strings.Join(tokens, " ")
}

// convertToSlice converts various slice types to []interface{}
func (b *Builder) convertToSlice(value interface{}) ([]interface{}, error) {
	switch v := value.(type) {
	case []interface{}:
		return v, nil
	case []string:
		result := make([]interface{}, len(v))
		for i, s := range v {
			result[i] = s
		}
		return result, nil
	case []int:
		result := make([]interface{}, len(v))
		for i, n := range v {
			result[i] = n
		}
		return result, nil
	default:
		return nil, errors.New("value must be a slice for IN operator")
	}
}

// ExpressionComponents holds all expression components
type ExpressionComponents struct {
	KeyConditionExpression    string
	FilterExpression          string
	ProjectionExpression      string
	UpdateExpression          string
	ConditionExpression       string
	ExpressionAttributeNames  map[string]string
	ExpressionAttributeValues map[string]types.AttributeValue
}

// AddAdvancedFunction adds support for DynamoDB functions
func (b *Builder) AddAdvancedFunction(function string, field string, args ...interface{}) (string, error) {
	nameRef := b.addName(field)

	switch strings.ToLower(function) {
	case "size":
		return fmt.Sprintf("size(%s)", nameRef), nil

	case "attribute_type":
		if len(args) != 1 {
			return "", errors.New("attribute_type requires one argument (type)")
		}
		valueRef := b.addValue(args[0])
		return fmt.Sprintf("attribute_type(%s, %s)", nameRef, valueRef), nil

	case "attribute_exists":
		return fmt.Sprintf("attribute_exists(%s)", nameRef), nil

	case "attribute_not_exists":
		return fmt.Sprintf("attribute_not_exists(%s)", nameRef), nil

	case "list_append":
		if len(args) != 1 {
			return "", errors.New("list_append requires one argument (value to append)")
		}
		valueRef := b.addValue(args[0])
		return fmt.Sprintf("list_append(%s, %s)", nameRef, valueRef), nil

	default:
		return "", fmt.Errorf("unsupported function: %s", function)
	}
}

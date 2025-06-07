package index

import (
	"strings"

	"github.com/pay-theory/dynamorm/pkg/core"
)

// Selector helps select the optimal index for a query
type Selector struct {
	indexes []core.IndexSchema
}

// NewSelector creates a new index selector
func NewSelector(indexes []core.IndexSchema) *Selector {
	return &Selector{
		indexes: indexes,
	}
}

// RequiredKeys represents the keys needed by a query
type RequiredKeys struct {
	PartitionKey string
	SortKey      string
	SortKeyOp    string // "=", "begins_with", "between", etc.
}

// SelectOptimal selects the best index for the given query requirements
func (s *Selector) SelectOptimal(required RequiredKeys, conditions []interface{}) (*core.IndexSchema, error) {
	var bestIndex *core.IndexSchema
	var bestScore int

	// First check the primary index (table)
	primaryScore := s.scoreIndex(core.IndexSchema{
		Name:         "",
		Type:         "PRIMARY",
		PartitionKey: required.PartitionKey,
		SortKey:      required.SortKey,
	}, required)

	if primaryScore > 0 {
		bestIndex = &core.IndexSchema{
			Name: "",
			Type: "PRIMARY",
		}
		bestScore = primaryScore
	}

	// Check GSIs and LSIs
	for _, idx := range s.indexes {
		score := s.scoreIndex(idx, required)
		if score > bestScore {
			bestScore = score
			idxCopy := idx // Make a copy to avoid pointer issues
			bestIndex = &idxCopy
		}
	}

	// If no suitable index found, scanning is required
	if bestIndex == nil {
		return nil, nil
	}

	return bestIndex, nil
}

// scoreIndex calculates a score for how well an index matches the requirements
func (s *Selector) scoreIndex(idx core.IndexSchema, required RequiredKeys) int {
	score := 0

	// Partition key must match exactly for Query operation
	if idx.PartitionKey != required.PartitionKey {
		return 0
	}
	score += 100 // Base score for partition key match

	// Sort key scoring
	if required.SortKey != "" && idx.SortKey == required.SortKey {
		switch required.SortKeyOp {
		case "=":
			score += 50 // Exact match on sort key
		case "begins_with":
			score += 40 // Prefix match
		case "between", "<", "<=", ">", ">=":
			score += 30 // Range query
		}
	}

	// Prefer GSI over LSI for better performance isolation
	if idx.Type == "GSI" {
		score += 10
	}

	// Prefer indexes with ALL projection for flexibility
	if idx.ProjectionType == "ALL" {
		score += 5
	}

	return score
}

// AnalyzeConditions analyzes query conditions to determine key requirements
func AnalyzeConditions(conditions []Condition) RequiredKeys {
	var required RequiredKeys

	for _, cond := range conditions {
		// Look for partition key conditions (must be equality)
		if cond.Operator == "=" || strings.EqualFold(cond.Operator, "eq") {
			if required.PartitionKey == "" {
				required.PartitionKey = cond.Field
			}
		}

		// Look for sort key conditions
		if required.SortKey == "" && required.PartitionKey != "" && cond.Field != required.PartitionKey {
			required.SortKey = cond.Field
			required.SortKeyOp = normalizeOperator(cond.Operator)
		}
	}

	return required
}

// normalizeOperator converts operator variations to standard form
func normalizeOperator(op string) string {
	switch strings.ToUpper(op) {
	case "EQ", "=":
		return "="
	case "LT", "<":
		return "<"
	case "LE", "<=":
		return "<="
	case "GT", ">":
		return ">"
	case "GE", ">=":
		return ">="
	case "BEGINS_WITH":
		return "begins_with"
	case "BETWEEN":
		return "between"
	default:
		return strings.ToLower(op)
	}
}

// Condition represents a query condition (moved from query package to avoid circular dependency)
type Condition struct {
	Field    string
	Operator string
	Value    interface{}
}

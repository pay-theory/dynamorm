package query

import "github.com/pay-theory/dynamorm/pkg/core"

type cov5Metadata struct {
	table        string
	primaryKey   core.KeySchema
	indexes      []core.IndexSchema
	attributes   map[string]*core.AttributeMetadata
	versionField string
}

func (m cov5Metadata) TableName() string { return m.table }

func (m cov5Metadata) PrimaryKey() core.KeySchema { return m.primaryKey }

func (m cov5Metadata) Indexes() []core.IndexSchema {
	return append([]core.IndexSchema(nil), m.indexes...)
}

func (m cov5Metadata) AttributeMetadata(field string) *core.AttributeMetadata {
	if m.attributes == nil {
		return nil
	}
	return m.attributes[field]
}

func (m cov5Metadata) VersionFieldName() string { return m.versionField }

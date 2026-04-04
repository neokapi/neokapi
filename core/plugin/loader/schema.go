// Package loader re-exports schema types from core/format/schema for backward compatibility.
// New code should import core/format/schema directly.
package loader

import "github.com/neokapi/neokapi/core/format/schema"

// Type aliases for backward compatibility.
type FormatSchema = schema.FormatSchema
type FormatMeta = schema.FormatMeta
type ParameterGroup = schema.ParameterGroup
type PropertySchema = schema.PropertySchema
type SchemaRegistry = schema.SchemaRegistry

// NewSchemaRegistry creates a new schema registry.
//
// Deprecated: Use schema.NewSchemaRegistry() directly.
var NewSchemaRegistry = schema.NewSchemaRegistry

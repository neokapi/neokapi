package model

import "strings"

// EntityType classifies named entities found in content.
type EntityType string

const (
	EntityPerson       EntityType = "entity:person"
	EntityOrganization EntityType = "entity:organization"
	EntityProduct      EntityType = "entity:product"
	EntityLocation     EntityType = "entity:location"
	EntityDate         EntityType = "entity:date"
	EntityTime         EntityType = "entity:time"
	EntityCurrency     EntityType = "entity:currency"
	EntityMeasurement  EntityType = "entity:measurement"
	EntityOther        EntityType = "entity:other"

	// EntityPrefix is the prefix used for all entity type strings.
	EntityPrefix = "entity:"
)

// IsEntityTypeString returns true if the given type string represents an entity type.
func IsEntityTypeString(typeName string) bool {
	return strings.HasPrefix(typeName, EntityPrefix)
}

// entityTypeLabel extracts the entity label from a type string.
// "entity:person" → "PERSON", "entity:organization" → "ORGANIZATION".
func entityTypeLabel(typeName string) string {
	return strings.ToUpper(strings.TrimPrefix(typeName, EntityPrefix))
}

// EntityAnnotation carries a named entity with its position in source text.
// Implements the Annotation interface. Used by entity-annotate tool,
// TM generalization (ADR-010), and terminology management (ADR-016).
type EntityAnnotation struct {
	Text   string           // the entity text as found in source
	Type   EntityType       // classification
	Locale LocaleID         // locale-specific formatting hint
	DNT    bool             // do-not-translate flag
	Source ExtractionSource // how this entity was discovered ("llm", "ner", "manual")
}

// AnnotationType returns the type identifier for entity annotations.
func (ea *EntityAnnotation) TypeName() string { return "entity" }

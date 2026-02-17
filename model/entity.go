package model

// EntityType classifies named entities found in content.
type EntityType string

const (
	EntityPerson       EntityType = "person"
	EntityOrganization EntityType = "organization"
	EntityProduct      EntityType = "product"
	EntityLocation     EntityType = "location"
	EntityDate         EntityType = "date"
	EntityTime         EntityType = "time"
	EntityCurrency     EntityType = "currency"
	EntityMeasurement  EntityType = "measurement"
	EntityOther        EntityType = "other"
)

// EntityAnnotation carries a named entity with its position in source text.
// Implements the Annotation interface. Used by entity-annotate tool,
// TM generalization (ADR-010), and terminology management (ADR-016).
type EntityAnnotation struct {
	Text     string     // the entity text as found in source
	Type     EntityType // classification
	Position TextRange  // character offset range in source text
	Locale   LocaleID   // locale-specific formatting hint
	DNT      bool       // do-not-translate flag
}

// AnnotationType returns the type identifier for entity annotations.
func (ea *EntityAnnotation) AnnotationType() string { return "entity" }

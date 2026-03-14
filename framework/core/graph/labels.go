package graph

// Edge labels aligned with W3C SKOS vocabulary for terminology interoperability.
const (
	// Hierarchical relationships (SKOS)
	LabelBroader  = "BROADER"  // skos:broader — parent concept
	LabelNarrower = "NARROWER" // skos:narrower — child concept

	// Associative relationships (SKOS)
	LabelRelated = "RELATED" // skos:related — associative link

	// Compositional relationships
	LabelPartOf  = "PART_OF"  // meronymy — component of
	LabelHasPart = "HAS_PART" // holonymy — contains component

	// Terminological relationships
	LabelHasTerm    = "HAS_TERM"     // concept → term designation
	LabelUseInstead = "USE_INSTEAD"  // deprecated term → preferred term
	LabelReplacedBy = "REPLACED_BY"  // superseded concept → replacement

	// Equivalence relationships
	LabelExactMatch = "EXACT_MATCH" // skos:exactMatch — cross-scheme equivalence
	LabelCloseMatch = "CLOSE_MATCH" // skos:closeMatch — approximate equivalence

	// Brand voice relationships
	LabelForbidden = "FORBIDDEN" // brand → forbidden term
	LabelPreferred = "PREFERRED" // brand → preferred term
	LabelCompetitor = "COMPETITOR" // brand → competitor term
)

// InverseLabel returns the inverse of a directional label, or empty string if symmetric.
func InverseLabel(label string) string {
	switch label {
	case LabelBroader:
		return LabelNarrower
	case LabelNarrower:
		return LabelBroader
	case LabelPartOf:
		return LabelHasPart
	case LabelHasPart:
		return LabelPartOf
	default:
		return ""
	}
}

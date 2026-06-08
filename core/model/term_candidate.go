package model

// ExtractionSource identifies how a term candidate or entity was discovered.
type ExtractionSource string

const (
	ExtractionSourceLLM    ExtractionSource = "llm"
	ExtractionSourceNER    ExtractionSource = "ner"
	ExtractionSourceManual ExtractionSource = "manual"
)

// CandidateStatus tracks a term candidate through the review lifecycle.
type CandidateStatus string

const (
	CandidateStatusPending  CandidateStatus = "pending"
	CandidateStatusApproved CandidateStatus = "approved"
	CandidateStatusRejected CandidateStatus = "rejected"
)

// TermCategory classifies a term candidate by domain.
type TermCategory string

const (
	TermCategoryBrand     TermCategory = "brand"
	TermCategoryTechnical TermCategory = "technical"
	TermCategoryUI        TermCategory = "ui"
	TermCategoryLegal     TermCategory = "legal"
	TermCategoryMarketing TermCategory = "marketing"
	TermCategoryGeneral   TermCategory = "general"
)

// Translatability indicates how a term should be handled during translation.
type Translatability string

const (
	// TranslatabilityDNT means the term should never be translated (brand names, acronyms).
	TranslatabilityDNT Translatability = "dnt"
	// TranslatabilityConsistent means the term should be translated the same way everywhere.
	TranslatabilityConsistent Translatability = "consistent"
	// TranslatabilityFree means the term can be translated naturally without consistency requirements.
	TranslatabilityFree Translatability = "free"
)

// TermCandidateAnnotation carries a proposed term that needs human review.
// Implements the Annotation interface. Produced by the AI entity-extract tool
// and by manual marking in the translation editor.
//
// Distinct from TermAnnotation, which represents a confirmed match against
// an existing termbase entry.
type TermCandidateAnnotation struct {
	Text            string           // the term text as found in source
	Definition      string           // AI-proposed or user-provided definition
	Category        TermCategory     // domain classification
	Translatability Translatability  // how the term should be handled during translation
	Confidence      float64          // extraction confidence [0,1]
	Locale          LocaleID         // locale where the term was found
	Source          ExtractionSource // how this candidate was discovered
	Status          CandidateStatus  // review lifecycle state
}

// AnnotationType returns the type identifier for term candidate annotations.
func (tc *TermCandidateAnnotation) TypeName() string { return "term-candidate" }

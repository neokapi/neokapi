package termbase

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// CSVImportOptions controls how CSV data is interpreted.
type CSVImportOptions struct {
	Delimiter    rune           // field delimiter (default: comma)
	HasHeader    bool           // first row is header
	SourceLocale model.LocaleID // locale for source column
	TargetLocale model.LocaleID // locale for target column
	Domain       string         // domain to assign to imported concepts
	IDPrefix     string         // prefix for generated concept IDs

	// Monolingual imports a single-locale concept list — one term column plus
	// an optional definition column — instead of source/target term pairs. Each
	// row contributes one term in SourceLocale; TargetLocale is ignored. This is
	// the path for concept or brand-vocabulary lists that have no translation.
	Monolingual bool
}

// ImportCSV reads a CSV file of terms and imports them as concepts.
// Bilingual (default): source_term, target_term[, domain][, definition][, status].
// Monolingual (opts.Monolingual): term[, definition][, domain][, status] — one
// term per concept in SourceLocale, with no translation pair.
// Returns the number of concepts imported.
func ImportCSV(ctx context.Context, tb TermBase, reader io.Reader, opts CSVImportOptions) (int, error) {
	csvReader := csv.NewReader(reader)
	if opts.Delimiter != 0 {
		csvReader.Comma = opts.Delimiter
	}
	csvReader.FieldsPerRecord = -1 // allow variable field count
	csvReader.TrimLeadingSpace = true

	records, err := csvReader.ReadAll()
	if err != nil {
		return 0, fmt.Errorf("read CSV: %w", err)
	}

	if len(records) == 0 {
		return 0, nil
	}

	startIdx := 0
	if opts.HasHeader {
		startIdx = 1
	}

	imported := 0
	prefix := opts.IDPrefix
	if prefix == "" {
		prefix = "csv"
	}

	for i := startIdx; i < len(records); i++ {
		row := records[i]
		conceptID := fmt.Sprintf("%s-%d", prefix, i-startIdx+1)

		var (
			concept Concept
			ok      bool
		)
		if opts.Monolingual {
			concept, ok = monolingualConcept(row, conceptID, opts)
		} else {
			concept, ok = bilingualConcept(row, conceptID, opts)
		}
		if !ok {
			continue
		}

		if err := tb.AddConcept(ctx, concept); err != nil {
			return imported, fmt.Errorf("add concept %s: %w", conceptID, err)
		}
		imported++
	}

	return imported, nil
}

// bilingualConcept builds a concept from a source/target CSV row. Layout:
// source_term, target_term[, domain][, definition][, status][, term_source][, competitor].
// Returns ok=false when the row lacks a source or target term.
func bilingualConcept(row []string, conceptID string, opts CSVImportOptions) (Concept, bool) {
	if len(row) < 2 {
		return Concept{}, false
	}

	sourceTerm := strings.TrimSpace(row[0])
	targetTerm := strings.TrimSpace(row[1])
	if sourceTerm == "" || targetTerm == "" {
		return Concept{}, false
	}

	domain := opts.Domain
	if len(row) > 2 && strings.TrimSpace(row[2]) != "" {
		domain = strings.TrimSpace(row[2])
	}

	definition := ""
	if len(row) > 3 {
		definition = strings.TrimSpace(row[3])
	}

	status := model.TermApproved
	if len(row) > 4 {
		if s := parseTermStatus(strings.TrimSpace(row[4])); s != "" {
			status = s
		}
	}

	var termSource TermSource
	if len(row) > 5 {
		if s := strings.TrimSpace(row[5]); s == string(TermSourceBrandVocabulary) {
			termSource = TermSourceBrandVocabulary
		}
	}

	competitorTerm := false
	if len(row) > 6 {
		competitorTerm = strings.EqualFold(strings.TrimSpace(row[6]), "true")
	}

	return Concept{
		ID:         conceptID,
		Domain:     domain,
		Definition: definition,
		Source:     termSource,
		Terms: []Term{
			{
				Text:           sourceTerm,
				Locale:         opts.SourceLocale,
				Status:         status,
				CompetitorTerm: competitorTerm,
			},
			{
				Text:           targetTerm,
				Locale:         opts.TargetLocale,
				Status:         status,
				CompetitorTerm: competitorTerm,
			},
		},
	}, true
}

// monolingualConcept builds a single-locale concept from a CSV row. Layout:
// term[, definition][, domain][, status][, term_source][, competitor]. The term
// is placed in opts.SourceLocale; opts.TargetLocale is ignored. This lets a
// concept or brand-vocabulary list import without a translation pair.
// Returns ok=false when the row lacks a term.
func monolingualConcept(row []string, conceptID string, opts CSVImportOptions) (Concept, bool) {
	if len(row) < 1 {
		return Concept{}, false
	}

	term := strings.TrimSpace(row[0])
	if term == "" {
		return Concept{}, false
	}

	definition := ""
	if len(row) > 1 {
		definition = strings.TrimSpace(row[1])
	}

	domain := opts.Domain
	if len(row) > 2 && strings.TrimSpace(row[2]) != "" {
		domain = strings.TrimSpace(row[2])
	}

	status := model.TermApproved
	if len(row) > 3 {
		if s := parseTermStatus(strings.TrimSpace(row[3])); s != "" {
			status = s
		}
	}

	var termSource TermSource
	if len(row) > 4 {
		if s := strings.TrimSpace(row[4]); s == string(TermSourceBrandVocabulary) {
			termSource = TermSourceBrandVocabulary
		}
	}

	competitorTerm := false
	if len(row) > 5 {
		competitorTerm = strings.EqualFold(strings.TrimSpace(row[5]), "true")
	}

	return Concept{
		ID:         conceptID,
		Domain:     domain,
		Definition: definition,
		Source:     termSource,
		Terms: []Term{
			{
				Text:           term,
				Locale:         opts.SourceLocale,
				Status:         status,
				CompetitorTerm: competitorTerm,
			},
		},
	}, true
}

// ExportCSV writes all concepts as CSV source/target pairs.
func ExportCSV(ctx context.Context, tb TermBase, writer io.Writer, sourceLocale, targetLocale model.LocaleID, includeHeader bool) error {
	csvWriter := csv.NewWriter(writer)
	defer csvWriter.Flush()

	if includeHeader {
		if err := csvWriter.Write([]string{"source", "target", "domain", "definition", "status", "concept_id", "term_source", "competitor_term"}); err != nil {
			return fmt.Errorf("write CSV header: %w", err)
		}
	}

	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return fmt.Errorf("list concepts: %w", err)
	}
	for _, concept := range concepts {
		sourceTerm := concept.SourceTerm(sourceLocale)
		if sourceTerm == nil {
			continue
		}

		for _, target := range concept.TargetTerms(targetLocale) {
			competitorStr := ""
			if target.CompetitorTerm {
				competitorStr = "true"
			}
			if err := csvWriter.Write([]string{
				sourceTerm.Text,
				target.Text,
				concept.Domain,
				concept.Definition,
				string(target.Status),
				concept.ID,
				string(concept.Source),
				competitorStr,
			}); err != nil {
				return fmt.Errorf("write CSV row: %w", err)
			}
		}
	}

	return csvWriter.Error()
}

func parseTermStatus(s string) model.TermStatus {
	switch strings.ToLower(s) {
	case "proposed":
		return model.TermProposed
	case "approved":
		return model.TermApproved
	case "preferred":
		return model.TermPreferred
	case "admitted":
		return model.TermAdmitted
	case "deprecated":
		return model.TermDeprecated
	case "forbidden":
		return model.TermForbidden
	default:
		return ""
	}
}

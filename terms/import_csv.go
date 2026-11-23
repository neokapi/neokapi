package terms

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

	// Monolingual imports a single-locale concept list — one term column plus
	// an optional definition column — instead of source/target term pairs. Each
	// row contributes one term in SourceLocale; TargetLocale is ignored. This is
	// the path for concept or voice-vocabulary lists that have no translation.
	Monolingual bool
}

// ImportCSV reads a CSV file of terms and imports them as concepts.
// Bilingual (default): source_term, target_term[, domain][, definition][, status].
// Monolingual (opts.Monolingual): term[, definition][, domain][, status] — one
// term per concept in SourceLocale, with no translation pair.
// Each concept takes the id a concept_id column gives it, or ImportID of its
// source term, so importing the same file again writes the same concepts.
// Returns the number of concepts imported.
func ImportCSV(ctx context.Context, tb Terminology, reader io.Reader, opts CSVImportOptions) (int, error) {
	rows, err := ParseCSV(reader, opts)
	if err != nil {
		return 0, err
	}
	return writeRows(ctx, tb, rows)
}

// writeRows writes each row's concept to tb, in file order.
func writeRows(ctx context.Context, tb Terminology, rows []ImportRow) (int, error) {
	imported := 0
	for _, row := range rows {
		if err := tb.AddConcept(ctx, row.Concept); err != nil {
			return imported, fmt.Errorf("add concept %s: %w", row.Concept.ID, err)
		}
		imported++
	}
	return imported, nil
}

// bilingualConcept builds a concept from a source/target CSV row. Layout:
// source_term, target_term[, domain][, definition][, status][, term_source][, competitor][, advisory].
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
		Advisory:   len(row) > 7 && strings.EqualFold(strings.TrimSpace(row[7]), "true"),
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
// term[, definition][, domain][, status][, term_source][, competitor][, advisory]. The term
// is placed in opts.SourceLocale; opts.TargetLocale is ignored. This lets a
// concept or voice-vocabulary list import without a translation pair.
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
		Advisory:   len(row) > 6 && strings.EqualFold(strings.TrimSpace(row[6]), "true"),
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

// ExportCSV writes all concepts as CSV source/target pairs, in the bilingual
// layout ImportCSV reads, so an export imports back with its term source,
// competitor and advisory markings. The concept ID follows as a last column,
// which ImportCSV ignores.
//
// The flush is explicit, and its error is the export's error. csv.Writer
// buffers, so Write reports only what has already spilled to the destination —
// an export smaller than the buffer reaches it for the first time at Flush, and
// that is where a full disk or a closed pipe announces itself. Deferring the
// flush puts it after the return value is computed, which is how an export that
// wrote nothing at all can report success.
func ExportCSV(ctx context.Context, tb Terminology, writer io.Writer, sourceLocale, targetLocale model.LocaleID, includeHeader bool) error {
	csvWriter := csv.NewWriter(writer)

	if includeHeader {
		if err := csvWriter.Write([]string{"source", "target", "domain", "definition", "status", "term_source", "competitor_term", "advisory", "concept_id"}); err != nil {
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
			advisoryStr := ""
			if concept.Advisory {
				advisoryStr = "true"
			}
			if err := csvWriter.Write([]string{
				sourceTerm.Text,
				target.Text,
				concept.Domain,
				concept.Definition,
				string(target.Status),
				string(concept.Source),
				competitorStr,
				advisoryStr,
				concept.ID,
			}); err != nil {
				return fmt.Errorf("write CSV row: %w", err)
			}
		}
	}

	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return fmt.Errorf("flush CSV: %w", err)
	}
	return nil
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

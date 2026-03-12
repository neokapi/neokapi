package termbase

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/gokapi/gokapi/core/model"
)

// CSVImportOptions controls how CSV data is interpreted.
type CSVImportOptions struct {
	Delimiter    rune           // field delimiter (default: comma)
	HasHeader    bool           // first row is header
	SourceLocale model.LocaleID // locale for source column
	TargetLocale model.LocaleID // locale for target column
	Domain       string         // domain to assign to imported concepts
	IDPrefix     string         // prefix for generated concept IDs
}

// ImportCSV reads a CSV file with source/target term pairs and imports them.
// Expected format: source_term, target_term[, domain][, definition][, status]
// Returns the number of concepts imported.
func ImportCSV(tb TermBase, reader io.Reader, opts CSVImportOptions) (int, error) {
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
		if len(row) < 2 {
			continue
		}

		sourceTerm := strings.TrimSpace(row[0])
		targetTerm := strings.TrimSpace(row[1])
		if sourceTerm == "" || targetTerm == "" {
			continue
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

		conceptID := fmt.Sprintf("%s-%d", prefix, i-startIdx+1)

		concept := Concept{
			ID:         conceptID,
			Domain:     domain,
			Definition: definition,
			Terms: []Term{
				{
					Text:   sourceTerm,
					Locale: opts.SourceLocale,
					Status: status,
				},
				{
					Text:   targetTerm,
					Locale: opts.TargetLocale,
					Status: status,
				},
			},
		}

		if err := tb.AddConcept(concept); err != nil {
			return imported, fmt.Errorf("add concept %s: %w", conceptID, err)
		}
		imported++
	}

	return imported, nil
}

// ExportCSV writes all concepts as CSV source/target pairs.
func ExportCSV(tb TermBase, writer io.Writer, sourceLocale, targetLocale model.LocaleID, includeHeader bool) error {
	csvWriter := csv.NewWriter(writer)
	defer csvWriter.Flush()

	if includeHeader {
		if err := csvWriter.Write([]string{"source", "target", "domain", "definition", "status", "concept_id"}); err != nil {
			return fmt.Errorf("write CSV header: %w", err)
		}
	}

	for _, concept := range tb.Concepts() {
		sourceTerm := concept.SourceTerm(sourceLocale)
		if sourceTerm == nil {
			continue
		}

		for _, target := range concept.TargetTerms(targetLocale) {
			if err := csvWriter.Write([]string{
				sourceTerm.Text,
				target.Text,
				concept.Domain,
				concept.Definition,
				string(target.Status),
				concept.ID,
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

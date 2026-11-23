package terms

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// ImportRow is one concept a terms file holds, and where it came from.
type ImportRow struct {
	// Row is where the concept sits in the file: the CSV line, counted from
	// 1 with any header line included, or the entry's position, from 1, in
	// a JSON or TBX file.
	Row int
	// Concept is the concept as the file states it.
	Concept Concept
	// IDGiven reports that the file names the concept's id: a concept_id
	// column, a JSON id, or a TBX entry id. Otherwise the id is ImportID of
	// SourceTerm, so the same file imported twice names the same concepts.
	IDGiven bool
	// SourceTerm and SourceLocale are the term the concept is known by in
	// the file, which a derived id is made from.
	SourceTerm   string
	SourceLocale model.LocaleID
	// Document is the name the file declares for itself (a JSON file's
	// name), for display. BaseRevision is the revision the concept had when
	// the file was exported (JSONStore.Revisions), empty when the file says
	// none.
	Document     string
	BaseRevision string
}

// ImportID is the id an imported concept takes when its file names none:
// the id a decision on its source term opens (DecisionConceptID), so an
// import and a decision about the same term meet on one concept. A term
// with no letter or digit the id can keep, such as one in a script other
// than Latin, takes a digest of the term instead, so two such terms never
// share an id.
func ImportID(text string, locale model.LocaleID) string {
	if slug(text) != "" {
		return DecisionConceptID(text, locale)
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(text)))
	return "term:" + string(model.NormalizeLocale(locale)) + ":h-" + hex.EncodeToString(sum[:6])
}

// ParseCSV reads a CSV file of terms into concepts, without writing any.
// The layout is ImportCSV's. With a header, a column named concept_id (the
// one ExportCSV writes) gives each concept its id.
func ParseCSV(reader io.Reader, opts CSVImportOptions) ([]ImportRow, error) {
	csvReader := csv.NewReader(reader)
	if opts.Delimiter != 0 {
		csvReader.Comma = opts.Delimiter
	}
	csvReader.FieldsPerRecord = -1 // allow variable field count
	csvReader.TrimLeadingSpace = true

	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read CSV: %w", err)
	}
	if len(records) == 0 {
		return nil, nil
	}

	startIdx := 0
	idColumn := -1
	if opts.HasHeader {
		startIdx = 1
		for i, name := range records[0] {
			if strings.EqualFold(strings.TrimSpace(name), "concept_id") {
				idColumn = i
			}
		}
	}

	var rows []ImportRow
	for i := startIdx; i < len(records); i++ {
		row := records[i]
		source := ""
		if len(row) > 0 {
			source = strings.TrimSpace(row[0])
		}
		id, given := "", false
		if idColumn >= 0 && idColumn < len(row) {
			id = strings.TrimSpace(row[idColumn])
			given = id != ""
		}
		if !given {
			id = ImportID(source, opts.SourceLocale)
		}

		var (
			concept Concept
			ok      bool
		)
		if opts.Monolingual {
			concept, ok = monolingualConcept(row, id, opts)
		} else {
			concept, ok = bilingualConcept(row, id, opts)
		}
		if !ok {
			continue
		}
		rows = append(rows, ImportRow{
			Row: i + 1, Concept: concept, IDGiven: given,
			SourceTerm: source, SourceLocale: opts.SourceLocale,
		})
	}
	return rows, nil
}

// ParseJSON reads a JSON terms file into concepts, without writing any. A
// concept that names no id takes ImportID of its first term.
func ParseJSON(reader io.Reader) ([]ImportRow, error) {
	var doc JSONStore
	if err := json.NewDecoder(reader).Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse JSON terms: %w", err)
	}
	rows := make([]ImportRow, 0, len(doc.Concepts))
	for i, concept := range doc.Concepts {
		if concept.ID == "" && len(concept.Terms) == 0 {
			// Nothing to know the concept by, and nothing to import.
			continue
		}
		row := rowOf(i+1, concept)
		row.Document = doc.Name
		if row.IDGiven {
			row.BaseRevision = doc.Revisions[concept.ID]
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// rowOf is the import row of a concept read from a file whose entries name
// their id or not, known by its first term.
func rowOf(position int, concept Concept) ImportRow {
	row := ImportRow{Row: position, IDGiven: concept.ID != ""}
	if len(concept.Terms) > 0 {
		row.SourceTerm = strings.TrimSpace(concept.Terms[0].Text)
		row.SourceLocale = concept.Terms[0].Locale
	}
	if !row.IDGiven {
		concept.ID = ImportID(row.SourceTerm, row.SourceLocale)
	}
	row.Concept = concept
	return row
}

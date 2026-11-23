package terms

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/core/model"
	fw "github.com/neokapi/neokapi/terms"
)

// ImportFingerprintProperty is the property an import records on each
// concept it writes: a digest of the content it wrote (importFingerprint).
// The next import compares it with what the concept holds, and a concept
// whose content moved since was edited after the import.
const ImportFingerprintProperty = "import.fingerprint"

// ImportConflict is a row an import did not apply, and why.
type ImportConflict struct {
	Row       int    `json:"row"`
	Term      string `json:"term"`
	ConceptID string `json:"concept_id"`
	Reason    string `json:"reason"`
}

// ImportReport is what an import did with each row of its file.
type ImportReport struct {
	Created   int              `json:"created"`
	Updated   int              `json:"updated"`
	Unchanged int              `json:"unchanged"`
	Conflicts []ImportConflict `json:"conflicts,omitempty"`
}

// Imported counts the concepts the import wrote.
func (r ImportReport) Imported() int { return r.Created + r.Updated }

// ImportRows writes the concepts of a terms file to st.
//
// A row's concept id is the one its file names, or one derived from its
// source term (fw.ImportID), so the same file imported twice changes
// nothing, and two files name the same concept only when they name the
// same term. A row whose derived id would join a different concept than the
// stored concept holding its term (a homonym), and a row that appears twice
// in the file with different content, is reported as a conflict and the
// stored concept is left alone: the remedy is a concept_id column.
//
// A row that changes a stored concept is written at the concept's revision,
// and only when the concept is as the last import left it: one a person
// edited since is reported, never overwritten. A concept no import wrote is
// changed only by a row that names it by id.
func ImportRows(ctx context.Context, st Store, rows []fw.ImportRow) (ImportReport, error) {
	var report ImportReport
	all, err := st.Concepts(ctx)
	if err != nil {
		return report, fmt.Errorf("read the terms: %w", err)
	}
	holders := map[string]string{} // term key -> the concept holding it
	for _, c := range all {
		for _, t := range c.Terms {
			holders[termKey(t.Text, t.Locale)] = c.ID
		}
	}
	inFile := map[string]string{} // concept id -> fingerprint of its first row
	for _, row := range rows {
		id := row.Concept.ID
		fp := importFingerprint(row.Concept)
		if first, dup := inFile[id]; dup {
			if first == fp {
				report.Unchanged++
			} else {
				report.Conflicts = append(report.Conflicts, conflict(row,
					fmt.Sprintf("%q appears in more than one row of this file; give each row its own id in a concept_id column", row.SourceTerm)))
			}
			continue
		}
		inFile[id] = fp
		outcome, reason, err := importRow(ctx, st, row, fp, holders)
		if err != nil {
			return report, fmt.Errorf("row %d: %w", row.Row, err)
		}
		switch outcome {
		case importCreated:
			report.Created++
			for _, t := range row.Concept.Terms {
				holders[termKey(t.Text, t.Locale)] = id
			}
		case importUpdated:
			report.Updated++
		case importUnchanged:
			report.Unchanged++
		case importRefused:
			report.Conflicts = append(report.Conflicts, conflict(row, reason))
		}
	}
	return report, nil
}

type importOutcome int

const (
	importCreated importOutcome = iota
	importUpdated
	importUnchanged
	importRefused
)

// importRow decides one row against the stored concept and writes it. When
// another write lands first it decides again on what is stored then.
func importRow(ctx context.Context, st Store, row fw.ImportRow, fp string, holders map[string]string) (importOutcome, string, error) {
	id := row.Concept.ID
	key := termKey(row.SourceTerm, row.SourceLocale)
	for range rederiveAttempts {
		stored, rev, ok, err := st.GetConceptAt(ctx, id)
		if err != nil {
			return 0, "", err
		}
		if !ok {
			if holder := holders[key]; !row.IDGiven && holder != "" && holder != id {
				return importRefused, fmt.Sprintf("%q is already a term of concept %s; to update that concept, give this row its id in a concept_id column, or give it a new id for a different meaning", row.SourceTerm, holder), nil
			}
			next := withFingerprint(row.Concept, nil, fp)
			if _, err := st.CreateConcept(ctx, next, ""); err != nil {
				if errors.Is(err, ErrConceptExists) {
					continue
				}
				return 0, "", err
			}
			return importCreated, "", nil
		}

		if !row.IDGiven && !holdsTerm(stored, key) {
			return importRefused, fmt.Sprintf("the id %s that %q gives is held by a concept with other terms; give this row its id in a concept_id column", id, row.SourceTerm), nil
		}
		current := importFingerprint(stored)
		if current == fp {
			return importUnchanged, "", nil
		}
		recorded := stored.Properties[ImportFingerprintProperty]
		switch {
		case recorded == "" && !row.IDGiven:
			return importRefused, fmt.Sprintf("concept %s was not written by an import, so it is left as it is; to update it from this file, give this row its id in a concept_id column", id), nil
		case recorded != "" && recorded != current:
			return importRefused, fmt.Sprintf("concept %s was edited after it was last imported, so this row was not applied; review the edit, then import again", id), nil
		}
		next := withFingerprint(row.Concept, stored.Properties, fp)
		next.CreatedAt = stored.CreatedAt
		if _, err := st.UpdateConcept(ctx, next, "", rev); err != nil {
			if errors.Is(err, storage.ErrStale) || errors.Is(err, ErrConceptNotFound) {
				continue
			}
			return 0, "", err
		}
		return importUpdated, "", nil
	}
	return 0, "", fmt.Errorf("concept %s: %w", id, ErrKeptChanging)
}

// withFingerprint is c carrying the properties it is stored with, the file's
// own over them, and the import's fingerprint.
func withFingerprint(c fw.Concept, stored map[string]string, fp string) fw.Concept {
	props := maps.Clone(stored)
	if props == nil {
		props = map[string]string{}
	}
	maps.Copy(props, c.Properties)
	props[ImportFingerprintProperty] = fp
	c.Properties = props
	return c
}

// importFingerprint is a digest of the content an import writes: the
// concept's domain, definition, flags and terms, in a fixed order. What a
// later process adds (forms, properties, the scope a keep sets) is left out,
// so only a change to what the file states counts as an edit.
func importFingerprint(c fw.Concept) string {
	type term struct {
		Text, Locale, Status, Note, PartOfSpeech, Gender string
		Competitor                                       bool
	}
	terms := make([]term, 0, len(c.Terms))
	for _, t := range c.Terms {
		terms = append(terms, term{
			Text: strings.TrimSpace(t.Text), Locale: string(model.NormalizeLocale(t.Locale)),
			Status: string(t.Status), Note: t.Note, PartOfSpeech: t.PartOfSpeech, Gender: t.Gender,
			Competitor: t.CompetitorTerm,
		})
	}
	slices.SortFunc(terms, func(a, b term) int {
		return cmp.Or(cmp.Compare(a.Locale, b.Locale), cmp.Compare(a.Text, b.Text))
	})
	raw, _ := json.Marshal(struct {
		Domain, Definition, Source string
		DoNotTranslate, Advisory   bool
		Terms                      []term
	}{c.Domain, c.Definition, string(cmp.Or(c.Source, fw.TermSourceTerminology)), c.DoNotTranslate, c.Advisory, terms})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:12])
}

func termKey(text string, locale model.LocaleID) string {
	return string(model.NormalizeLocale(locale)) + "\x00" + strings.ToLower(strings.TrimSpace(text))
}

func holdsTerm(c fw.Concept, key string) bool {
	for _, t := range c.Terms {
		if termKey(t.Text, t.Locale) == key {
			return true
		}
	}
	return false
}

func conflict(row fw.ImportRow, reason string) ImportConflict {
	return ImportConflict{Row: row.Row, Term: row.SourceTerm, ConceptID: row.Concept.ID, Reason: reason}
}

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

// ImportIDProperty is the property an import records on each concept it
// writes: the id the server minted for the import (ImportRecord.ID). Only an
// import that a person names as an update of that id changes the concept
// again.
const ImportIDProperty = "import.id"

// ImportSource is the import a file's rows belong to.
type ImportSource struct {
	// ID is the import's id: one the server minted for a new import, or the
	// id of the earlier import the person named this one an update of. A row
	// changes a stored concept only when an import with this id wrote it.
	ID string
}

// ImportRows writes the concepts of a terms file to st, as the import src.
//
// A row without an id of its own is known by its source term (fw.ImportID).
// An id a file gives names a concept within that file, not in the workspace:
// the concept is stored under the id of its source term with the file's id
// beside it, so two files that both say c1 for different terms keep two
// concepts, and two that say c1 for the same term meet on one concept, which
// the second may change only as an update of the first. An id that also
// names a workspace concept is reported, never followed.
//
// A row of a file exported from the workspace carries the revision its
// concept had at the export (fw.JSONStore.Revisions), and is applied as an
// edit at that revision: a concept that changed since is reported.
//
// A row changes any other stored concept only when the import src.ID wrote
// it and nobody edited it since; otherwise it is reported by row and nothing
// is written for it. A row whose content the concept already holds is
// unchanged, so the same file imported twice changes nothing. A row whose
// term belongs to another concept (a homonym), and a term given twice in one
// file with different content, are reported the same way.
func ImportRows(ctx context.Context, st Store, rows []fw.ImportRow, src ImportSource) (ImportReport, error) {
	var report ImportReport
	all, err := st.Concepts(ctx)
	if err != nil {
		return report, fmt.Errorf("read the terms: %w", err)
	}
	holders := map[termIdentity]string{} // term -> the concept holding it
	for _, c := range all {
		for _, t := range c.Terms {
			holders[termKey(t.Text, t.Locale)] = c.ID
		}
	}
	inFile := map[string]string{} // concept id -> fingerprint of its first row
	for _, row := range rows {
		exported := row.IDGiven && row.BaseRevision != ""
		given := row.Concept.ID
		if !exported {
			row.Concept.ID = storedImportID(row)
		}
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
		var (
			outcome importOutcome
			reason  string
		)
		if exported {
			outcome, reason, err = importExportedRow(ctx, st, row, fp)
		} else {
			outcome, reason, err = importRow(ctx, st, row, given, fp, src, holders)
		}
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

// storedImportID is the id a row's concept is stored under: the id of its
// source term (fw.ImportID), with the file's own id beside it when the file
// gives one, so ids from different files never meet unless their terms do.
// An id that already is the source term's id is kept as it is.
func storedImportID(row fw.ImportRow) string {
	derived := fw.ImportID(row.SourceTerm, row.SourceLocale)
	if !row.IDGiven || row.Concept.ID == derived {
		return derived
	}
	return derived + ":" + idPart(row.Concept.ID)
}

// idPart is a file's concept id as it can stand in a stored id: letters and
// digits kept, anything else a dash, so it never holds the ':' that
// separates the parts, with a digest added whenever that changed the id, so
// two ids never collapse into one.
func idPart(id string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(id)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	part := strings.Trim(b.String(), "-")
	if part == id {
		return part
	}
	sum := sha256.Sum256([]byte(id))
	if part == "" {
		return "h-" + hex.EncodeToString(sum[:6])
	}
	return part + "-" + hex.EncodeToString(sum[:3])
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
func importRow(ctx context.Context, st Store, row fw.ImportRow, given, fp string, src ImportSource, holders map[termIdentity]string) (importOutcome, string, error) {
	id := row.Concept.ID
	key := termKey(row.SourceTerm, row.SourceLocale)
	if row.IDGiven && given != id {
		// The file's id names a concept within that file. When it also names
		// a workspace concept, the file may mean that one, or one of its own:
		// say so, and write nothing for the row.
		if _, _, held, err := st.GetConceptAt(ctx, given); err != nil {
			return 0, "", err
		} else if held {
			return importRefused, fmt.Sprintf("this row's id %s also names concept %s in the workspace, which this import does not change; to change that concept, export the workspace's terms and edit the exported file, or give this row another id", given, given), nil
		}
	}
	for range rederiveAttempts {
		stored, rev, ok, err := st.GetConceptAt(ctx, id)
		if err != nil {
			return 0, "", err
		}
		if !ok {
			if holder := holders[key]; !row.IDGiven && holder != "" && holder != id {
				return importRefused, fmt.Sprintf("%q is already a term of concept %s; to change that concept, export the workspace's terms and edit the exported file, or give this row an id of its own in a concept_id column for a different meaning", row.SourceTerm, holder), nil
			}
			next := withImportRecord(row.Concept, nil, fp, src)
			if _, err := st.CreateConcept(ctx, next, ""); err != nil {
				if errors.Is(err, ErrConceptExists) {
					continue
				}
				return 0, "", err
			}
			return importCreated, "", nil
		}

		if !holdsTerm(stored, key) {
			return importRefused, fmt.Sprintf("the id %s that %q gives is held by a concept with other terms; give this row an id of its own in a concept_id column", id, row.SourceTerm), nil
		}
		current := importFingerprint(stored)
		if current == fp {
			return importUnchanged, "", nil
		}
		switch recorded := stored.Properties[ImportIDProperty]; {
		case recorded == "":
			return importRefused, fmt.Sprintf("concept %s was not written by an import, so it is left as it is; to change it, export the workspace's terms and edit the exported file", id), nil
		case recorded != src.ID:
			return importRefused, fmt.Sprintf("concept %s belongs to %s, so this row was not applied; to change it, import this file as an update of that import", id, describeImport(ctx, st, recorded)), nil
		case stored.Properties[ImportFingerprintProperty] != current:
			return importRefused, fmt.Sprintf("concept %s was edited after it was last imported, so this row was not applied; review the edit, then import again", id), nil
		}
		next := withImportRecord(row.Concept, stored.Properties, fp, src)
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

// describeImport names an import for a person: its file, who created it and
// when, read from its record.
func describeImport(ctx context.Context, st Store, id string) string {
	r, ok, err := st.GetImport(ctx, id)
	if err != nil || !ok {
		return "the import " + id
	}
	return fmt.Sprintf("the import of %s by %s on %s (%s)", cmp.Or(r.Name, "an unnamed file"), cmp.Or(r.CreatedBy, "an unknown person"), r.CreatedAt.UTC().Format("2006-01-02 15:04 UTC"), r.ID)
}

// importExportedRow applies a row that carries the revision its concept had
// at an export to the concept it names, at that revision, through the same
// conditional update every writer uses: the edit is the person's, made on
// the export. A concept that changed since, or a revision that was never the
// concept's, is reported, and nothing is written for the row.
func importExportedRow(ctx context.Context, st Store, row fw.ImportRow, fp string) (importOutcome, string, error) {
	id := row.Concept.ID
	stored, _, ok, err := st.GetConceptAt(ctx, id)
	if err != nil {
		return 0, "", err
	}
	if !ok {
		return importRefused, fmt.Sprintf("concept %s was deleted after this file was exported, so this row was not applied", id), nil
	}
	if importFingerprint(stored) == fp {
		return importUnchanged, "", nil
	}
	next := row.Concept
	next.Properties = maps.Clone(stored.Properties)
	maps.Copy(next.Properties, row.Concept.Properties)
	next.CreatedAt = stored.CreatedAt
	if _, err := st.UpdateConcept(ctx, next, "", row.BaseRevision); err != nil {
		if errors.Is(err, storage.ErrStale) {
			return importRefused, fmt.Sprintf("concept %s changed in Bowrain after this file was exported, so this row was not applied; export again and redo the edit", id), nil
		}
		if errors.Is(err, ErrConceptNotFound) {
			return importRefused, fmt.Sprintf("concept %s was deleted after this file was exported, so this row was not applied", id), nil
		}
		return 0, "", err
	}
	return importUpdated, "", nil
}

// withImportRecord is c carrying the properties it is stored with, the
// file's own over them, and the import's record: its fingerprint and its id.
func withImportRecord(c fw.Concept, stored map[string]string, fp string, src ImportSource) fw.Concept {
	props := maps.Clone(stored)
	if props == nil {
		props = map[string]string{}
	}
	maps.Copy(props, c.Properties)
	props[ImportFingerprintProperty] = fp
	props[ImportIDProperty] = src.ID
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

// termIdentity is a term as the import compares terms: its locale and its
// text, case and surrounding space aside.
type termIdentity struct {
	locale model.LocaleID
	text   string
}

func termKey(text string, locale model.LocaleID) termIdentity {
	return termIdentity{model.NormalizeLocale(locale), strings.ToLower(strings.TrimSpace(text))}
}

func holdsTerm(c fw.Concept, key termIdentity) bool {
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

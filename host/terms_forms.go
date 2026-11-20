package host

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/expand"
	"github.com/neokapi/neokapi/terms/ktb"
)

// Surface forms for the terms store. `kapi terms expand` proposes the forms each
// term takes and writes them onto the terms; `kapi terms validate` reports the
// target terms that still lack them. Both read the same concepts.

// TermsFormsTarget is a set of concepts read for expansion or validation, and
// the place expanded forms are written back to.
type TermsFormsTarget struct {
	// Label names the target for the user: a bundle path or a store path.
	Label   string
	bundle  string
	file    *ktb.File
	store   terms.Terminology
	release func()
}

// OpenTermsFormsTarget resolves the concepts `kapi terms expand` and `kapi
// terms validate` work on. In order: a bundle named on the command line; a
// .terms.json named by --file; otherwise the store every `kapi terms`
// subcommand uses.
func (a *App) OpenTermsFormsTarget(cmd Command, bundle string) (*TermsFormsTarget, error) {
	file, _ := cmd.Flags().GetString("file")
	if bundle == "" && ktb.IsBundlePath(file) {
		bundle = file
	}
	if bundle != "" {
		if !ktb.IsBundlePath(bundle) {
			return nil, fmt.Errorf("%s is not a terms bundle: expected a %s file", bundle, ktb.Ext)
		}
		return openBundleTarget(bundle)
	}

	tb, label, release, err := a.OpenTermsSQLite(cmd)
	if err != nil {
		return nil, err
	}
	return &TermsFormsTarget{Label: label, store: tb, release: release}, nil
}

func openBundleTarget(path string) (*TermsFormsTarget, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("open terms bundle: %w", err)
	}
	file, err := loadKTBFile(path)
	if err != nil {
		return nil, err
	}
	return &TermsFormsTarget{Label: path, bundle: path, file: file}, nil
}

// loadKTBFile reads the terms bundle a person named on the command line. The
// whole document is returned, concepts and relations, so a write of it
// round-trips everything the file carried rather than only the part that moved.
func loadKTBFile(path string) (*ktb.File, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ktb.FromConcepts(nil), nil
		}
		return nil, fmt.Errorf("open terms bundle: %w", err)
	}
	defer f.Close()
	file, err := ktb.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("parse terms bundle: %w", err)
	}
	return file, nil
}

// writeKTB serializes a bundle to a deterministic document at the path a person
// named, and reports whether the bytes on disk moved. A serialization identical
// to what is already there leaves the file alone, so a run with nothing to say
// puts nothing in a diff.
func writeKTB(path string, file *ktb.File) (bool, error) {
	data, err := ktb.Marshal(file)
	if err != nil {
		return false, fmt.Errorf("marshal terms bundle: %w", err)
	}
	if existing, rerr := os.ReadFile(path); rerr == nil && bytes.Equal(existing, data) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("create bundle dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return false, fmt.Errorf("write terms bundle: %w", err)
	}
	return true, nil
}

// Concepts returns the target's concepts. For a bundle they are the document's
// own, so changes to them are what Save writes.
func (t *TermsFormsTarget) Concepts(ctx context.Context) ([]terms.Concept, error) {
	if t.file != nil {
		return t.file.Concepts, nil
	}
	concepts, err := t.store.Concepts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list concepts: %w", err)
	}
	return concepts, nil
}

// Save writes back the concepts whose ids are in changed and reports whether
// anything was written. A bundle is rewritten whole, and only when its
// serialization moved; a store takes each changed concept.
func (t *TermsFormsTarget) Save(ctx context.Context, concepts []terms.Concept, changed map[string]bool) (bool, error) {
	if len(changed) == 0 {
		return false, nil
	}
	if t.file != nil {
		t.file.Concepts = concepts
		return writeKTB(t.bundle, t.file)
	}
	for _, c := range concepts {
		if !changed[c.ID] {
			continue
		}
		if err := t.store.AddConcept(ctx, c); err != nil {
			return false, fmt.Errorf("write concept %s: %w", c.ID, err)
		}
	}
	return true, nil
}

// Close releases the store, when the target holds one.
func (t *TermsFormsTarget) Close() {
	if t.release != nil {
		t.release()
	}
}

// The proposal and the write live in terms/expand, which the platform's
// term-forms job runs too. These names keep the host API the CLI calls.
type (
	// TermsExpandOptions selects the terms `kapi terms expand` asks about.
	TermsExpandOptions = expand.Options
	// FormsProposal is what expansion proposes for one term.
	FormsProposal = expand.Proposal
)

// ExpandableTerms counts the terms ProposeTermForms would ask about.
func ExpandableTerms(concepts []terms.Concept, opts TermsExpandOptions) int {
	return expand.Count(concepts, opts)
}

// ProposeTermForms asks p for the surface forms of each selected term
// (expand.Propose).
func ProposeTermForms(ctx context.Context, p aiprovider.LLMProvider, concepts []terms.Concept, opts TermsExpandOptions) ([]FormsProposal, error) {
	return expand.Propose(ctx, p, concepts, opts)
}

// ApplyFormsProposals writes each proposal's forms onto its term
// (expand.Apply).
func ApplyFormsProposals(concepts []terms.Concept, proposals []FormsProposal, overwrite bool) (map[string]bool, int) {
	return expand.Apply(concepts, proposals, overwrite)
}

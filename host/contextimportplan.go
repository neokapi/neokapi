package host

import (
	"context"
	"errors"
	"fmt"
	"io"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/rootpath"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/memory/kmb"
	"github.com/neokapi/neokapi/terms/ktb"
)

// ContextImportPlan is what `kapi store import` would read, listed before it
// reads anything: the person sees what enters the store and confirms it.
type ContextImportPlan struct {
	// Dir is the layout the import reads, as the caller named it.
	Dir string `json:"dir"`
	// Sources are the files the import would read, in the order it reads them.
	Sources []PlannedContextSource `json:"sources,omitempty"`
	// Decisions counts the approvals the decision record carries.
	Decisions int `json:"decisions,omitempty"`
	// Unchanged counts the sources this checkout already read at these bytes.
	Unchanged int `json:"unchanged,omitempty"`

	// digests holds the bytes each listed source had when it was counted, by
	// its relative path, so the import reads only what was shown.
	digests map[string]string
}

// PlannedContextSource is one file an import would read and what it carries.
type PlannedContextSource struct {
	Path string `json:"path"`
	// Kind is terms, memory or voice.
	Kind string `json:"kind"`
	// Concepts counts the concepts a terms file carries.
	Concepts int `json:"concepts,omitempty"`
	// Entries counts the content-memory entries a memory bundle carries.
	Entries int `json:"entries,omitempty"`
	// WordRules counts the word rules a voice file carries, which the import
	// moves into terms.
	WordRules int `json:"word_rules,omitempty"`
}

// Reads reports whether the import would read anything.
func (p ContextImportPlan) Reads() bool { return len(p.Sources) > 0 || p.Decisions > 0 }

// PlanProjectContextImport lists what ImportProjectContext would read for req,
// without writing to the store.
func (a *App) PlanProjectContextImport(ctx context.Context, projectPath string, req ContextImportRequest) (ContextImportPlan, error) {
	plan := ContextImportPlan{digests: map[string]string{}}
	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return plan, err
	}
	from, _, err := resolveContextLayout(layout, req.Dir)
	if err != nil {
		return plan, err
	}
	plan.Dir = reportedPath(layout.Root, from.StateDir)
	db, err := a.ProjectDB(ctx, layout.Root)
	if err != nil {
		return plan, err
	}
	sources, err := committedContextSources(from)
	if err != nil {
		return plan, err
	}
	checkout := normalizedCheckout(layout.Root)
	identity, _ := recipeIdentity(layout.RecipePath)
	projectKey := workspace.ProjectKey(identity)
	stamps, err := a.loadImportStamps(ctx, projectKey, checkout)
	if err != nil {
		return plan, err
	}
	for _, src := range sources {
		if !storeHolds(db, src.kind) {
			plan.Unchanged++
			continue
		}
		digest, err := fileDigest(from.Root, src.path)
		if err != nil {
			return plan, err
		}
		if !req.Force && stamps[importStampKey(projectKey, checkout, src.rel)] == digest {
			plan.Unchanged++
			continue
		}
		ps, err := countContextSource(from.Root, src)
		if err != nil {
			return plan, err
		}
		plan.Sources = append(plan.Sources, ps)
		plan.digests[src.rel] = digest
	}
	recordDir := from.Export().UnitStateDir()
	if committedRecordExists(from.Root, recordDir) {
		recordRel := relSlash(from.Root, recordDir)
		digest, err := state.CommittedDigest(recordDir)
		if err != nil {
			return plan, err
		}
		if !req.Force && stamps[importStampKey(projectKey, checkout, recordRel)] == digest {
			plan.Unchanged++
		} else {
			units, err := state.ReadCommitted(recordDir)
			if err != nil {
				return plan, err
			}
			for _, u := range units {
				if u.Decides() {
					plan.Decisions++
				}
			}
			plan.digests[recordRel] = digest
		}
	}
	return plan, nil
}

// countContextSource decodes one source and counts what it carries.
func countContextSource(root string, src contextSource) (PlannedContextSource, error) {
	ps := PlannedContextSource{Path: src.rel}
	switch src.kind {
	case sourceKindTerms:
		ps.Kind = "terms"
		f, err := rootpath.OpenRegular(root, src.path)
		if err != nil {
			return ps, fmt.Errorf("open terms source: %w", err)
		}
		defer f.Close()
		file, err := ktb.Decode(f)
		if err != nil {
			return ps, fmt.Errorf("read terms %s: %w", src.rel, err)
		}
		ps.Concepts = len(file.Concepts)
	case sourceKindMemory:
		ps.Kind = "memory"
		f, err := rootpath.OpenRegular(root, src.path)
		if err != nil {
			return ps, fmt.Errorf("open %s: %w", src.rel, err)
		}
		defer f.Close()
		file, err := kmb.Decode(f)
		if err != nil {
			return ps, fmt.Errorf("read content memory %s: %w", src.rel, err)
		}
		ps.Entries = len(file.ModelEntries())
	case sourceKindVoice:
		ps.Kind = "voice"
		data, err := rootpath.ReadFile(root, src.path)
		if err != nil {
			return ps, fmt.Errorf("open voice profile: %w", err)
		}
		file, err := coreprofile.ParseVoiceFile(data)
		if err != nil {
			return ps, fmt.Errorf("read voice profile %s: %w", src.rel, err)
		}
		ps.WordRules = len(file.Terms)
	default:
		return ps, fmt.Errorf("read context source %s: unknown kind", src.rel)
	}
	return ps, nil
}

// expectShown refuses a source whose bytes differ from the ones a plan showed,
// or one the plan did not list. A nil plan expects nothing.
func (p *ContextImportPlan) expectShown(rel, digest string) error {
	if p == nil {
		return nil
	}
	if shown, ok := p.digests[rel]; !ok || shown != digest {
		return fmt.Errorf("%s changed after it was listed; run the import again to see what it holds now", rel)
	}
	return nil
}

// FormatText lists the plan for the person about to confirm it.
func (p ContextImportPlan) FormatText(w io.Writer) error {
	if _, err := fmt.Fprintf(w, "kapi would read these into the project store from %s:\n", p.Dir); err != nil {
		return err
	}
	for _, s := range p.Sources {
		var what string
		switch s.Kind {
		case "terms":
			what = pluralUnit(s.Concepts, "term concept", "term concepts")
		case "memory":
			what = pluralUnit(s.Entries, "content-memory entry", "content-memory entries")
		case "voice":
			what = "1 voice profile"
			if s.WordRules > 0 {
				what += ", " + pluralUnit(s.WordRules, "word rule", "word rules")
			}
		}
		if _, err := fmt.Fprintf(w, "  %s  %s\n", s.Path, what); err != nil {
			return err
		}
	}
	if p.Decisions > 0 {
		if _, err := fmt.Fprintf(w, "  decision record  %s\n", pluralUnit(p.Decisions, "approval", "approvals")); err != nil {
			return err
		}
	}
	if p.Unchanged > 0 {
		if _, err := fmt.Fprintf(w, "  %s already read at these bytes\n", pluralUnit(p.Unchanged, "source", "sources")); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(w, "What you read is in force for every checkout of this project on this machine. Read only context you trust.")
	return err
}

// ConfirmContextImport lists a plan and asks the person at a terminal, or
// takes --yes. With no terminal and no --yes it refuses.
func (a *App) ConfirmContextImport(cmd Command, plan ContextImportPlan) (bool, error) {
	if err := plan.FormatText(cmd.ErrOrStderr()); err != nil {
		return false, err
	}
	return a.confirmAtTerminal(cmd, "Read these into the project store? [y/N] ",
		"no terminal is attached, so there is nobody to confirm; pass --yes to read these files")
}

// confirmAtTerminal asks question at a terminal, or takes --yes. With no
// terminal and no --yes it returns refusal as the error.
func (a *App) confirmAtTerminal(cmd Command, question, refusal string) (bool, error) {
	if a.AssumeYes {
		return true, nil
	}
	isTTY := a.isTTY
	if isTTY == nil {
		isTTY = defaultIsStdinTTY
	}
	if !isTTY() {
		return false, errors.New(refusal)
	}
	return ConfirmDefaultNo(cmd.InOrStdin(), cmd.ErrOrStderr(), question)
}

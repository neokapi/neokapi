package host

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/neokapi/neokapi/core/blockstore/exporter"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/kpz"
)

// `kapi info` reports what `kapi pack` would capture, and what a `.kpz` holds.
//
// One verb answers for both subjects — a project and a `.kpz` — so "what would
// packing this project actually include?" is answerable before doing it. The
// answer is organised the same way in each case: the archive's parts, one line
// each, with a size.
//
// That symmetry is the point of the trio. `pack` writes exactly the parts `info`
// lists; `unpack` restores exactly those parts. When `info` on a project and
// `info` on its snapshot agree, the round trip is complete — which is a property
// a user can check, not just a claim in a doc.

// ProjectArchivePart is one component of a project archive: what it is, whether
// it is present, and how big.
type ProjectArchivePart struct {
	// Name is the part's stable identity ("recipe", "blocks", "memory", …).
	Name string `json:"name"`
	// Path is where the part lives relative to the project root (for a project)
	// or inside the archive (for a .kpz).
	Path string `json:"path,omitempty"`
	// Present reports whether the part exists to be packed.
	Present bool `json:"present"`
	// Count is how many items the part holds (sources, blocks, overlays, memory
	// entries, terms), when countable. Zero means "not counted" or "none".
	Count int `json:"count,omitempty"`
	// Bytes is the part's size on disk, when present and measurable.
	Bytes int64 `json:"bytes,omitempty"`
	// Note explains a part's role, or why it is absent.
	Note string `json:"note,omitempty"`
}

// ProjectInfoOutput is the structured result of `kapi info` on a project: what a
// `kapi pack` from here would capture.
type ProjectInfoOutput struct {
	Project string `json:"project"`
	Recipe  string `json:"recipe"`
	// SourceLang / TargetLangs come from the recipe.
	SourceLang  string   `json:"sourceLang,omitempty"`
	TargetLangs []string `json:"targetLangs,omitempty"`
	// Content is the number of files the recipe tracks.
	Content int `json:"content"`
	// Parts is what a snapshot would contain, in archive order.
	Parts []ProjectArchivePart `json:"parts"`
	// Excluded names what pack deliberately leaves out, so the omission is
	// visible rather than surprising.
	Excluded []string `json:"excluded,omitempty"`
}

// FormatText renders the project's packable state.
func (o ProjectInfoOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "project    %s\n", o.Project)
	fmt.Fprintf(w, "recipe     %s\n", o.Recipe)
	if o.SourceLang != "" {
		fmt.Fprintf(w, "languages  %s → %v\n", o.SourceLang, o.TargetLangs)
	}
	fmt.Fprintf(w, "content    %d file(s) tracked\n\n", o.Content)

	output.Title(w, "A snapshot (kapi pack) would capture:")
	t := output.NewTable(w).Accent(0).Headers("PART", "PRESENT", "ITEMS", "SIZE", "WHAT IT IS")
	for _, p := range o.Parts {
		t.Row(p.Name, presentLabel(p.Present), countLabel(p.Count), byteLabel(p.Bytes), p.Note)
	}
	t.Render()

	if len(o.Excluded) > 0 {
		fmt.Fprintln(w)
		output.Note(w, "Excluded (regenerable or secret): %v", o.Excluded)
	}
	return nil
}

func presentLabel(ok bool) string {
	if ok {
		return "yes"
	}
	return "—"
}

func byteLabel(n int64) string {
	if n <= 0 {
		return "—"
	}
	return HumanBytes(n)
}

func countLabel(n int) string {
	if n <= 0 {
		return "—"
	}
	return strconv.Itoa(n)
}

// archivePartNotes documents each part's role once, so `kapi info` on a project
// and on an archive describe the same thing in the same words.
var archivePartNotes = map[string]string{
	"recipe":   "the project's kapi.yaml, with server/hooks/automations stripped inert",
	"sources":  "source identity + per-file skeletons (raw bytes only with --with-source)",
	"blocks":   "extracted content blocks (.kbf.json)",
	"overlays": "committed target overlays, one per locale",
	"memory":   "the authoritative content memory (.memory.json)",
	"terms":    "the authoritative terms (.terms.json)",
	"history":  "advisory provenance chain (--log)",
}

// archivePartAbsentNotes say what to do about a part that is not there, since
// "—" on its own tells a user nothing about whether it matters.
var archivePartAbsentNotes = map[string]string{
	"recipe":   "no recipe in this archive",
	"sources":  "no extraction manifest: `kapi extract` records source identity + skeletons",
	"blocks":   "nothing extracted yet: run `kapi extract` or `kapi up`",
	"overlays": "no committed targets yet",
	"memory":   "no content memory yet: it fills as work is committed",
	"terms":    "no terms yet: `kapi terms import` seeds one",
	"history":  "no provenance chain: `kapi pack --log` stamps one",
}

// projectArchiveExcluded names what a snapshot deliberately omits.
var projectArchiveExcluded = []string{
	".kapi/work/cache/ (regenerable: parse cache, extractions, collections)",
	"credentials (OS keychain, never in an archive)",
	"server sync tokens",
}

// RunProjectInfo reports what `kapi pack` would capture from the project in
// scope — the project-subject half of `kapi info`.
func (a *App) RunProjectInfo(cmd Command) error {
	a.InitRegistries()
	projectPath, err := RequireProjectPath(cmd)
	if err != nil {
		return err
	}
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return err
	}

	pctx := project.NewProjectContext(proj, projectPath)
	resolved, rerr := pctx.ResolveContent(a.FormatReg)
	if rerr != nil {
		return fmt.Errorf("resolve project content: %w", rerr)
	}

	out := ProjectInfoOutput{
		Project:    proj.Name,
		Recipe:     relativeToCwd(projectPath),
		SourceLang: string(proj.Defaults.SourceLanguage),
		Content:    len(resolved),
		Excluded:   projectArchiveExcluded,
	}
	for _, l := range proj.Defaults.TargetLanguages {
		out.TargetLangs = append(out.TargetLangs, string(l))
	}

	// Every part is measured the way `pack` collects it, not by a proxy that
	// happens to correlate. Reporting "sources: yes" because the recipe *tracks*
	// files, for instance, promised bytes a snapshot would not carry: pack takes
	// sources from the extraction manifests, so nothing extracted means nothing
	// packed however many files the recipe lists.
	ctx := cmd.Context()

	out.Parts = append(out.Parts,
		part("recipe", relativeToCwd(projectPath), 1, fileSize(projectPath)),
		part("sources", "", countPackableSources(layout), 0),
	)

	// Three of the parts now live in one file, so each names `.kapi/work/store.db`
	// and only the whole-store size is reportable. Per-part BYTES would be a
	// fiction — there is no per-schema size in a SQLite file — so the size is
	// carried once, on the first part, and the counts do the distinguishing.
	// Presence is a count, which is what it always meant to a reader of this
	// report: a store with no concepts has no terms to pack.
	storePath := relativeToCwd(layout.StorePath())
	storeBytes := fileSize(layout.StorePath())
	nBlocks, nOverlays, nMemory, nTerms := a.countProjectStore(ctx, layout)
	out.Parts = append(out.Parts,
		part("blocks", storePath, nBlocks, storeBytes),
		part("overlays", "", nOverlays, 0),
		part("memory", storePath, nMemory, 0),
		part("terms", storePath, nTerms, 0),
	)

	return output.Print(cmd, out)
}

// part builds one archive part from its measured item count, choosing the
// role note or the actionable absence note.
func part(name, path string, count int, bytes int64) ProjectArchivePart {
	p := ProjectArchivePart{
		Name: name, Path: path, Present: count > 0, Count: count,
		Note: archivePartNotes[name],
	}
	if count == 0 {
		p.Bytes = 0
		if note, ok := archivePartAbsentNotes[name]; ok {
			p.Note = note
		}
		return p
	}
	p.Bytes = bytes
	return p
}

// countPackableSources counts the distinct sources `pack` would carry — the
// extraction manifests, deduped by source path, exactly as collectProjectSources
// walks them.
func countPackableSources(layout project.Layout) int {
	manifests, err := project.ListExtractionManifests(layout)
	if err != nil {
		return 0
	}
	seen := map[string]bool{}
	for _, m := range manifests {
		for _, pair := range m.Pairs {
			for _, ef := range pair.Files {
				if ef.Source != "" {
					seen[ef.Source] = true
				}
			}
		}
	}
	return len(seen)
}

// countProjectStore reports how much of each part a snapshot would carry. The
// block figures come from the very export `pack` runs, so the two cannot report
// different numbers for the same store.
//
// It does not open a store that is not there: `info` is a diagnostic and
// creating the project's database to report on it would be a read command with
// a side effect. An unreadable store counts as nothing rather than failing, so
// the rest of the report still answers.
func (a *App) countProjectStore(ctx context.Context, layout project.Layout) (blocks, overlays, entries, concepts int) {
	if !fileExists(layout.StorePath()) {
		return 0, 0, 0, 0
	}
	db, err := a.ProjectDB(ctx, layout.Root)
	if err != nil {
		return 0, 0, 0, 0
	}
	if store := db.BlocksAutocommit(); store != nil {
		if snap, eerr := exporter.Export(ctx, store); eerr == nil {
			blocks, overlays = len(snap.Blocks), len(snap.Overlays)
		}
	}
	if tm := db.Memory(); tm != nil {
		if es, eerr := tm.Entries(ctx); eerr == nil {
			entries = len(es)
		}
	}
	if tb := db.Terms(); tb != nil {
		if cs, cerr := tb.Concepts(ctx); cerr == nil {
			concepts = len(cs)
		}
	}
	return blocks, overlays, entries, concepts
}

// ArchiveInfoOutput is the structured result of `kapi info <file.kpz>` on a
// project snapshot: what the archive actually holds.
//
// It carries the same Parts as ProjectInfoOutput on purpose. `pack` writes the
// parts `info` lists and `unpack` restores them, so a user can check the round
// trip by comparing the two reports — which is only possible if they are the
// same report.
type ArchiveInfoOutput struct {
	Archive string `json:"archive"`
	// Project / SourceLang / TargetLangs come from the recipe the archive carries.
	Project     string   `json:"project,omitempty"`
	SourceLang  string   `json:"sourceLang,omitempty"`
	TargetLangs []string `json:"targetLangs,omitempty"`
	// Bytes is the archive's own size on disk.
	Bytes int64 `json:"bytes,omitempty"`
	// Parts is what the archive holds, in the same order and with the same names
	// as a project report.
	Parts []ProjectArchivePart `json:"parts"`
}

// FormatText renders the archive's contents.
func (o ArchiveInfoOutput) FormatText(w io.Writer) error {
	fmt.Fprintf(w, "archive    %s (%s)\n", o.Archive, HumanBytes(o.Bytes))
	if o.Project != "" {
		fmt.Fprintf(w, "project    %s\n", o.Project)
	}
	if o.SourceLang != "" {
		fmt.Fprintf(w, "languages  %s → %v\n", o.SourceLang, o.TargetLangs)
	}
	fmt.Fprintln(w)

	output.Title(w, "This snapshot holds:")
	t := output.NewTable(w).Accent(0).Headers("PART", "PRESENT", "ITEMS", "WHAT IT IS")
	for _, p := range o.Parts {
		t.Row(p.Name, presentLabel(p.Present), countLabel(p.Count), p.Note)
	}
	t.Render()

	fmt.Fprintln(w)
	output.Note(w, "Restore it into a project with: kapi unpack %s", filepath.Base(o.Archive))
	return nil
}

// RunArchiveInfo reports what a project-snapshot `.kpz` holds — the archive
// half of `kapi info`. pkg may be nil, in which case the archive is loaded.
func (a *App) RunArchiveInfo(cmd Command, path string, pkg *kpz.Package) error {
	if pkg == nil {
		loaded, err := LoadWorkspace(path)
		if err != nil {
			return err
		}
		pkg = loaded
	}

	out := ArchiveInfoOutput{Archive: path, Bytes: fileSize(path)}
	if pkg.Recipe != nil {
		out.Project = pkg.Recipe.Name
		out.SourceLang = string(pkg.Recipe.Defaults.SourceLanguage)
		for _, l := range pkg.Recipe.Defaults.TargetLanguages {
			out.TargetLangs = append(out.TargetLangs, string(l))
		}
	}

	recipeCount := 0
	if pkg.Recipe != nil {
		recipeCount = 1
	}
	blocks := 0
	for _, d := range pkg.Blocks {
		if d.File == nil {
			continue
		}
		for _, doc := range d.File.Documents {
			blocks += len(doc.Blocks)
		}
	}
	memory := 0
	if pkg.Memory != nil {
		memory = len(pkg.Memory.ModelEntries())
	}
	terms := 0
	if pkg.Terms != nil {
		terms = len(pkg.Terms.Concepts)
	}

	out.Parts = []ProjectArchivePart{
		part("recipe", "", recipeCount, 0),
		part("sources", "", len(pkg.Sources), 0),
		part("blocks", "", blocks, 0),
		part("overlays", "", len(pkg.Overlays), 0),
		part("memory", "", memory, 0),
		part("terms", "", terms, 0),
		part("history", "", len(pkg.History), 0),
	}
	return output.Print(cmd, out)
}

// fileSize returns a file's size, or 0 when it cannot be measured.
func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// ArchivePartNote exposes a part's documented role, so a surface outside this
// package describes the archive in the same words.
func ArchivePartNote(name string) string { return archivePartNotes[name] }

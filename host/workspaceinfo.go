package host

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/neokapi/neokapi/core/blockstore/exporter"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/kpz"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)

// `kapi info` reports what `kapi pack` would capture, and what a `.kpz` holds.
//
// The two used to be different commands' worth of knowledge: `pack` worked on a
// project *or* a `.kpz`, while `info` accepted only a `.kpz` — so there was no
// way to ask "what would packing this project actually include?" before doing
// it. One verb now answers for both subjects, and the answer is organised the
// same way in each case: the archive's parts, one line each, with a size.
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
	"blocks":   "extracted content blocks (.kbf)",
	"overlays": "committed target overlays, one per locale",
	"memory":   "the authoritative content memory (.kmb)",
	"terms":    "the authoritative terms (.ktb)",
	"history":  "advisory provenance chain (--log)",
}

// archivePartAbsentNotes say what to do about a part that is not there, since
// "—" on its own tells a user nothing about whether it matters.
var archivePartAbsentNotes = map[string]string{
	"recipe":   "no recipe in this archive",
	"sources":  "no extraction manifest — `kapi extract` records source identity + skeletons",
	"blocks":   "no block store yet — run `kapi extract` or `kapi up`",
	"overlays": "no committed targets yet",
	"memory":   "no content memory yet — it fills as work is committed",
	"terms":    "no terms yet — `kapi terms import` seeds one",
	"history":  "no provenance chain — `kapi pack --log` stamps one",
}

// projectArchiveExcluded names what a snapshot deliberately omits.
var projectArchiveExcluded = []string{
	".kapi/cache/ (regenerable: block store, extractions, collections)",
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

	blocks := layout.BlockStorePath()
	nBlocks, nOverlays := countBlockStore(ctx, blocks)
	out.Parts = append(out.Parts,
		part("blocks", relativeToCwd(blocks), nBlocks, fileSize(blocks)),
		part("overlays", "", nOverlays, 0),
	)

	tmPath := filepath.Join(layout.StateDir, "tm.db")
	out.Parts = append(out.Parts,
		part("memory", relativeToCwd(tmPath), countTMEntries(ctx, tmPath), fileSize(tmPath)))

	tbPath := filepath.Join(layout.StateDir, "terms.db")
	out.Parts = append(out.Parts,
		part("terms", relativeToCwd(tbPath), countTermConcepts(ctx, tbPath), fileSize(tbPath)))

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

// countBlockStore reports how many blocks and overlays a snapshot would carry.
// It runs the very export `pack` runs, so the two cannot report different
// numbers for the same store. A missing or unreadable store counts as nothing
// rather than failing the report — `info` is a diagnostic and must still answer
// the rest.
func countBlockStore(ctx context.Context, path string) (blocks, overlays int) {
	if !fileExists(path) {
		return 0, 0
	}
	store, err := sqlitestore.New(path)
	if err != nil {
		return 0, 0
	}
	snap, err := exporter.Export(ctx, store)
	_ = store.Close()
	if err != nil {
		return 0, 0
	}
	return len(snap.Blocks), len(snap.Overlays)
}

// countTMEntries reports how many content-memory entries a snapshot would carry.
func countTMEntries(ctx context.Context, path string) int {
	if !fileExists(path) {
		return 0
	}
	tm, err := memory.NewSQLiteStore(path)
	if err != nil {
		return 0
	}
	defer func() { _ = tm.Close() }()
	entries, err := tm.Entries(ctx)
	if err != nil {
		return 0
	}
	return len(entries)
}

// countTermConcepts reports how many term concepts a snapshot would carry.
func countTermConcepts(ctx context.Context, path string) int {
	if !fileExists(path) {
		return 0
	}
	tb, err := terms.NewSQLiteStore(path)
	if err != nil {
		return 0
	}
	defer func() { _ = tb.Close() }()
	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return 0
	}
	return len(concepts)
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

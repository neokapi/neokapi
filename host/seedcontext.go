package host

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/memory/kmb"
)

// Committed context sources — the terms bundle and the content-memory bundles
// under `.kapi/` — are the truth; the project store is their projection
// (AD-009/AD-010 custody). A read path that never compiles them reads a store
// that holds whatever the last write happened to leave there: on a fresh clone,
// nothing at all, and after a `git pull` of an edited bundle, the pre-pull
// state. Seeding closes that gap by compiling the sources on the way into the
// convergence loop, so the store is the union of what git carries and what a
// venue pull brought.
//
// The compile is keyed by content digest, not by timestamp: bundles are
// generated artifacts whose mtime says nothing about whether their content
// moved, and a re-import of an unchanged bundle is pure cost.

// MetaContextSourceDigests is the store-metadata key holding the digest each
// committed context source carried when it was last compiled into the store —
// a JSON object of project-relative slash path to digest.
const MetaContextSourceDigests = "context.sourceDigests"

// SeedContextResult reports what a seeding pass compiled. Skipped counts the
// sources whose digest was unchanged, so a caller can tell "nothing to do" from
// "no sources at all".
type SeedContextResult struct {
	// TermsFiles and Concepts count the terms bundles compiled and the concepts
	// they carried.
	TermsFiles int `json:"termsFiles,omitempty"`
	Concepts   int `json:"concepts,omitempty"`
	// MemoryFiles and Entries count the content-memory bundles compiled and the
	// entries they carried.
	MemoryFiles int `json:"memoryFiles,omitempty"`
	Entries     int `json:"entries,omitempty"`
	// Skipped counts sources already in the store at their current digest.
	Skipped int `json:"skipped,omitempty"`
	// Record reports the committed translations absorbed after the bundles —
	// the other half of the rebuild (AD-009).
	Record RecordAbsorbResult `json:"record,omitzero"`
}

// Compiled reports whether the pass has anything to say: what it wrote into the
// store, or a pairing it declined because the record's own basis contradicts it.
func (r SeedContextResult) Compiled() bool {
	return r.TermsFiles > 0 || r.MemoryFiles > 0 || r.Record.Absorbed() || r.Record.Superseded > 0
}

// formatSeedLine renders a seeding pass as the one line a run prints before the
// plan header — what git carried into the store this time.
func formatSeedLine(r SeedContextResult) string {
	var head string
	switch {
	case r.TermsFiles > 0 && r.MemoryFiles > 0:
		head = fmt.Sprintf("seeded: %d concept(s) and %d content-memory entry(ies) from %d committed source(s)",
			r.Concepts, r.Entries, r.TermsFiles+r.MemoryFiles)
	case r.TermsFiles > 0:
		head = fmt.Sprintf("seeded: %d concept(s) from the committed terms source", r.Concepts)
	case r.MemoryFiles > 0:
		head = fmt.Sprintf("seeded: %d content-memory entry(ies) from %d committed bundle(s)", r.Entries, r.MemoryFiles)
	}
	rec := formatRecordLine(r.Record)
	switch {
	case head == "":
		return rec
	case rec == "":
		return head
	}
	return head + "\n" + rec
}

// formatRecordLine renders what the committed translations taught the store, or
// "" when they taught it nothing.
func formatRecordLine(r RecordAbsorbResult) string {
	// A pass that wrote nothing but declined a pairing the record contradicts
	// still has something to say: that decline is why the unit beside it is
	// about to be re-drafted rather than confirmed.
	if !r.Absorbed() && r.Superseded == 0 {
		return ""
	}
	line := fmt.Sprintf("absorbed: %d pair(s) from %d committed target document(s) — %d learned, %d reconciled",
		r.Pairs, r.Documents, r.Learned, r.Reconciled)
	if r.Contested > 0 {
		line += fmt.Sprintf("; %d source string(s) the record answers more than one way", r.Contested)
	}
	if r.Refused > 0 {
		line += fmt.Sprintf("; %d pair(s) refused for dropped inline codes", r.Refused)
	}
	if r.Superseded > 0 {
		line += fmt.Sprintf("; %d pair(s) whose source was rewritten since the translation was decided", r.Superseded)
	}
	return line
}

// contextSource is one committed context bundle a seeding pass considers.
type contextSource struct {
	// path is absolute; rel is the project-relative slash form used as the
	// digest key, so a store stays portable across checkouts.
	path string
	rel  string
	kind contextSourceKind
}

type contextSourceKind int

const (
	sourceKindTerms contextSourceKind = iota
	sourceKindMemory
)

// SeedProjectContext compiles the project's committed context sources into its
// store, skipping every source whose bytes are unchanged since the last
// compile. It is the read-path counterpart of the compile `kapi apply` runs
// after editing a source: the same importers, the same single store-write path.
//
// The sources are the terms bundle bound by `defaults.terms_source` and every
// content-memory bundle under `.kapi/memory/`, plus the bundle bound by
// `defaults.memory_source` when it lives elsewhere. The memory directory is
// globbed rather than read from the binding alone because a project keeps one
// bundle per content surface and `defaults.memory_source` names only the
// primary among them — a recipe that binds none would otherwise seed nothing.
//
// A project with no committed sources is not an error: the result is empty and
// the store is left alone. A store this build cannot open (the browser build)
// is likewise not an error — there is nothing to project into.
func (a *App) SeedProjectContext(ctx context.Context, projectPath string) (SeedContextResult, error) {
	var res SeedContextResult

	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return res, err
	}
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return res, fmt.Errorf("load project: %w", err)
	}
	sources, err := committedContextSources(proj, layout)
	if err != nil {
		return res, err
	}

	db, err := a.ProjectDB(ctx, layout.Root)
	if err != nil {
		return res, err
	}
	stamps := loadContextDigests(ctx, db)
	next := make(map[string]string, len(sources))

	for _, src := range sources {
		if !storeHolds(db, src.kind) {
			// A build with no file-backed store for this subsystem (the browser
			// build) has nothing to project into. No stamp is recorded, so the
			// source compiles the first time a real store is there.
			res.Skipped++
			continue
		}
		digest, derr := fileDigest(src.path)
		if derr != nil {
			return res, derr
		}
		next[src.rel] = digest
		if stamps[src.rel] == digest {
			res.Skipped++
			continue
		}
		n, cerr := a.compileContextSource(ctx, db, src)
		if cerr != nil {
			return res, cerr
		}
		switch src.kind {
		case sourceKindTerms:
			res.TermsFiles++
			res.Concepts += n
		case sourceKindMemory:
			res.MemoryFiles++
			res.Entries += n
		}
	}

	if res.MemoryFiles > 0 {
		if tm := db.Memory(); tm != nil {
			a.RebuildMemorySearchIndexes(ctx, tm)
		}
	}
	// The stamp map is rebuilt from the sources this pass saw rather than merged
	// into the stored one, so a bundle deleted from git stops being remembered.
	// It is saved only when there were sources at all: a project with none is
	// left with its stamps as they were rather than having an empty map written
	// over them.
	if len(sources) > 0 {
		if err := saveContextDigests(ctx, db, next); err != nil {
			return res, err
		}
	}

	// The committed translations, after the bundles and never before them: on the
	// pass that compiles both — a fresh clone — the record answers last, which is
	// what makes the reviewed wording git carries supersede the accelerant.
	record, rerr := a.absorbCommittedRecord(ctx, db, proj, projectPath, layout)
	if rerr != nil {
		return res, rerr
	}
	res.Record = record
	return res, nil
}

// stampContextSource records a source's current digest as compiled, for a
// caller that just wrote the file and imported it itself (the apply paths, the
// terminology projection). Without it the next seeding pass re-imports a source
// the store already holds — harmless, since the importers upsert, but pure cost
// and it would repeat on every run.
//
// Best-effort: the stamp is an optimization, and a store that cannot record it
// simply re-imports next time.
func (a *App) stampContextSource(ctx context.Context, root, srcPath string) {
	db, err := a.ProjectDB(ctx, root)
	if err != nil {
		return
	}
	digest, derr := fileDigest(srcPath)
	if derr != nil {
		return
	}
	stamps := loadContextDigests(ctx, db)
	stamps[relSlash(root, srcPath)] = digest
	_ = saveContextDigests(ctx, db, stamps)
}

// ContextSourcesUnseeded reports a project that binds committed context sources
// none of which has ever been compiled into its store — a fresh clone, before
// anything ran. A read surface uses it to say that an empty answer is unread
// rather than absent.
//
// It reads the compile stamps only: no digest is computed, so a source edited
// since its last compile is NOT reported here. That is the intended line — the
// state worth naming is a store that has never been written, because a stale one
// still answers with something the project decided.
func (a *App) ContextSourcesUnseeded(ctx context.Context, projectPath string) bool {
	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return false
	}
	proj, err := project.LoadWithOptions(projectPath, project.LoadOptions{SkipRequiresCheck: true})
	if err != nil {
		return false
	}
	sources, err := committedContextSources(proj, layout)
	if err != nil || len(sources) == 0 {
		return false
	}
	if _, statErr := os.Stat(layout.StorePath()); statErr != nil {
		return true
	}
	db, err := a.ProjectDB(ctx, layout.Root)
	if err != nil {
		return false
	}
	stamps := loadContextDigests(ctx, db)
	for _, src := range sources {
		if stamps[src.rel] != "" {
			return false
		}
	}
	return true
}

// projectStoreExists reports whether the project already has a store file. It
// stats rather than opens, because opening creates one — the distinction a dry
// run depends on.
func projectStoreExists(projectPath string) bool {
	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return false
	}
	_, statErr := os.Stat(layout.StorePath())
	return statErr == nil
}

// storeHolds reports whether this build's store carries the subsystem a source
// compiles into.
func storeHolds(db *projectdb.DB, kind contextSourceKind) bool {
	switch kind {
	case sourceKindTerms:
		return db.Terms() != nil
	case sourceKindMemory:
		return db.Memory() != nil
	}
	return false
}

// compileContextSource imports one bundle through the same importer the apply
// path uses, returning the number of concepts or entries it carried.
func (a *App) compileContextSource(ctx context.Context, db *projectdb.DB, src contextSource) (int, error) {
	switch src.kind {
	case sourceKindTerms:
		tb := db.Terms()
		if tb == nil {
			return 0, fmt.Errorf("compile terms: %w", projectdb.ErrNoStore)
		}
		f, err := os.Open(src.path)
		if err != nil {
			return 0, fmt.Errorf("open terms source: %w", err)
		}
		defer f.Close()
		n, err := ImportKTBFile(ctx, tb, f)
		if err != nil {
			return 0, fmt.Errorf("compile terms %s: %w", src.rel, err)
		}
		return n, nil
	case sourceKindMemory:
		tm := db.Memory()
		if tm == nil {
			return 0, fmt.Errorf("compile content memory: %w", projectdb.ErrNoStore)
		}
		n, err := ImportKMBFile(ctx, tm, src.path)
		if err != nil {
			return 0, fmt.Errorf("compile content memory %s: %w", src.rel, err)
		}
		return n, nil
	}
	return 0, fmt.Errorf("compile context source %s: unknown kind", src.rel)
}

// committedContextSources resolves the committed bundles a seeding pass
// compiles, in a stable order (terms first, then memory bundles by path) so two
// runs over the same tree do the same work in the same sequence. A bound source
// that does not exist yet is skipped, not an error: a recipe may bind the file
// an author has not written.
func committedContextSources(proj *project.KapiProject, layout project.Layout) ([]contextSource, error) {
	var out []contextSource
	seen := map[string]bool{}

	add := func(path string, kind contextSourceKind) error {
		abs := resolveUnder(layout.Root, path)
		if seen[abs] {
			return nil
		}
		info, err := os.Stat(abs)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("stat context source %s: %w", path, err)
		}
		if info.IsDir() {
			return nil
		}
		seen[abs] = true
		out = append(out, contextSource{path: abs, rel: relSlash(layout.Root, abs), kind: kind})
		return nil
	}

	if bound := proj.Defaults.TermsSource; bound != "" {
		if err := add(bound, sourceKindTerms); err != nil {
			return nil, err
		}
	}

	var memoryPaths []string
	entries, err := os.ReadDir(layout.MemoryDir())
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read content-memory bundles: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !kmb.IsBundlePath(e.Name()) {
			continue
		}
		memoryPaths = append(memoryPaths, filepath.Join(layout.MemoryDir(), e.Name()))
	}
	sort.Strings(memoryPaths)
	if bound := proj.Defaults.MemorySource; bound != "" {
		memoryPaths = append(memoryPaths, resolveUnder(layout.Root, bound))
	}
	for _, p := range memoryPaths {
		if err := add(p, sourceKindMemory); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// relSlash renders path relative to root in slash form, falling back to the
// base name when the two share no prefix (a bound source outside the project).
func relSlash(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.Base(path)
	}
	return filepath.ToSlash(rel)
}

// fileDigest is the content identity a compile is keyed by.
func fileDigest(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read context source %s: %w", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// loadContextDigests reads the per-source compile stamps. Any uncertainty — an
// unreadable value, a store without the metadata schema — yields an empty map,
// so every source reads as needing a compile. That is the safe direction: a
// redundant import is idempotent, a skipped one leaves the store behind git.
func loadContextDigests(ctx context.Context, db *projectdb.DB) map[string]string {
	v, ok, err := db.Meta(ctx, MetaContextSourceDigests)
	if err != nil || !ok {
		return map[string]string{}
	}
	var stamps map[string]string
	if json.Unmarshal([]byte(v), &stamps) != nil || stamps == nil {
		return map[string]string{}
	}
	return stamps
}

// saveContextDigests records the per-source compile stamps.
func saveContextDigests(ctx context.Context, db *projectdb.DB, stamps map[string]string) error {
	data, err := json.Marshal(stamps)
	if err != nil {
		return fmt.Errorf("encode context source digests: %w", err)
	}
	if err := db.PutMeta(ctx, MetaContextSourceDigests, string(data)); err != nil && !errors.Is(err, projectdb.ErrNoStore) {
		return err
	}
	return nil
}

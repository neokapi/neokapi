// Package sample provides the embedded sample project for the kapi-desktop
// app: KapiMart, a governed multi-collection project in the natural per-area
// layout (web/src/legal/marketing with locale dirs beside source), shipping the
// committed context its recipe binds.
package sample

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/kmb"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
)

// `all:` is required: the sample commits its `.kapi/` context — the voice
// profile, the terms record, the content memory and the unit-state ledger — and
// a plain pattern excludes every name beginning with a dot.
//
//go:embed all:kapimart
var assetsFS embed.FS

// DisplayName maps an internal sample name to its user-facing name.
var DisplayName = map[string]string{
	"kapimart": "KapiMart",
}

// List returns the available sample project names.
func List() []string {
	return []string{"kapimart"}
}

// Scaffold creates a sample project on disk at targetDir.
// name must be "kapimart".
func Scaffold(name, targetDir string) error {
	if _, ok := DisplayName[name]; !ok {
		return fmt.Errorf("unknown sample project %q", name)
	}

	// Copy source files: natural per-area layout (source under <area>/en/,
	// localized files beside it under sibling locale dirs — no separate
	// output/ tree).
	for _, area := range []string{"web", "src", "legal", "marketing"} {
		if err := copyEmbeddedDir("kapimart/"+area, filepath.Join(targetDir, area)); err != nil {
			return fmt.Errorf("copy %s files: %w", area, err)
		}
	}

	// Copy the committed context: the voice profile, the terms record and the
	// content memory the recipe binds. They land on disk as authored files, the
	// same ones `git diff` would review in a real project, and the store is
	// compiled from them below rather than from a second copy.
	if err := copyEmbeddedDir("kapimart/"+project.StateDirName, filepath.Join(targetDir, project.StateDirName)); err != nil {
		return fmt.Errorf("copy context files: %w", err)
	}

	// Copy the project recipe (kapi.yaml).
	kapiData, err := assetsFS.ReadFile(name + "/kapi.yaml")
	if err != nil {
		return fmt.Errorf("read kapi.yaml: %w", err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create project dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "kapi.yaml"), kapiData, 0o644); err != nil {
		return fmt.Errorf("write kapi.yaml: %w", err)
	}

	// Seed the content memory and terms into the project's own store. Both are
	// schemas of `.kapi/work/store.db`, so this is one handle rather than two files —
	// and the handle must be closed before the caller opens the project, or the
	// process would hold two connection pools on it.
	if err := seedStore(targetDir); err != nil {
		return err
	}

	// Stamp the sample manifest (.kapi/sample.json) so the desktop can detect an
	// out-of-date scaffolded copy on disk and offer to refresh it.
	if err := writeManifest(name, targetDir); err != nil {
		return fmt.Errorf("write sample manifest: %w", err)
	}

	return nil
}

// --- KapiMart seed functions ---

var v2Targets = []model.LocaleID{"de", "fr", "ja", "nb", "ar"}

// seedStore opens the sample project's store, seeds the content memory and the
// terms, and closes it. Opening creates `.kapi/` and the store file, so nothing
// needs to make the state directory first.
func seedStore(targetDir string) error {
	layout := project.Layout{
		Root:     targetDir,
		StateDir: filepath.Join(targetDir, project.StateDirName),
	}
	db, err := projectdb.Open(context.Background(), layout)
	if err != nil {
		return fmt.Errorf("open sample project store: %w", err)
	}
	defer db.Close()

	if err := seedTMv2(db.Memory(), targetDir); err != nil {
		return fmt.Errorf("seed content memory: %w", err)
	}
	if err := seedTermsv2(db.Terms(), targetDir); err != nil {
		return fmt.Errorf("seed terms: %w", err)
	}
	return nil
}

// MemorySourceRel and TermsSourceRel are where the committed context sources
// sit inside a scaffolded project. They are the paths `kapi.yaml` binds under
// defaults.memory_source and defaults.terms_source, so the store is compiled
// from the same files the recipe names.
var (
	MemorySourceRel = filepath.Join(project.StateDirName, project.MemoryDirName, kmb.ConventionalName)
	TermsSourceRel  = filepath.Join(project.StateDirName, ktb.ConventionalName)
)

func seedTMv2(tm *memory.SQLiteStore, targetDir string) error {
	ctx := context.Background()

	// Compiled through the same importer `kapi apply` uses, from the file the
	// recipe binds, so the store has one writer and the committed bundle is the
	// only description of what is in it.
	if _, err := host.ImportKMBFile(ctx, tm, filepath.Join(targetDir, MemorySourceRel)); err != nil {
		return fmt.Errorf("compile content memory: %w", err)
	}

	// The bulk path skips the per-row FTS5 inserts, leaving the search and fuzzy
	// side-tables empty until they are rebuilt set-wise. Exact lookup works
	// without this; search and fuzzy lookup return nothing and report no error.
	if err := tm.RebuildSearchIndex(ctx); err != nil {
		return fmt.Errorf("rebuild content-memory search index: %w", err)
	}
	if err := tm.RebuildFuzzyIndex(ctx); err != nil {
		return fmt.Errorf("rebuild content-memory fuzzy index: %w", err)
	}

	// The entries keep the timestamps the bundle carries. They are spread across
	// the history the sample models, and each one matches the decision in
	// `.kapi/state` that approved that unit — a scaffold-time reshuffle would
	// give the memory a different account of when the work happened than the
	// ledger has.
	return nil
}

func seedTermsv2(tb *terms.SQLiteStore, targetDir string) error {
	f, err := os.Open(filepath.Join(targetDir, TermsSourceRel))
	if err != nil {
		return fmt.Errorf("open terms source: %w", err)
	}
	defer f.Close()

	if _, err := host.ImportKTBFile(context.Background(), tb, f); err != nil {
		return fmt.Errorf("compile terms: %w", err)
	}
	spreadTimestamps(tb.DB(), "tb_concepts", 90)
	return nil
}

func spreadTimestamps(db *storage.DB, table string, days int) {
	// Ordered by id, not at random: the sequence has to be the same on every
	// machine for the seeded rng below to mean anything.
	rows, err := db.Query(fmt.Sprintf("SELECT id FROM %s ORDER BY id", table))
	if err != nil {
		return
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return
	}

	now := time.Now()
	rng := rand.New(rand.NewSource(42))
	for _, id := range ids {
		// Bias toward recent: square the random value so more entries cluster near today.
		daysAgo := int(float64(days) * rng.Float64() * rng.Float64())
		ts := now.AddDate(0, 0, -daysAgo).Format(time.RFC3339)
		_, _ = db.Exec(
			fmt.Sprintf("UPDATE %s SET created_at = ?, updated_at = ? WHERE id = ?", table),
			ts, ts, id,
		)
	}
}

func copyEmbeddedDir(srcDir, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	return fs.WalkDir(assetsFS, srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(destDir, rel)

		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		data, err := assetsFS.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0o644)
	})
}

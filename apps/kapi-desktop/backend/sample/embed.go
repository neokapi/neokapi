// Package sample provides embedded sample projects for the kapi-desktop app.
// Two sample projects ("kapimart" and "okapimart") share identical source files
// but use different format engines — native Go vs Okapi Bridge — so users can
// compare them side by side.
package sample

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/termbase"
)

//go:embed shared/* kapimart/* okapimart/*
var assetsFS embed.FS

// DisplayName maps an internal sample name to its user-facing name.
var DisplayName = map[string]string{
	"kapimart":  "KapiMart",
	"okapimart": "OkapiMart",
}

// List returns the available sample project names.
func List() []string {
	return []string{"kapimart", "okapimart"}
}

// Scaffold creates a sample project on disk at targetDir.
// name must be "kapimart" or "okapimart".
func Scaffold(name, targetDir string) error {
	if _, ok := DisplayName[name]; !ok {
		return fmt.Errorf("unknown sample project %q", name)
	}

	// 1. Copy shared input files.
	if err := copyEmbeddedDir("shared/input", filepath.Join(targetDir, "input")); err != nil {
		return fmt.Errorf("copy input files: %w", err)
	}

	// 2. Copy the project-specific .kapi file.
	kapiData, err := assetsFS.ReadFile(name + "/project.kapi")
	if err != nil {
		return fmt.Errorf("read project.kapi: %w", err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create project dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "project.kapi"), kapiData, 0o644); err != nil {
		return fmt.Errorf("write project.kapi: %w", err)
	}

	// 3. Create output directory.
	if err := os.MkdirAll(filepath.Join(targetDir, "output"), 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}

	// 4. Seed TM.
	kapiDir := filepath.Join(targetDir, ".kapi")
	if err := os.MkdirAll(kapiDir, 0o755); err != nil {
		return fmt.Errorf("create .kapi dir: %w", err)
	}
	if err := seedTM(filepath.Join(kapiDir, "tm.db")); err != nil {
		return fmt.Errorf("seed TM: %w", err)
	}

	// 5. Seed termbase.
	if err := seedTermbase(filepath.Join(kapiDir, "termbase.db")); err != nil {
		return fmt.Errorf("seed termbase: %w", err)
	}

	return nil
}

func seedTM(dbPath string) error {
	tmxData, err := assetsFS.ReadFile("shared/tm-seed.tmx")
	if err != nil {
		return fmt.Errorf("read TMX: %w", err)
	}
	tm, err := sievepen.NewSQLiteTM(dbPath)
	if err != nil {
		return err
	}
	defer tm.Close()
	// Import once per target language since ImportTMX requires a specific pair.
	for _, tgt := range []model.LocaleID{"fr-FR", "de-DE", "ja-JP"} {
		if _, err := sievepen.ImportTMX(tm, bytes.NewReader(tmxData), "en-US", tgt); err != nil {
			return fmt.Errorf("import TMX for %s: %w", tgt, err)
		}
	}
	// Spread created_at over the past 30 days to simulate realistic activity.
	spreadTimestamps(tm.DB(), "tm_entries", 30)
	return nil
}

func seedTermbase(dbPath string) error {
	tbData, err := assetsFS.ReadFile("shared/termbase-seed.json")
	if err != nil {
		return fmt.Errorf("read termbase JSON: %w", err)
	}
	tb, err := termbase.NewSQLiteTermBase(dbPath)
	if err != nil {
		return err
	}
	defer tb.Close()
	if _, err := termbase.ImportJSON(tb, bytes.NewReader(tbData)); err != nil {
		return fmt.Errorf("import termbase: %w", err)
	}
	// Spread created_at over the past 30 days to simulate realistic activity.
	spreadTimestamps(tb.DB(), "tb_concepts", 30)
	return nil
}

// spreadTimestamps distributes created_at timestamps across the past `days`
// days so sample data produces a realistic activity chart. Each row gets a
// random date within the window, with a bias toward more recent dates.
func spreadTimestamps(db *storage.DB, table string, days int) {
	rows, err := db.Query(fmt.Sprintf("SELECT id FROM %s ORDER BY RANDOM()", table))
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
	rng := rand.New(rand.NewSource(42)) // deterministic for reproducibility
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
		// Compute relative path from srcDir.
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

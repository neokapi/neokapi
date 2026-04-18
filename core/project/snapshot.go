package project

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Snapshot writes the project at `layout` to `dst` as a `.klz`
// archive. The output is a zip with the canonical layout:
//
//	manifest.yaml          ← merged Recipe + State
//	cache.db               ← optional, klzdb snapshot if present
//	collections/**         ← sidecar kinds that live on disk
//	sources/**             ← included only if opts.IncludeSources
//
// No hidden `.kapi/` prefix anywhere in the zip. No recipe filename
// at zip root (users don't address the recipe inside an archive).
// See AD-046 §6.
func Snapshot(layout Layout, dst io.Writer, opts SnapshotOptions) error {
	recipeBytes, err := os.ReadFile(layout.RecipePath)
	if err != nil {
		return fmt.Errorf("project: read recipe: %w", err)
	}

	archiveManifest, err := mergeToArchiveManifest(recipeBytes, layout)
	if err != nil {
		return err
	}
	archiveManifest.State.SnapshotAt = time.Now().UTC().Format(time.RFC3339)

	zw := zip.NewWriter(dst)
	defer zw.Close()

	// manifest.yaml (canonical at zip root).
	manifestBytes, err := EncodeArchiveManifest(archiveManifest)
	if err != nil {
		return err
	}
	if err := writeZipFile(zw, ArchiveManifestFilename, manifestBytes); err != nil {
		return err
	}

	// cache.db (optional — klzdb provider output).
	cacheDB := filepath.Join(layout.StateDir, "cache.db")
	if info, err := os.Stat(cacheDB); err == nil && !info.IsDir() {
		if !opts.ExcludeCacheDB {
			if err := zipFileFromDisk(zw, cacheDB, "cache.db"); err != nil {
				return err
			}
		}
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("project: stat cache.db: %w", err)
	}

	// collections/** (sidecar kinds on disk).
	collectionsDir := filepath.Join(layout.StateDir, "collections")
	if err := zipDirIfExists(zw, collectionsDir, "collections"); err != nil {
		return err
	}

	// sources/** (optional handoff mode).
	if opts.IncludeSources {
		for _, globRoot := range opts.SourceRoots {
			if err := zipDirIfExists(zw, globRoot, filepath.Join("sources", filepath.Base(globRoot))); err != nil {
				return err
			}
		}
	}

	return nil
}

// SnapshotOptions controls what goes into a `.klz`.
type SnapshotOptions struct {
	// IncludeSources copies the named directories into `sources/**`
	// inside the archive. Off by default — handoff artefacts should
	// not embed the authoring tree.
	IncludeSources bool
	// SourceRoots is the list of absolute directories whose trees
	// get copied under `sources/<basename>/**` when IncludeSources
	// is true. Typically the project's declared source globs,
	// resolved to their roots.
	SourceRoots []string
	// ExcludeCacheDB skips the klzdb snapshot. The archive is still
	// valid (sidecars + recipe are present); consumers that want to
	// rebuild state from the source files can opt for smaller
	// archives this way.
	ExcludeCacheDB bool
}

// Open extracts a `.klz` archive into a working project at
// `targetDir`. The directory is created if it doesn't exist; it must
// be empty, or Open errors (refuses to overwrite a non-empty target).
//
// On success, `targetDir` contains:
//
//	{project.id}.kapi        ← recipe (name from ArchiveManifest.Project.ID)
//	.kapi/
//	  manifest.yaml          ← state bookkeeping
//	  cache.db               ← only if the archive carried one
//	  collections/**         ← sidecars
//
// Returns the resolved Layout ready for further operations.
func Open(src io.ReaderAt, size int64, targetDir string) (Layout, error) {
	if err := ensureEmptyDir(targetDir); err != nil {
		return Layout{}, err
	}
	zr, err := zip.NewReader(src, size)
	if err != nil {
		return Layout{}, fmt.Errorf("project: read archive: %w", err)
	}

	// Parse the manifest first so we know the recipe filename.
	manifest, err := readArchiveManifest(zr)
	if err != nil {
		return Layout{}, err
	}

	projectID := manifest.Project.ID
	if projectID == "" {
		projectID = "project"
	}
	recipeName := projectID + RecipeExt

	layout := Layout{
		Root:       targetDir,
		RecipePath: filepath.Join(targetDir, recipeName),
		StateDir:   filepath.Join(targetDir, StateDirName),
	}
	if err := EnsureLayout(layout); err != nil {
		return Layout{}, err
	}

	// Split manifest into recipe + state and write them.
	if err := writeRecipeFromArchive(layout, manifest); err != nil {
		return Layout{}, err
	}
	if err := writeStateFromArchive(layout, manifest); err != nil {
		return Layout{}, err
	}

	// Extract non-manifest entries.
	for _, f := range zr.File {
		if f.Name == ArchiveManifestFilename {
			continue
		}
		if strings.HasPrefix(f.Name, "sources/") {
			// Skip sources on open by default — they'd clash with the
			// user's own working tree. Users who want them can unzip
			// the archive manually.
			continue
		}
		dest := filepath.Join(layout.StateDir, f.Name)
		if err := extractZipEntry(f, dest); err != nil {
			return Layout{}, err
		}
	}

	return layout, nil
}

// ─── internals ──────────────────────────────────────────────────

func mergeToArchiveManifest(recipeBytes []byte, layout Layout) (*ArchiveManifest, error) {
	var rawRecipe map[string]any
	if err := yaml.Unmarshal(recipeBytes, &rawRecipe); err != nil {
		return nil, fmt.Errorf("project: decode recipe: %w", err)
	}

	recipe := ArchiveRecipe{Raw: rawRecipe}
	if v, ok := rawRecipe["version"].(string); ok {
		recipe.Version = v
		delete(rawRecipe, "version")
	}
	if v, ok := rawRecipe["id"].(string); ok {
		recipe.ID = v
		delete(rawRecipe, "id")
	}
	if v, ok := rawRecipe["name"].(string); ok {
		recipe.Name = v
		delete(rawRecipe, "name")
	}
	if v, ok := rawRecipe["sourceLocale"].(string); ok {
		recipe.SourceLocale = v
		delete(rawRecipe, "sourceLocale")
	}
	if v, ok := rawRecipe["targetLocales"].([]any); ok {
		for _, t := range v {
			if s, ok := t.(string); ok {
				recipe.TargetLocales = append(recipe.TargetLocales, s)
			}
		}
		delete(rawRecipe, "targetLocales")
	}

	if recipe.ID == "" {
		// Fall back to the recipe filename stem.
		stem := strings.TrimSuffix(filepath.Base(layout.RecipePath), RecipeExt)
		recipe.ID = stem
	}

	state := ArchiveState{}
	if sm, err := LoadState(layout); err != nil {
		return nil, err
	} else if sm != nil {
		state.Generator = sm.Generator
		state.Blocks = sm.Blocks
	}

	return &ArchiveManifest{
		SchemaVersion: 1,
		Kind:          ArchiveManifestKind,
		Project:       recipe,
		State:         state,
	}, nil
}

func writeRecipeFromArchive(layout Layout, m *ArchiveManifest) error {
	// Reassemble recipe YAML: top-level fields + the Raw map.
	out := map[string]any{}
	for k, v := range m.Project.Raw {
		out[k] = v
	}
	if m.Project.Version != "" {
		out["version"] = m.Project.Version
	}
	if m.Project.ID != "" {
		out["id"] = m.Project.ID
	}
	if m.Project.Name != "" {
		out["name"] = m.Project.Name
	}
	if m.Project.SourceLocale != "" {
		out["sourceLocale"] = m.Project.SourceLocale
	}
	if len(m.Project.TargetLocales) > 0 {
		out["targetLocales"] = m.Project.TargetLocales
	}
	b, err := yaml.Marshal(out)
	if err != nil {
		return fmt.Errorf("project: encode recipe: %w", err)
	}
	if err := os.WriteFile(layout.RecipePath, b, 0o644); err != nil {
		return fmt.Errorf("project: write recipe: %w", err)
	}
	return nil
}

func writeStateFromArchive(layout Layout, m *ArchiveManifest) error {
	state := &StateManifest{
		SchemaVersion: 1,
		Kind:          StateManifestKind,
		Generator:     m.State.Generator,
		Project: StateProjectRef{
			ID:   m.Project.ID,
			Path: "../" + filepath.Base(layout.RecipePath),
		},
		Blocks: m.State.Blocks,
	}
	return SaveState(layout, state)
}

func readArchiveManifest(zr *zip.Reader) (*ArchiveManifest, error) {
	for _, f := range zr.File {
		if f.Name != ArchiveManifestFilename {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("project: open archive manifest: %w", err)
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		if err != nil {
			return nil, fmt.Errorf("project: read archive manifest: %w", err)
		}
		return DecodeArchiveManifest(b)
	}
	return nil, errors.New("project: archive missing manifest.yaml at zip root")
}

func writeZipFile(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("project: zip create %s: %w", name, err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("project: zip write %s: %w", name, err)
	}
	return nil
}

func zipFileFromDisk(zw *zip.Writer, srcPath, arcName string) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("project: read %s: %w", srcPath, err)
	}
	return writeZipFile(zw, arcName, data)
}

func zipDirIfExists(zw *zip.Writer, srcDir, arcPrefix string) error {
	info, err := os.Stat(srcDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("project: stat %s: %w", srcDir, err)
	}
	if !info.IsDir() {
		return nil
	}
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("project: rel %s: %w", path, err)
		}
		arcName := filepath.ToSlash(filepath.Join(arcPrefix, rel))
		return zipFileFromDisk(zw, path, arcName)
	})
}

func extractZipEntry(f *zip.File, dest string) error {
	if strings.HasSuffix(f.Name, "/") {
		return os.MkdirAll(dest, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("project: mkdir %s: %w", filepath.Dir(dest), err)
	}
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("project: open zip entry %s: %w", f.Name, err)
	}
	defer rc.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("project: create %s: %w", dest, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, rc); err != nil {
		return fmt.Errorf("project: copy %s: %w", dest, err)
	}
	return nil
}

func ensureEmptyDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("project: mkdir %s: %w", dir, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("project: read %s: %w", dir, err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("project: target directory %q is not empty", dir)
	}
	return nil
}

// Ensure we import bytes even if used only by tests compiled later.
var _ = bytes.NewReader

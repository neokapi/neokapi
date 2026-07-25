package host

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/blockstore/exporter"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/kbf"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/kpz"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/sievepen/kmb"
	"github.com/neokapi/neokapi/termbase"
	"github.com/neokapi/neokapi/termbase/ktb"
)

// RunPack snapshots a .kapi project's working state — block-store overlays
// (and any blocks), the authoritative TM, and the termbase — into a
// portable .kpz. Regenerable caches and secrets are excluded (AD-025 §4).
func (a *App) RunPack(cmd Command) error {
	projectPath, err := RequireProjectPath(cmd)
	if err != nil {
		return err
	}
	outPath, _ := cmd.Flags().GetString("output")
	if outPath == "" {
		outPath = "snapshot" + workspaceExt
	}
	if !strings.HasSuffix(outPath, workspaceExt) {
		outPath += workspaceExt
	}

	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	withSource, _ := cmd.Flags().GetBool("with-source")
	pkg := &kpz.Package{Kind: kpz.KindProject, Generator: &kpz.GeneratorInfo{ID: "kapi"}}

	// Full project recipe — the one source of truth for intent (AD-025 §6).
	// Side-effecting Extras (server/hooks/automations) are stripped so they
	// travel inert; secrets never live in a recipe (keychain).
	if recipe, lerr := project.Load(projectPath); lerr == nil {
		pkg.Recipe = kpz.SanitizeRecipe(recipe)
	} else {
		fmt.Fprintf(os.Stderr, "Warning: pack: load recipe %s: %v (packing content only)\n", projectPath, lerr)
	}

	// Source identity + per-source skeletons collected from the project's
	// extraction manifests (deduped by source hash). Raw bytes only with
	// --with-source.
	if err := a.collectProjectSources(pkg, layout, projectPath, withSource); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: pack: collect sources: %v\n", err)
	}

	// Block store (blocks + overlays).
	if blocksPath := layout.BlockStorePath(); fileExists(blocksPath) {
		store, err := sqlitestore.New(blocksPath)
		if err != nil {
			return fmt.Errorf("open block store: %w", err)
		}
		snap, err := exporter.Export(ctx, store)
		_ = store.Close()
		if err != nil {
			return fmt.Errorf("export block store: %w", err)
		}
		pkg.Overlays = storeToKpzOverlays(snap.Overlays)
		if len(snap.Blocks) > 0 {
			pkg.Blocks = []kpz.BlockDoc{{Path: "blocks/project.kbf", File: blocksToKBF(snap.Blocks, a.SourceLang)}}
		}
	}

	// Authoritative TM.
	if tmPath := filepath.Join(layout.StateDir, "tm.db"); fileExists(tmPath) {
		tm, err := sievepen.NewSQLiteTM(tmPath)
		if err != nil {
			return fmt.Errorf("open project TM: %w", err)
		}
		entries, err := tm.Entries(cmd.Context())
		_ = tm.Close()
		if err != nil {
			return fmt.Errorf("read project TM: %w", err)
		}
		if len(entries) > 0 {
			pkg.TM = kmb.FromModel(entries, nil)
		}
	}

	// Termbase.
	if tbPath := filepath.Join(layout.StateDir, "termbase.db"); fileExists(tbPath) {
		tb, err := termbase.NewSQLiteTermBase(tbPath)
		if err != nil {
			return fmt.Errorf("open project termbase: %w", err)
		}
		concepts, err := tb.Concepts(cmd.Context())
		_ = tb.Close()
		if err != nil {
			return fmt.Errorf("read project termbase: %w", err)
		}
		if len(concepts) > 0 {
			pkg.Termbase = ktb.FromConcepts(concepts)
		}
	}

	// Opt-in tamper-evident provenance: a hash-chained line recording this
	// pack. Advisory and content-subordinate — excluded from the package
	// rootHash, never read to decide anything, safe to delete (AD-025 §5).
	if logIt, _ := cmd.Flags().GetBool("log"); logIt {
		pkg.History = kpz.AppendHistory(pkg.History, kpz.HistoryEvent{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Event:     "pack",
			Note:      filepath.Base(projectPath),
		})
	}

	// Refuse to write a content-less snapshot — the way `git bundle` refuses an
	// empty bundle. A project with no extracted content, TM, or terminology has
	// nothing worth packing; its intent is the .kapi recipe, which is shared via
	// git, not a .kpz.
	if !pkg.HasContent() {
		return fmt.Errorf("pack: nothing to pack — %s has no extracted content, translation memory, or terminology yet; run `kapi extract` (and translate) first, or share the kapi.yaml recipe directly", filepath.Base(projectPath))
	}

	if err := saveWorkspace(pkg, outPath); err != nil {
		return err
	}
	if a.Quiet {
		return nil
	}
	return outputPrint(cmd, "Packed project working state → "+outPath)
}

// collectProjectSources gathers per-source identity + round-trip skeletons
// from the project's extraction manifests, deduped by source content hash
// (AD-025 §6). The skeleton (the derived extract template) always travels;
// raw source bytes ride only with withSource. Sources whose format had no
// skeleton emitter at extract time contribute identity only.
func (a *App) collectProjectSources(pkg *kpz.Package, layout project.Layout, projectPath string, withSource bool) error {
	manifests, err := project.ListExtractionManifests(layout)
	if err != nil {
		return err
	}
	seen := make(map[string]bool) // source rel path → already added
	for _, m := range manifests {
		for _, pair := range m.Pairs {
			for _, ef := range pair.Files {
				if ef.Source == "" || seen[ef.Source] {
					continue
				}
				seen[ef.Source] = true
				si := kpz.SourceIdentity{
					SourcePath:  ef.Source,
					FormatID:    ef.Format,
					ContentHash: ef.SourceHash,
				}

				// Skeleton: the per-source .bin captured at extract time, streamed
				// from its file into the parcel on demand.
				if ef.Skeleton != "" {
					skelPath := filepath.Join(project.ExtractionDir(layout, m.BatchID), ef.Skeleton)
					if _, serr := os.Stat(skelPath); serr == nil {
						member := kpz.SkeletonDir + ef.Source
						pkg.Skeletons = append(pkg.Skeletons, kpz.SkeletonDoc{
							Path: member, SourcePath: ef.Source, FormatID: ef.Format,
							ContentHash: ef.SourceHash, Content: kpz.FileContent(skelPath),
						})
						si.SkeletonPath = member
					}
				}

				// Raw source bytes (opt-in), streamed from the source file.
				if withSource {
					srcAbs := filepath.Join(layout.Root, ef.Source)
					if _, serr := os.Stat(srcAbs); serr == nil {
						pkg.Source = append(pkg.Source, kpz.SourceDoc{Path: kpz.SourceDir + ef.Source, Content: kpz.FileContent(srcAbs)})
						si.HasRawSource = true
					}
				}
				pkg.Sources = append(pkg.Sources, si)
			}
		}
	}
	return nil
}

// RunUnpack rehydrates a project's working state from a .kpz snapshot into
// the local .kapi/ state dir, recreating the block store, TM, and termbase.
// A workspace .kpz (one carrying a Recipe) instead rebuilds its shadow cache.
func (a *App) RunUnpack(cmd Command, snapshotPath string) error {
	pkg, err := LoadWorkspace(snapshotPath)
	if err != nil {
		return err
	}
	if isWorkspacePackage(pkg) {
		return a.unpackKpz(cmd.Context(), snapshotPath)
	}

	// Resolve the destination project. When one is in scope use it; otherwise
	// reconstitute a fresh <name>.kapi from the snapshot beside the .kpz, since
	// a project snapshot carries the full recipe (AD-025 §6) and can rebuild a
	// complete project in a file.
	projectPath, err := ResolveProjectPath(cmd)
	if err != nil {
		return err
	}
	if projectPath == "" {
		projectPath = reconstitutedProjectPath(snapshotPath, pkg)
	}
	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return err
	}
	if err := project.EnsureLayout(layout); err != nil {
		return err
	}

	// Write the recipe back as <name>.kapi (the authoritative intent). When a
	// project already exists, its on-disk recipe is authoritative — do not
	// overwrite it; only reconstitute when there was no recipe.
	if pkg.Recipe != nil {
		if !fileExists(layout.RecipePath) {
			if serr := project.Save(layout.RecipePath, pkg.Recipe); serr != nil {
				return fmt.Errorf("unpack: write recipe: %w", serr)
			}
		}
	}
	// Verify the advisory provenance chain if present. It is advisory, so a
	// broken chain warns rather than blocks — the content is what matters.
	if len(pkg.History) > 0 {
		if verr := kpz.VerifyHistory(pkg.History); verr != nil {
			fmt.Fprintf(os.Stderr, "Warning: snapshot provenance log is broken: %v\n", verr)
		}
	}
	ctx := cmd.Context()

	// Block store.
	if len(pkg.Overlays) > 0 || len(pkg.Blocks) > 0 {
		store, err := sqlitestore.New(layout.BlockStorePath())
		if err != nil {
			return fmt.Errorf("open block store: %w", err)
		}
		snap := &exporter.Snapshot{Overlays: kpzToStoreOverlays(pkg.Overlays), Blocks: kbfToBlocks(pkg.Blocks)}
		err = exporter.Load(ctx, store, snap)
		_ = store.Close()
		if err != nil {
			return fmt.Errorf("load block store: %w", err)
		}
	}

	// TM.
	if pkg.TM != nil {
		tm, err := sievepen.NewSQLiteTM(filepath.Join(layout.StateDir, "tm.db"))
		if err != nil {
			return fmt.Errorf("open project TM: %w", err)
		}
		for _, e := range pkg.TM.ModelEntries() {
			if aerr := tm.Add(cmd.Context(), e); aerr != nil {
				_ = tm.Close()
				return fmt.Errorf("restore TM entry: %w", aerr)
			}
		}
		_ = tm.Close()
	}

	// Termbase.
	if pkg.Termbase != nil {
		tb, err := termbase.NewSQLiteTermBase(filepath.Join(layout.StateDir, "termbase.db"))
		if err != nil {
			return fmt.Errorf("open project termbase: %w", err)
		}
		for _, c := range pkg.Termbase.Concepts {
			if aerr := tb.AddConcept(cmd.Context(), c); aerr != nil {
				_ = tb.Close()
				return fmt.Errorf("restore concept: %w", aerr)
			}
		}
		_ = tb.Close()
	}

	// Restore raw source bytes when the snapshot carries them. `pack
	// --with-source` embeds them precisely so the recipient can *re-extract*
	// (AD-025 §6), which needs the file on disk — packing bytes that unpack
	// dropped made the flag a one-way trip. An existing file is left alone: the
	// working tree is authoritative wherever it already has an answer, the same
	// rule the recipe follows above.
	if err := restoreSources(pkg, layout.Root); err != nil {
		return err
	}

	// Restore per-source skeletons into an extraction cache dir so a later
	// merge can reuse the round-trip templates without re-extracting (AD-025
	// §6). One synthetic batch holds them all, keyed by source content hash
	// (the same SkeletonFilename scheme extract uses).
	if len(pkg.Skeletons) > 0 {
		batchDir, derr := project.EnsureExtractionDir(layout, "unpacked")
		if derr != nil {
			return fmt.Errorf("unpack: create extraction dir: %w", derr)
		}
		for _, skel := range pkg.Skeletons {
			hash := strings.TrimPrefix(skel.ContentHash, "sha256:")
			if hash == "" {
				hash = skel.SourcePath
			}
			dst := filepath.Join(batchDir, project.SkeletonFilename(hash))
			if werr := copyContentToFile(skel.Content, dst); werr != nil {
				return fmt.Errorf("unpack: write skeleton: %w", werr)
			}
		}
	}

	if a.Quiet {
		return nil
	}
	return outputPrint(cmd, fmt.Sprintf("Unpacked %s → %s", snapshotPath, layout.StateDir))
}

// restoreSources writes a snapshot's raw source members back into the project
// tree under their logical paths (the archive path minus the "source/" prefix).
//
// A file already present is kept: unpack restores state, and the working tree is
// the authority on its own sources. A member whose logical path escapes the
// project root is refused rather than skipped — an archive from elsewhere must
// not be able to write outside the directory it was unpacked into.
func restoreSources(pkg *kpz.Package, root string) error {
	if len(pkg.Source) == 0 {
		return nil
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("unpack: resolve project root: %w", err)
	}
	for _, doc := range pkg.Source {
		rel := strings.TrimPrefix(doc.Path, kpz.SourceDir)
		if rel == "" || strings.HasPrefix(rel, "/") {
			// An absolute member path would be silently reinterpreted as
			// project-relative by filepath.Join. Refusing beats writing
			// somewhere the archive did not name.
			return fmt.Errorf("unpack: source member %q has no usable relative path", doc.Path)
		}
		dst := filepath.Join(absRoot, filepath.FromSlash(rel))
		if !strings.HasPrefix(dst, absRoot+string(os.PathSeparator)) {
			return fmt.Errorf("unpack: source member %q escapes the project root", doc.Path)
		}
		if fileExists(dst) {
			continue // the working tree already has this source
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("unpack: create source dir: %w", err)
		}
		if err := copyContentToFile(doc.Content, dst); err != nil {
			return fmt.Errorf("unpack: write source %s: %w", rel, err)
		}
	}
	return nil
}

// reconstitutedProjectPath derives the kapi.yaml path to materialize when
// unpacking a project snapshot with no project in scope. The recipe filename
// is fixed, so the project's identity is carried by its folder: a fresh
// <name>/ directory beside the snapshot (named from the recipe, else the
// snapshot's base name) holding the kapi.yaml recipe.
func reconstitutedProjectPath(snapshotPath string, pkg *kpz.Package) string {
	dir := filepath.Dir(snapshotPath)
	name := strings.TrimSuffix(filepath.Base(snapshotPath), filepath.Ext(snapshotPath))
	if pkg != nil && pkg.Recipe != nil && pkg.Recipe.Name != "" {
		name = pkg.Recipe.Name
	}
	return filepath.Join(dir, name, project.RecipeFileName)
}

// blocksToKBF wraps exported block-store blocks into a single kbf.File
// document so they ride as a blocks/ member.
func blocksToKBF(entries []exporter.BlockEntry, sourceLocale string) *kbf.File {
	blocks := make([]kbf.Block, 0, len(entries))
	for _, e := range entries {
		blocks = append(blocks, e.Block)
	}
	return &kbf.File{
		SchemaVersion: kbf.SchemaVersion,
		Kind:          kbf.Kind,
		Generator:     kbf.GeneratorInfo{ID: "kapi", Version: "1"},
		Project:       kbf.ProjectInfo{ID: "project", SourceLocale: sourceLocale},
		Documents: []kbf.Document{{
			ID:           "project",
			DocumentType: kbf.DocumentTypeJSX,
			Path:         "project",
			Blocks:       blocks,
		}},
	}
}

// kbfToBlocks flattens block-member documents back into store entries.
func kbfToBlocks(docs []kpz.BlockDoc) []exporter.BlockEntry {
	var out []exporter.BlockEntry
	for _, d := range docs {
		if d.File == nil {
			continue
		}
		for _, doc := range d.File.Documents {
			for _, b := range doc.Blocks {
				out = append(out, exporter.BlockEntry{Block: b})
			}
		}
	}
	return out
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// outputPrint prints a simple string result honoring --json.
func outputPrint(cmd Command, msg string) error {
	return output.Print(cmd, simpleMessage{Message: msg})
}

type simpleMessage struct {
	Message string `json:"message"`
}

func (m simpleMessage) FormatText(w io.Writer) error {
	_, err := fmt.Fprintln(w, m.Message)
	return err
}

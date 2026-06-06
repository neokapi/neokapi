package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/core/blockstore/exporter"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/klz"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/sievepen/klftm"
	"github.com/neokapi/neokapi/termbase"
	"github.com/neokapi/neokapi/termbase/klftb"
	"github.com/spf13/cobra"
)

// NewPackCmd creates the "pack" command: snapshot an in-progress .kapi
// project's working state (block-store overlays + content) into a portable
// .klz, for hand-off or backup.
func (a *App) NewPackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pack -o <snapshot.klz>",
		Short:   "Snapshot a .kapi project's working state into a .klz",
		GroupID: "content",
		Long: `Snapshot a .kapi project's working state — the block-store overlays, the
authoritative translation memory, and the termbase — into a portable .klz.
Regenerable caches and secrets are excluded. Move the snapshot to another
machine and "kapi unpack" it to resume work there.`,
		Example: `  kapi pack -o snapshot.klz   # a .kapi project
  kapi pack work.klz         # eject a .klz workspace's cache`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Ad-hoc workspace: `kapi pack work.klz` ejects the .klz's working
			// cache into the file (the git-bundle hand-off boundary).
			if len(args) == 1 && isKlzPath(args[0]) {
				return a.packKlz(cmd.Context(), args[0])
			}
			return a.runPack(cmd)
		},
	}
	AddProjectFlag(cmd)
	cmd.Flags().StringP("output", "o", "", "output .klz snapshot path")
	cmd.Flags().Bool("log", false, "stamp a tamper-evident provenance line into the snapshot's advisory history")
	cmd.Flags().Bool("with-source", false, "embed raw source bytes in the .klz (default: identity + skeleton only)")
	return cmd
}

// NewInfoCmd creates the "info" command: show a .klz workspace's state —
// documents, locales, output layout, and whether its working cache is dirty
// (has work not yet packed into the .klz). Named `info` because the bowrain
// plugin owns `status`.
func (a *App) NewInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "info <work.klz>",
		Short:   "Show a .klz workspace's state (dirty?)",
		GroupID: "content",
		Example: `  kapi info work.klz`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !isKlzPath(args[0]) {
				return errors.New("info: expects a .klz workspace")
			}
			return a.infoKlz(cmd, args[0])
		},
	}
	output.AddFlags(cmd)
	return cmd
}

// NewUnpackCmd creates the "unpack" command: rehydrate a project's working
// state from a .klz snapshot into the local .kapi/ state dir.
func (a *App) NewUnpackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "unpack <snapshot.klz>",
		Short:   "Rehydrate a project's working state from a .klz snapshot",
		GroupID: "content",
		Example: `  kapi unpack snapshot.klz`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.runUnpack(cmd, args[0])
		},
	}
	AddProjectFlag(cmd)
	return cmd
}

// runPack snapshots a .kapi project's working state — block-store overlays
// (and any blocks), the authoritative TM, and the termbase — into a
// portable .klz. Regenerable caches and secrets are excluded (AD-025 §4).
func (a *App) runPack(cmd *cobra.Command) error {
	projectPath, err := RequireProjectPath(cmd)
	if err != nil {
		return err
	}
	outPath, _ := cmd.Flags().GetString("output")
	if outPath == "" {
		outPath = "snapshot" + WorkspaceExt
	}
	if !strings.HasSuffix(outPath, WorkspaceExt) {
		outPath += WorkspaceExt
	}

	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	withSource, _ := cmd.Flags().GetBool("with-source")
	pkg := &klz.Package{Kind: klz.KindProject, Generator: &klz.GeneratorInfo{ID: "kapi"}}

	// Full project recipe — the one source of truth for intent (AD-025 §6).
	// Side-effecting Extras (server/hooks/automations) are stripped so they
	// travel inert; secrets never live in a recipe (keychain).
	if recipe, lerr := project.Load(projectPath); lerr == nil {
		pkg.Recipe = klz.SanitizeRecipe(recipe)
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
		pkg.Overlays = storeToKlzOverlays(snap.Overlays)
		if len(snap.Blocks) > 0 {
			pkg.Blocks = []klz.BlockDoc{{Path: "blocks/project.klf", File: blocksToKLF(snap.Blocks, a.SourceLang)}}
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
			pkg.TM = klftm.FromModel(entries, nil)
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
			pkg.Termbase = klftb.FromConcepts(concepts)
		}
	}

	// Opt-in tamper-evident provenance: a hash-chained line recording this
	// pack. Advisory and content-subordinate — excluded from the package
	// rootHash, never read to decide anything, safe to delete (AD-025 §5).
	if logIt, _ := cmd.Flags().GetBool("log"); logIt {
		pkg.History = klz.AppendHistory(pkg.History, klz.HistoryEvent{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Event:     "pack",
			Note:      filepath.Base(projectPath),
		})
	}

	// Refuse to write a content-less snapshot — the way `git bundle` refuses an
	// empty bundle. A project with no extracted content, TM, or terminology has
	// nothing worth packing; its intent is the .kapi recipe, which is shared via
	// git, not a .klz.
	if !pkg.HasContent() {
		return fmt.Errorf("pack: nothing to pack — %s has no extracted content, translation memory, or terminology yet; run `kapi extract` (and translate) first, or share the .kapi recipe directly", filepath.Base(projectPath))
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
func (a *App) collectProjectSources(pkg *klz.Package, layout project.Layout, projectPath string, withSource bool) error {
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
				si := klz.SourceIdentity{
					SourcePath:  ef.Source,
					FormatID:    ef.Format,
					ContentHash: ef.SourceHash,
				}

				// Skeleton: the per-source .bin captured at extract time.
				if ef.Skeleton != "" {
					skelPath := filepath.Join(project.ExtractionDir(layout, m.BatchID), ef.Skeleton)
					if data, rerr := os.ReadFile(skelPath); rerr == nil {
						member := klz.SkeletonDir + ef.Source
						pkg.Skeletons = append(pkg.Skeletons, klz.SkeletonDoc{
							Path: member, SourcePath: ef.Source, FormatID: ef.Format,
							ContentHash: ef.SourceHash, Data: data,
						})
						si.SkeletonPath = member
					}
				}

				// Raw source bytes (opt-in).
				if withSource {
					srcAbs := filepath.Join(layout.Root, ef.Source)
					if data, rerr := os.ReadFile(srcAbs); rerr == nil {
						pkg.Source = append(pkg.Source, klz.SourceDoc{Path: "source/" + ef.Source, Data: data})
						si.HasRawSource = true
					}
				}
				pkg.Sources = append(pkg.Sources, si)
			}
		}
	}
	return nil
}

// runUnpack rehydrates a project's working state from a .klz snapshot into
// the local .kapi/ state dir, recreating the block store, TM, and termbase.
// A workspace .klz (one carrying a Recipe) instead rebuilds its shadow cache.
func (a *App) runUnpack(cmd *cobra.Command, snapshotPath string) error {
	pkg, err := loadWorkspace(snapshotPath)
	if err != nil {
		return err
	}
	if isWorkspacePackage(pkg) {
		return a.unpackKlz(cmd.Context(), snapshotPath)
	}

	// Resolve the destination project. When one is in scope use it; otherwise
	// reconstitute a fresh <name>.kapi from the snapshot beside the .klz, since
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
		if verr := klz.VerifyHistory(pkg.History); verr != nil {
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
		snap := &exporter.Snapshot{Overlays: klzToStoreOverlays(pkg.Overlays), Blocks: klfToBlocks(pkg.Blocks)}
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
			if werr := os.WriteFile(dst, skel.Data, 0o644); werr != nil {
				return fmt.Errorf("unpack: write skeleton: %w", werr)
			}
		}
	}

	if a.Quiet {
		return nil
	}
	return outputPrint(cmd, fmt.Sprintf("Unpacked %s → %s", snapshotPath, layout.StateDir))
}

// reconstitutedProjectPath derives the <name>.kapi path to materialize when
// unpacking a project snapshot with no project in scope: prefer the recipe's
// name, else the snapshot's base name, placed beside the snapshot.
func reconstitutedProjectPath(snapshotPath string, pkg *klz.Package) string {
	dir := filepath.Dir(snapshotPath)
	name := strings.TrimSuffix(filepath.Base(snapshotPath), filepath.Ext(snapshotPath))
	if pkg != nil && pkg.Recipe != nil && pkg.Recipe.Name != "" {
		name = pkg.Recipe.Name
	}
	return filepath.Join(dir, name+project.RecipeExt)
}

// blocksToKLF wraps exported block-store blocks into a single klf.File
// document so they ride as a blocks/ member.
func blocksToKLF(entries []exporter.BlockEntry, sourceLocale string) *klf.File {
	blocks := make([]klf.Block, 0, len(entries))
	for _, e := range entries {
		blocks = append(blocks, e.Block)
	}
	return &klf.File{
		SchemaVersion: klf.SchemaVersion,
		Kind:          klf.Kind,
		Generator:     klf.GeneratorInfo{ID: "kapi", Version: "1"},
		Project:       klf.ProjectInfo{ID: "project", SourceLocale: sourceLocale},
		Documents: []klf.Document{{
			ID:           "project",
			DocumentType: klf.DocumentTypeJSX,
			Path:         "project",
			Blocks:       blocks,
		}},
	}
}

// klfToBlocks flattens block-member documents back into store entries.
func klfToBlocks(docs []klz.BlockDoc) []exporter.BlockEntry {
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
func outputPrint(cmd *cobra.Command, msg string) error {
	return output.Print(cmd, simpleMessage{Message: msg})
}

type simpleMessage struct {
	Message string `json:"message"`
}

func (m simpleMessage) FormatText(w io.Writer) error {
	_, err := fmt.Fprintln(w, m.Message)
	return err
}

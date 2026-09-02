package host

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/format"

	"github.com/neokapi/neokapi/core/blockstore/exporter"
	"github.com/neokapi/neokapi/core/kbf"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/kpz"
	"github.com/neokapi/neokapi/memory/kmb"
	"github.com/neokapi/neokapi/terms/ktb"
)

// RunPack snapshots a .kapi project's working state — block-store overlays
// (and any blocks), the authoritative content memory, and the terms store — into a
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
	// Everything side-effecting is stripped so the recipe travels inert (see
	// kpz.SanitizeRecipe); secrets never live in a recipe (keychain). The same
	// sweep runs again on ingest, since this side only binds honest packers —
	// but running it here is what tells the AUTHOR which parts of their own
	// recipe a hand-off cannot carry.
	if recipe, lerr := project.Load(projectPath); lerr == nil {
		sanitized, removed := kpz.SanitizeRecipe(recipe)
		pkg.Recipe = sanitized
		for _, r := range removed {
			fmt.Fprintf(os.Stderr, "Note: pack: %s stays behind, because a package travels inert\n", r)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Warning: pack: load recipe %s: %v (packing content only)\n", projectPath, lerr)
	}

	// Source identity + per-source skeletons collected from the project's
	// extraction manifests (deduped by source hash). Raw bytes only with
	// --with-source.
	if err := a.collectProjectSources(pkg, layout, projectPath, withSource); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: pack: collect sources: %v\n", err)
	}

	// The three stores a snapshot carries all live in the one project store now,
	// so each part is gated on that subsystem holding ROWS rather than on a file
	// existing. The schemas exist from the store's first open, which is why file
	// presence stopped distinguishing "has a content memory" from "has a state
	// directory" — a stat would have packed an empty part for every project.
	db, err := a.ProjectDB(ctx, layout.Root)
	if err != nil {
		return fmt.Errorf("open project store: %w", err)
	}

	// Block store (blocks + overlays). Either alone is worth packing: a flow
	// run leaves overlays without caching blocks.
	if has, herr := db.HasBlockCache(ctx); herr != nil {
		return fmt.Errorf("read block cache: %w", herr)
	} else if has {
		snap, eerr := exporter.Export(ctx, db.BlocksAutocommit())
		if eerr != nil {
			return fmt.Errorf("export block store: %w", eerr)
		}
		pkg.Overlays = storeToKpzOverlays(snap.Overlays)
		if len(snap.Blocks) > 0 {
			pkg.Blocks = []kpz.BlockDoc{{Path: "blocks/project.kbf.json", File: blocksToKBF(snap.Blocks, a.SourceLocale())}}
		}
	}

	// Authoritative content memory.
	if has, herr := db.HasMemory(ctx); herr != nil {
		return fmt.Errorf("read project content memory: %w", herr)
	} else if has {
		entries, eerr := db.Memory().Entries(ctx)
		if eerr != nil {
			return fmt.Errorf("read project content memory: %w", eerr)
		}
		if len(entries) > 0 {
			pkg.Memory = kmb.FromModel(entries, nil)
		}
	}

	// Terms.
	if has, herr := db.HasTerms(ctx); herr != nil {
		return fmt.Errorf("read project terms store: %w", herr)
	} else if has {
		concepts, cerr := db.Terms().Concepts(ctx)
		if cerr != nil {
			return fmt.Errorf("read project terms store: %w", cerr)
		}
		if len(concepts) > 0 {
			pkg.Terms = ktb.FromConcepts(concepts)
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
	// empty bundle. A project with no extracted content, content memory, or terminology has
	// nothing worth packing; its intent is the .kapi recipe, which is shared via
	// git, not a .kpz.
	if !pkg.HasContent() {
		return fmt.Errorf("pack: %s has no extracted content, content memory, or terminology yet, so there is nothing to pack; run `kapi extract` (and translate) first, or share the kapi.yaml recipe directly", filepath.Base(projectPath))
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
// the local .kapi/ state dir, recreating the block store, content memory, and terms.
// A workspace .kpz (one carrying a Recipe) instead rebuilds its shadow cache.
func (a *App) RunUnpack(cmd Command, snapshotPath string) error {
	pkg, err := LoadWorkspace(snapshotPath)
	if err != nil {
		return err
	}
	if isWorkspacePackage(pkg) {
		return a.unpackKpz(cmd.Context(), snapshotPath)
	}

	// Resolve the destination project. When one is in scope its own recipe is
	// authoritative and the package's is only metadata; otherwise the snapshot
	// reconstitutes one, since a project snapshot carries the full recipe
	// (AD-025 §6) and can rebuild a complete project in a file.
	//
	// Either way the layout that comes back names a recipe that exists on disk
	// — project.LayoutFor stats it — so there is no later branch where unpack
	// might still have to write one.
	projectPath, err := ResolveProjectPath(cmd)
	if err != nil {
		return err
	}
	var layout project.Layout
	if projectPath == "" {
		layout, err = a.reconstituteProject(cmd, snapshotPath, pkg)
	} else {
		layout, err = project.LayoutFor(projectPath)
	}
	if err != nil {
		return err
	}
	if err := project.EnsureLayout(layout); err != nil {
		return err
	}
	// Verify the advisory provenance chain if present. It is advisory, so a
	// broken chain warns rather than blocks — the content is what matters.
	if len(pkg.History) > 0 {
		if verr := kpz.VerifyHistory(pkg.History); verr != nil {
			fmt.Fprintf(os.Stderr, "Warning: snapshot provenance log is broken: %v\n", verr)
		}
	}
	ctx := cmd.Context()

	db, err := a.ProjectDB(ctx, layout.Root)
	if err != nil {
		return fmt.Errorf("open project store: %w", err)
	}

	// Block store.
	if len(pkg.Overlays) > 0 || len(pkg.Blocks) > 0 {
		store := db.Blocks()
		if store == nil {
			return fmt.Errorf("restore the block cache: %w", projectdb.ErrNoStore)
		}
		snap := &exporter.Snapshot{Overlays: kpzToStoreOverlays(pkg.Overlays), Blocks: kbfToBlocks(pkg.Blocks)}
		if lerr := exporter.Load(ctx, store, snap); lerr != nil {
			return fmt.Errorf("load block store: %w", lerr)
		}
	}

	// Content memory.
	if pkg.Memory != nil {
		tm := db.Memory()
		if tm == nil {
			return fmt.Errorf("restore the content memory: %w", projectdb.ErrNoStore)
		}
		for _, e := range pkg.Memory.ModelEntries() {
			if aerr := tm.Add(ctx, e); aerr != nil {
				return fmt.Errorf("restore content-memory entry: %w", aerr)
			}
		}
	}

	// Terms.
	if pkg.Terms != nil {
		tb := db.Terms()
		if tb == nil {
			return fmt.Errorf("restore the terms store: %w", projectdb.ErrNoStore)
		}
		for _, c := range pkg.Terms.Concepts {
			if aerr := tb.AddConcept(ctx, c); aerr != nil {
				return fmt.Errorf("restore concept: %w", aerr)
			}
		}
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

// reconstituteProject materializes the project a snapshot describes, for a
// recipient who has none in scope. AD-025 §6 calls a .kpz a project in a file
// and says unpack reconstitutes a complete kapi.yaml; this is the half that
// makes that true, and it is the only path on which unpack writes a recipe at
// all. (Until #1537's follow-up it was unreachable: the path was computed and
// handed straight to project.LayoutFor, which stats the recipe and failed,
// so `kapi unpack` outside a project always died on a missing-file error and
// the adoption prompt guarding it never ran.)
//
// Writing that recipe is the moment the package stops being data and starts
// being intent, so it is the moment there is something to ask about.
// LoadWorkspace has already made the recipe inert — exec-class steps and
// non-local paths are gone — and the question that remains is whether this
// directory becomes a project someone else designed.
func (a *App) reconstituteProject(cmd Command, snapshotPath string, pkg *kpz.Package) (project.Layout, error) {
	base := filepath.Base(snapshotPath)
	if pkg.Recipe == nil {
		return project.Layout{}, fmt.Errorf(
			"unpack: no project here and %s carries no recipe to reconstitute one from. Run this inside a project, or name one with --project",
			base)
	}

	recipePath := reconstitutedProjectPath(snapshotPath, pkg)
	if fileExists(recipePath) {
		// A previous unpack already reconstituted this project. Its on-disk
		// recipe is authoritative exactly as an in-scope project's is, so
		// unpacking again refreshes the state and asks nothing.
		return project.LayoutFor(recipePath)
	}

	ok, err := a.confirmAdoptRecipe(cmd, snapshotPath)
	if err != nil {
		return project.Layout{}, err
	}
	if !ok {
		return project.Layout{}, fmt.Errorf(
			"unpack: no project here, and the recipe %s carries was not adopted, so there is nowhere to unpack into",
			base)
	}

	if err := os.MkdirAll(filepath.Dir(recipePath), 0o755); err != nil {
		return project.Layout{}, fmt.Errorf("unpack: create project directory: %w", err)
	}
	if err := project.Save(recipePath, pkg.Recipe); err != nil {
		return project.Layout{}, fmt.Errorf("unpack: write recipe: %w", err)
	}
	return project.LayoutFor(recipePath)
}

// confirmAdoptRecipe asks whether to adopt the recipe a package carries as this
// directory's project recipe. --yes adopts; an interactive terminal is asked;
// anything else declines and says how to adopt deliberately.
//
// Declining is not a failure in itself — it withholds only the intent, which is
// the part that makes later runs execute steps someone else wrote. Whether the
// unpack can continue afterwards is the caller's question: today the only
// caller is reconstituteProject, where a declined recipe leaves no project for
// the content to land in.
func (a *App) confirmAdoptRecipe(cmd Command, snapshotPath string) (bool, error) {
	if a.AssumeYes {
		return true, nil
	}
	isTTY := a.isTTY
	if isTTY == nil {
		isTTY = defaultIsStdinTTY
	}
	w := cmd.ErrOrStderr()
	if !isTTY() {
		fmt.Fprintf(w, "Note: %s carries its own project recipe; this directory has none.\n"+
			"      Not adopting it. Re-run with --yes to use the packaged recipe as %s.\n",
			filepath.Base(snapshotPath), project.RecipeFileName)
		return false, nil
	}
	fmt.Fprintf(w, "\n%s carries its own project recipe, and this directory has none.\n"+
		"Adopting it means later `kapi run` invocations here execute the flows it declares.\n",
		filepath.Base(snapshotPath))
	return Confirm(cmd.InOrStdin(), w, fmt.Sprintf("Use it as %s? [Y/n] ", project.RecipeFileName))
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
		dst, err := containedJoin(absRoot, rel, "unpack: source member")
		if err != nil {
			return err
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
//
// The recipe's name is package-controlled, so it is accepted only as a single
// directory segment — a name carrying a separator or ".." would place the
// reconstituted project (and the block store, content memory and terms that
// follow it) outside the directory the snapshot was unpacked in. It falls back
// to the snapshot's own base name rather than failing: the folder is a
// convenience, not the payload.
func reconstitutedProjectPath(snapshotPath string, pkg *kpz.Package) string {
	dir := filepath.Dir(snapshotPath)
	name := format.Stem(snapshotPath)
	if pkg != nil && pkg.Recipe != nil && pkg.Recipe.Name != "" {
		if err := checkPathSegment(pkg.Recipe.Name, "unpack: recipe name"); err == nil {
			name = pkg.Recipe.Name
		}
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

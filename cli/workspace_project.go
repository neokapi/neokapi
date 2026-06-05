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
	pkg := &klz.Package{Generator: &klz.GeneratorInfo{ID: "kapi"}}

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

	if err := saveWorkspace(pkg, outPath); err != nil {
		return err
	}
	if a.Quiet {
		return nil
	}
	return outputPrint(cmd, "Packed project working state → "+outPath)
}

// runUnpack rehydrates a project's working state from a .klz snapshot into
// the local .kapi/ state dir, recreating the block store, TM, and termbase.
// A workspace .klz (one carrying a Recipe) instead rebuilds its shadow cache.
func (a *App) runUnpack(cmd *cobra.Command, snapshotPath string) error {
	if pkg, err := loadWorkspace(snapshotPath); err == nil && pkg.Recipe != nil {
		return a.unpackKlz(cmd.Context(), snapshotPath)
	}
	projectPath, err := RequireProjectPath(cmd)
	if err != nil {
		return err
	}
	layout, err := project.LayoutFor(projectPath)
	if err != nil {
		return err
	}
	if err := project.EnsureLayout(layout); err != nil {
		return err
	}
	pkg, err := loadWorkspace(snapshotPath)
	if err != nil {
		return err
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

	if a.Quiet {
		return nil
	}
	return outputPrint(cmd, fmt.Sprintf("Unpacked %s → %s", snapshotPath, layout.StateDir))
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

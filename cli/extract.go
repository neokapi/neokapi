package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/neokapi/neokapi/core/formats/exec"
	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/core/klz"
	"github.com/neokapi/neokapi/core/project"
	"github.com/spf13/cobra"
)

// NewExtractCmd returns `kapi extract` — the project-level entry
// point for source → .klz extraction. Walks every archive-declared
// collection in a .kapi project, runs the FormatSpec declared on
// each item, and packs the resulting blocks into each collection's
// .klz.
//
// Today the only content-generating format supported via this
// command is `exec` (see core/formats/exec). Built-in file-reader
// formats (markdown, xliff, …) still go through `kapi run` flows.
func (a *App) NewExtractCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "extract",
		Short:   "Run declared extractors into each collection's .klz",
		GroupID: "content",
		Long: `kapi extract -p project.kapi looks at every content collection
that declares an archive: path, runs the format's extractor
(format: name: exec with a command, today), and packs the streamed
blocks into the collection's archive.

To override the argv per run without editing the project file,
use KAPI_EXEC_OVERRIDE=<command> — useful for CI where the
canonical command differs from local invocation.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, _ := cmd.Flags().GetString("project")
			if projectPath == "" {
				return errors.New("required flag: --project / -p <project.kapi>")
			}
			timeout, _ := cmd.Flags().GetDuration("timeout")
			return runExtract(cmd.Context(), cmd.OutOrStdout(), projectPath, timeout)
		},
	}
	cmd.Flags().StringP("project", "p", "", "path to .kapi project file")
	cmd.Flags().Duration("timeout", 5*time.Minute, "max runtime per extractor subprocess")
	return cmd
}

// NewPackCmd returns `kapi pack` — reads NDJSON block records from
// stdin or a directory of .klf files and writes a single .klz
// archive. The standalone counterpart to `kapi extract -p` for
// pipelines that produce streams directly (no .kapi required).
//
//	vp kapi-react extract --stream | kapi pack --out i18n/ui.klz
//	kapi pack --in i18n/klf/ --out i18n/ui.klz
func (a *App) NewPackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "pack",
		Short:   "Pack NDJSON blocks or a directory of .klf files into a .klz",
		GroupID: "content",
		RunE: func(cmd *cobra.Command, args []string) error {
			outPath, _ := cmd.Flags().GetString("out")
			if outPath == "" {
				return errors.New("required flag: --out <archive.klz>")
			}
			inDir, _ := cmd.Flags().GetString("in")
			return runPack(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), inDir, outPath)
		},
	}
	cmd.Flags().String("out", "", "output archive path (.klz)")
	cmd.Flags().String("in", "", "directory of .klf files (omit to read NDJSON from stdin)")
	return cmd
}

// ExtractPlan is one batch the extract command will execute. Built
// from the declared collections so callers (kapi-desktop, tests)
// can inspect the plan before spawning subprocesses.
type ExtractPlan struct {
	Collection string
	Archive    string
	// Command is the argv declared via `format: exec` config.command.
	Command []string
	// Files matched for this collection (project-relative paths).
	Files []string
}

// PlanExtract reads the .kapi project and produces one ExtractPlan
// per archive-declared collection whose items use `format: exec`.
// Collections without an archive, or items whose format isn't exec,
// are ignored — they're out of scope for this command.
func PlanExtract(projectPath string) ([]ExtractPlan, error) {
	proj, err := project.Load(projectPath)
	if err != nil {
		return nil, fmt.Errorf("load project: %w", err)
	}
	projDir := filepath.Dir(projectPath)

	var plans []ExtractPlan
	for i := range proj.Content {
		coll := &proj.Content[i]
		if coll.Archive == "" {
			continue
		}

		command, files, err := collectExecInputs(projDir, coll)
		if err != nil {
			return plans, fmt.Errorf("collection %q: %w", collectionLabel(coll), err)
		}
		if command == nil {
			continue
		}
		plans = append(plans, ExtractPlan{
			Collection: collectionLabel(coll),
			Archive:    coll.Archive,
			Command:    command,
			Files:      files,
		})
	}
	return plans, nil
}

// collectExecInputs scans a collection's items and returns the
// single `exec` command argv + aggregated matched files, or nil
// when the collection has no exec items. Returns an error if the
// collection mixes exec + non-exec formats (ambiguous — keep it
// simple: one format per archive-declared collection).
func collectExecInputs(projDir string, coll *project.ContentCollection) ([]string, []string, error) {
	var command []string
	var files []string
	for _, item := range coll.EffectiveItems() {
		matches, err := globMatches(projDir, item.Path)
		if err != nil {
			return nil, nil, fmt.Errorf("glob %q: %w", item.Path, err)
		}
		if item.Format == nil || item.Format.Name != exec.FormatName {
			// Not an exec item — contributes no files to this
			// extraction. A future `kapi extract` pass can handle
			// file-reader formats via the standard pipeline.
			continue
		}
		cmdString, _ := item.Format.Config["command"].(string)
		if cmdString == "" {
			return nil, nil, fmt.Errorf("item %q declares format: exec but no config.command", item.Path)
		}
		argv := splitCommand(cmdString)
		if override := os.Getenv("KAPI_EXEC_OVERRIDE"); override != "" {
			argv = splitCommand(override)
		}
		if command == nil {
			command = argv
		} else if !stringSliceEq(command, argv) {
			return nil, nil, errors.New("multiple items declare different exec commands; split into separate collections")
		}
		files = append(files, matches...)
	}
	sort.Strings(files)
	return command, files, nil
}

func runExtract(ctx context.Context, w io.Writer, projectPath string, timeout time.Duration) error {
	plans, err := PlanExtract(projectPath)
	if err != nil {
		return err
	}
	if len(plans) == 0 {
		fmt.Fprintf(w, "%s: no exec-format collections to extract.\n", projectPath)
		return nil
	}
	projDir := filepath.Dir(projectPath)

	for _, plan := range plans {
		if len(plan.Files) == 0 {
			fmt.Fprintf(w, "  %s (no matching files)\n", plan.Collection)
			continue
		}
		fmt.Fprintf(w, "  %s → %s (%d file(s))\n", plan.Collection, plan.Command[0], len(plan.Files))

		records, err := exec.Run(ctx, exec.Spec{
			Exec:    plan.Command,
			WorkDir: projDir,
			Timeout: timeout,
		}, plan.Files)
		if err != nil {
			return fmt.Errorf("extract %s: %w", plan.Collection, err)
		}

		archivePath := filepath.Join(projDir, plan.Archive)
		blocks, total := groupBlocks(records)
		if err := writeArchiveFromBlocks(archivePath, blocks); err != nil {
			return fmt.Errorf("write archive %s: %w", archivePath, err)
		}
		fmt.Fprintf(w, "  %s ← %d block(s) across %d document(s)\n",
			plan.Archive, total, len(blocks))
	}
	return nil
}

func runPack(_ context.Context, r io.Reader, w io.Writer, inDir, outPath string) error {
	var records []exec.Record
	if inDir != "" {
		recs, err := readKLFDir(inDir)
		if err != nil {
			return fmt.Errorf("read klf dir: %w", err)
		}
		records = recs
	} else {
		recs, err := readNDJSON(r)
		if err != nil {
			return fmt.Errorf("read ndjson stdin: %w", err)
		}
		records = recs
	}

	blocks, total := groupBlocks(records)
	if err := writeArchiveFromBlocks(outPath, blocks); err != nil {
		return fmt.Errorf("write archive %s: %w", outPath, err)
	}
	fmt.Fprintf(w, "%s ← %d block(s) across %d document(s)\n", outPath, total, len(blocks))
	return nil
}

// readKLFDir loads every `*.klf` file under dir and converts its
// blocks into exec.Record for unified packing downstream.
func readKLFDir(dir string) ([]exec.Record, error) {
	var out []exec.Record
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".klf") {
			return nil
		}
		data, err := os.ReadFile(path) // #nosec G304 — caller-provided dir
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		file, err := klf.Unmarshal(data)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		for _, doc := range file.Documents {
			for _, block := range doc.Blocks {
				out = append(out, exec.Record{
					Type:     "block",
					Document: doc.Path,
					Block:    block,
				})
			}
		}
		return nil
	})
	return out, err
}

// readNDJSON consumes NDJSON from r and returns every parsed record.
// Non-{ lines are skipped (matches the exec reader's noise-tolerance
// rules).
func readNDJSON(r io.Reader) ([]exec.Record, error) {
	var out []exec.Record
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		rec, ok, err := exec.DecodeLine(scanner.Bytes())
		if err != nil {
			return out, err
		}
		if ok {
			out = append(out, rec)
		}
	}
	return out, scanner.Err()
}

func groupBlocks(records []exec.Record) (map[string][]klf.Block, int) {
	blocks := map[string][]klf.Block{}
	total := 0
	for _, r := range records {
		if r.Type != "block" {
			continue
		}
		blocks[r.Document] = append(blocks[r.Document], r.Block)
		total++
	}
	return blocks, total
}

func writeArchiveFromBlocks(archivePath string, blocks map[string][]klf.Block) error {
	if err := os.MkdirAll(filepath.Dir(archivePath), 0o755); err != nil {
		return err
	}

	writer := klz.NewWriter(klz.WriterOptions{
		Generator: klz.ManifestGenerator{ID: "kapi", Version: "0.0.0"},
		Project:   klz.ManifestProject{ID: "kapi-extract", SourceLocale: "en"},
		Created:   time.Now().UTC().Format(time.RFC3339),
	})

	docPaths := make([]string, 0, len(blocks))
	for p := range blocks {
		docPaths = append(docPaths, p)
	}
	sort.Strings(docPaths)
	for _, docPath := range docPaths {
		file := &klf.File{
			SchemaVersion: klf.SchemaVersion,
			Kind:          klf.Kind,
			Generator:     klf.GeneratorInfo{ID: "kapi", Version: "0.0.0"},
			Project:       klf.ProjectInfo{ID: "kapi-extract", SourceLocale: "en"},
			Documents: []klf.Document{{
				ID:           docPath,
				DocumentType: klf.DocumentTypeJSX,
				Path:         docPath,
				Blocks:       blocks[docPath],
			}},
		}
		archivePart := fmt.Sprintf("documents/%s.klf", slugify(docPath))
		if err := writer.AddDocument(archivePart, file, nil); err != nil {
			return fmt.Errorf("add document %s: %w", docPath, err)
		}
	}

	f, err := os.Create(archivePath) // #nosec G304 — project-scoped path
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := writer.Write(f); err != nil {
		return err
	}
	return nil
}

// RunExtractInProcess runs the extract flow directly (used by
// kapi-desktop). Bounded at a 5-minute subprocess timeout.
func RunExtractInProcess(ctx context.Context, w io.Writer, projectPath string) error {
	return runExtract(ctx, w, projectPath, 5*time.Minute)
}

// globMatches expands one pattern relative to projDir. Returns
// project-relative paths (matching the existing content.items.path
// convention).
func globMatches(projDir, pattern string) ([]string, error) {
	abs := filepath.Join(projDir, pattern)
	matches, err := doublestar.FilepathGlob(abs, doublestar.WithFilesOnly())
	if err != nil {
		return nil, err
	}
	rel := make([]string, 0, len(matches))
	for _, m := range matches {
		r, err := filepath.Rel(projDir, m)
		if err != nil {
			return nil, err
		}
		rel = append(rel, filepath.ToSlash(r))
	}
	sort.Strings(rel)
	return rel, nil
}

// splitCommand tokenises a shell-ish command string: single- and
// double-quoted segments are preserved, everything else splits on
// whitespace. Not a full shell — backslash escapes and operators
// aren't supported — but matches the expectations of
// `command: "vp kapi-react extract --stream"` in a .kapi file.
func splitCommand(s string) []string {
	var out []string
	var cur strings.Builder
	inSingle, inDouble := false, false
	for _, r := range s {
		switch {
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case (r == ' ' || r == '\t') && !inSingle && !inDouble:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func stringSliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func slugify(path string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "_")
	return replacer.Replace(path)
}

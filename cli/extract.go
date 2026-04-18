package cli

import (
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

	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/core/klz"
	"github.com/neokapi/neokapi/core/plugin/extractor"
	"github.com/neokapi/neokapi/core/project"
	"github.com/spf13/cobra"
)

// NewExtractCmd returns `kapi extract` — the unified entry point for
// source → .klz extraction. Walks every archive-declared collection
// in a .kapi project, dispatches each file to a registered extractor
// (based on extension or explicit `extractor:` override), and writes
// the resulting blocks into each collection's declared archive.
func (a *App) NewExtractCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "extract",
		Short:   "Extract translatable content into the project's .klz archives",
		GroupID: "content",
		Long: `kapi extract -p project.kapi walks every content collection
that declares an archive: path and populates it by dispatching
matched files to the right extractor:

  1. The collection's explicit extractor: stanza wins.
  2. Otherwise, a kapi-plugin.json descriptor in the project's
     node_modules tree that claims the file's extension.
  3. Otherwise, the extension is skipped with a warning — no
     built-in extractor exists for source files yet (that path is
     reserved for pipeline-level format filters that read AND
     write source).

The extractor protocol is NUL-separated paths on stdin, NDJSON
block records on stdout. See core/plugin/extractor for details.`,
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

// ExtractPlan is one batch the extractor runner will execute. Built
// from the declared collections so callers can inspect the plan
// before we spawn subprocesses.
type ExtractPlan struct {
	Collection string
	Archive    string
	// Extractor identifies the extractor that will handle each
	// extension group. Multiple extensions can map to the same
	// extractor (eg .tsx + .jsx → kapi-react).
	Extractor extractor.Spec
	// PackageName is filled in for discovered extractors so progress
	// + error messages reference the package.
	PackageName string
	// Files matched for this extractor, relative to the project dir.
	Files []string
}

// PlanExtract resolves every (collection, extractor) pair that
// kapi extract will run for a project. Returns an empty plan when
// the project has no archive-declared collections.
func PlanExtract(projectPath string) ([]ExtractPlan, error) {
	proj, err := project.Load(projectPath)
	if err != nil {
		return nil, fmt.Errorf("load project: %w", err)
	}
	projDir := filepath.Dir(projectPath)

	discovered, err := extractor.Discover(projDir)
	if err != nil {
		return nil, fmt.Errorf("discover plugins: %w", err)
	}
	byExt := extractor.ByExtension(discovered)

	var plans []ExtractPlan
	for i := range proj.Content {
		coll := &proj.Content[i]
		if coll.Archive == "" {
			continue
		}

		// Group matched files by extension.
		byExtForColl := map[string][]string{}
		for _, item := range coll.EffectiveItems() {
			matches, err := globMatches(projDir, item.Path)
			if err != nil {
				return nil, fmt.Errorf("glob %q: %w", item.Path, err)
			}
			for _, m := range matches {
				ext := strings.ToLower(filepath.Ext(m))
				byExtForColl[ext] = append(byExtForColl[ext], m)
			}
		}
		if len(byExtForColl) == 0 {
			continue
		}

		// An explicit extractor: stanza handles every file in the
		// collection, regardless of extension.
		if coll.Extractor != nil && len(coll.Extractor.Exec) > 0 {
			var all []string
			for _, fs := range byExtForColl {
				all = append(all, fs...)
			}
			sort.Strings(all)
			plans = append(plans, ExtractPlan{
				Collection: collectionLabel(coll),
				Archive:    coll.Archive,
				Extractor:  extractor.Spec{Exec: coll.Extractor.Exec, WorkDir: projDir},
				Files:      all,
			})
			continue
		}

		// Otherwise, one plan per extension that has a discovered
		// extractor. Extensions without coverage are reported in the
		// error payload below.
		var unhandled []string
		for ext, fs := range byExtForColl {
			disc, ok := byExt[ext]
			if !ok || disc.Descriptor.Extract == nil {
				unhandled = append(unhandled, ext)
				continue
			}
			sort.Strings(fs)
			plans = append(plans, ExtractPlan{
				Collection:  collectionLabel(coll),
				Archive:     coll.Archive,
				Extractor:   extractor.Spec{Exec: disc.Descriptor.Extract.Exec, WorkDir: projDir},
				PackageName: disc.PackageName,
				Files:       fs,
			})
		}
		if len(unhandled) > 0 {
			sort.Strings(unhandled)
			return plans, fmt.Errorf(
				"collection %q has %d file(s) with no registered extractor: %s",
				collectionLabel(coll), countFiles(byExtForColl, unhandled),
				strings.Join(unhandled, ", "))
		}
	}
	return plans, nil
}

// RunExtractInProcess is the exported form of the kapi extract CLI
// logic — useful for callers (kapi-desktop) that want to run the
// same orchestration without shelling out to the binary. Uses the
// default 5-minute per-subprocess timeout; pass a context with a
// shorter deadline to bound externally.
func RunExtractInProcess(ctx context.Context, w io.Writer, projectPath string) error {
	return runExtract(ctx, w, projectPath, 5*time.Minute)
}

func runExtract(ctx context.Context, w io.Writer, projectPath string, timeout time.Duration) error {
	plans, err := PlanExtract(projectPath)
	if err != nil {
		return err
	}
	if len(plans) == 0 {
		fmt.Fprintf(w, "%s: no archive-declared collections to extract.\n", projectPath)
		return nil
	}
	projDir := filepath.Dir(projectPath)

	// Group plans by archive so we aggregate all blocks from every
	// extractor that feeds the same .klz before writing.
	byArchive := map[string][]ExtractPlan{}
	for _, p := range plans {
		byArchive[p.Archive] = append(byArchive[p.Archive], p)
	}

	archives := make([]string, 0, len(byArchive))
	for a := range byArchive {
		archives = append(archives, a)
	}
	sort.Strings(archives)

	for _, archiveRel := range archives {
		archivePath := filepath.Join(projDir, archiveRel)
		blocks := map[string][]klf.Block{} // document path → blocks
		total := 0
		for _, plan := range byArchive[archiveRel] {
			if len(plan.Files) == 0 {
				continue
			}
			label := plan.PackageName
			if label == "" && len(plan.Extractor.Exec) > 0 {
				label = plan.Extractor.Exec[0]
			}
			fmt.Fprintf(w, "  %s → %s (%d file(s))\n", plan.Collection, label, len(plan.Files))

			spec := plan.Extractor
			if spec.Timeout == 0 {
				spec.Timeout = timeout
			}
			records, err := extractor.Run(ctx, spec, plan.Files)
			if err != nil {
				return fmt.Errorf("extract %s via %s: %w", plan.Collection, label, err)
			}
			for _, r := range records {
				if r.Type != "block" {
					continue
				}
				blocks[r.Document] = append(blocks[r.Document], r.Block)
				total++
			}
		}
		if err := writeArchive(archivePath, blocks); err != nil {
			return fmt.Errorf("write archive %s: %w", archivePath, err)
		}
		fmt.Fprintf(w, "  %s ← %d block(s) across %d document(s)\n",
			archiveRel, total, len(blocks))
	}
	return nil
}

// globMatches expands one pattern relative to projDir. Returns
// project-relative paths (matching the existing content.items.path
// convention).
func globMatches(projDir, pattern string) ([]string, error) {
	abs := filepath.Join(projDir, pattern)
	// doublestar is already a transitive dep of kapi; using the
	// same globber as the content matcher keeps behaviour aligned.
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

func writeArchive(archivePath string, blocks map[string][]klf.Block) error {
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

func slugify(path string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "_")
	return replacer.Replace(path)
}

func countFiles(byExt map[string][]string, exts []string) int {
	n := 0
	for _, e := range exts {
		n += len(byExt[e])
	}
	return n
}


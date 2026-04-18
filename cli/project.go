package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/neokapi/neokapi/core/klf"
	"github.com/neokapi/neokapi/core/klz"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/spf13/cobra"
)

// NewStatusCmd returns `kapi status` — reports per-collection archive
// state for a .kapi project: locale coverage, block counts, whether
// an archive is missing from disk.
func (a *App) NewStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "status",
		Short:   "Show translation state for a .kapi project",
		GroupID: "content",
		Long: `Inspect a .kapi project and report on every ContentCollection
that declares an archive: field. For each declared .klz, show:

  - total source block count
  - per-locale coverage (translated blocks / total)
  - whether the archive is present on disk

Collections without archive: are listed with a reminder that they use
file-based flows and have no .klz state to inspect.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, _ := cmd.Flags().GetString("project")
			if projectPath == "" {
				return errors.New("required flag: --project / -p <project.kapi>")
			}
			return runStatus(cmd.OutOrStdout(), projectPath)
		},
	}
	cmd.Flags().StringP("project", "p", "", "path to .kapi project file")
	return cmd
}

// CollectionStatus is the per-collection summary surfaced by
// `kapi status` + reused by any consumer (kapi-desktop, CI hooks)
// that wants the same data.
type CollectionStatus struct {
	// Name of the collection (or path, for bare entries).
	Name string
	// Archive path as declared in the .kapi file. Empty when the
	// collection uses file-based flows.
	Archive string
	// ArchiveExists reports whether Archive resolves to a real file.
	// False when the declared path is absent; BlockCount + Coverage
	// are zero in that case.
	ArchiveExists bool
	// Total source blocks in the archive.
	BlockCount int
	// Coverage maps each declared target locale → translated-block
	// count. A locale that has no entry is absent entirely.
	Coverage map[model.LocaleID]int
	// TargetLanguages as resolved from project defaults + collection
	// overrides. The status report compares this against Coverage to
	// flag missing locales.
	TargetLanguages []model.LocaleID
}

// ProjectStatus bundles one CollectionStatus per ContentCollection in
// the project. Returned by CollectProjectStatus so UI layers can
// render the same data kapi status prints to stdout.
type ProjectStatus struct {
	ProjectPath string
	ProjectName string
	Collections []CollectionStatus
}

// CollectProjectStatus walks a .kapi file, opens each declared
// archive, and returns structured per-collection status. Callers
// handle pretty-printing or serialisation on their own.
func CollectProjectStatus(projectPath string) (*ProjectStatus, error) {
	proj, err := project.Load(projectPath)
	if err != nil {
		return nil, fmt.Errorf("load project: %w", err)
	}
	projDir := filepath.Dir(projectPath)

	out := &ProjectStatus{
		ProjectPath: projectPath,
		ProjectName: proj.Name,
	}

	for i := range proj.Content {
		coll := &proj.Content[i]
		cs := CollectionStatus{
			Name:            collectionLabel(coll),
			Archive:         coll.Archive,
			TargetLanguages: resolveTargetLocales(coll, proj.Defaults),
		}
		if coll.Archive == "" {
			out.Collections = append(out.Collections, cs)
			continue
		}

		archivePath := filepath.Join(projDir, coll.Archive)
		info, err := os.Stat(archivePath)
		if err != nil || info.IsDir() {
			out.Collections = append(out.Collections, cs)
			continue
		}
		cs.ArchiveExists = true

		cs.BlockCount, cs.Coverage, err = archiveCoverage(archivePath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", archivePath, err)
		}
		out.Collections = append(out.Collections, cs)
	}

	return out, nil
}

func runStatus(w io.Writer, projectPath string) error {
	status, err := CollectProjectStatus(projectPath)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "%s", status.ProjectPath)
	if status.ProjectName != "" {
		fmt.Fprintf(w, " (%s)", status.ProjectName)
	}
	fmt.Fprintln(w)

	if len(status.Collections) == 0 {
		fmt.Fprintln(w, "  (no content collections)")
		return nil
	}

	for _, cs := range status.Collections {
		fmt.Fprintf(w, "\n  %s", cs.Name)
		if cs.Archive == "" {
			fmt.Fprintf(w, "\n    (no archive — file-based flow)\n")
			continue
		}
		fmt.Fprintf(w, " → %s", cs.Archive)
		if !cs.ArchiveExists {
			fmt.Fprintln(w, " [MISSING]")
			fmt.Fprintln(w, "    run `kapi run extract -p "+projectPath+"` to create it")
			continue
		}
		fmt.Fprintf(w, "\n    %d blocks\n", cs.BlockCount)

		locales := append([]model.LocaleID(nil), cs.TargetLanguages...)
		for loc := range cs.Coverage {
			if !localeIn(locales, loc) {
				locales = append(locales, loc)
			}
		}
		sort.Slice(locales, func(i, j int) bool {
			return string(locales[i]) < string(locales[j])
		})
		for _, loc := range locales {
			translated := cs.Coverage[loc]
			marker := fmt.Sprintf("%d/%d translated", translated, cs.BlockCount)
			switch {
			case translated == 0:
				marker = "not translated"
			case translated == cs.BlockCount:
				marker = fmt.Sprintf("%d/%d translated (complete)", translated, cs.BlockCount)
			}
			fmt.Fprintf(w, "    %-8s %s\n", string(loc)+":", marker)
		}
	}
	return nil
}

func collectionLabel(coll *project.ContentCollection) string {
	if coll.Name != "" {
		return coll.Name
	}
	return coll.Path
}

func resolveTargetLocales(coll *project.ContentCollection, defaults project.Defaults) []model.LocaleID {
	if len(coll.TargetLanguages) > 0 {
		return append([]model.LocaleID(nil), coll.TargetLanguages...)
	}
	return append([]model.LocaleID(nil), defaults.TargetLanguages...)
}

// archiveCoverage opens a .klz and returns the total block count +
// how many blocks have targets in each locale.
func archiveCoverage(archivePath string) (int, map[model.LocaleID]int, error) {
	data, err := os.ReadFile(archivePath)
	if err != nil {
		return 0, nil, err
	}
	reader, err := klz.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return 0, nil, fmt.Errorf("open klz: %w", err)
	}
	defer reader.Close()

	docs, err := reader.Documents()
	if err != nil {
		return 0, nil, fmt.Errorf("read documents: %w", err)
	}
	total := 0
	coverage := make(map[model.LocaleID]int)
	for _, file := range docs {
		for _, doc := range file.Documents {
			for _, block := range doc.Blocks {
				if !block.Translatable {
					continue
				}
				total++
				for loc, runs := range block.Targets {
					if hasContent(runs) {
						coverage[model.LocaleID(loc)]++
					}
				}
			}
		}
	}
	return total, coverage, nil
}

func hasContent(runs []klf.Run) bool {
	for _, r := range runs {
		if r.Text != nil && r.Text.Text != "" {
			return true
		}
		if r.Ph != nil || r.PcOpen != nil || r.PcClose != nil || r.Sub != nil || r.Plural != nil || r.Select != nil {
			return true
		}
	}
	return false
}

func localeIn(locales []model.LocaleID, target model.LocaleID) bool {
	for _, l := range locales {
		if l == target {
			return true
		}
	}
	return false
}

// NewSyncCmd returns `kapi sync` — orchestrates translation top-ups
// for every ContentCollection in a .kapi project that declares an
// archive. For each (collection, target-locale) pair whose archive
// lacks complete coverage, sync runs the named tool against the
// archive with the missing locale as --target-lang. The writer's
// locale-additive behaviour accumulates the result in place.
//
// Extraction itself (JSX → .klz) is out of scope: that's kapi-react
// today. Sync picks up where a freshly-extracted .klz leaves off.
func (a *App) NewSyncCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "sync",
		Short:   "Bring a .kapi project's translations up to date",
		GroupID: "content",
		Long: `Iterate every ContentCollection that declares an archive: field.
For each archive + declared target locale where coverage is
incomplete, run the tool named by --tool against that archive with
--target-lang set to the missing locale.

Example:

  kapi sync -p project.kapi --tool ai-translate
  kapi sync -p project.kapi --tool pseudo-translate
  kapi sync -p project.kapi --dry-run   # print the plan and exit

--dry-run prints the command sequence without executing it. Useful
for CI pre-checks and for reviewing what sync would touch.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			projectPath, _ := cmd.Flags().GetString("project")
			if projectPath == "" {
				return errors.New("required flag: --project / -p <project.kapi>")
			}
			toolName, _ := cmd.Flags().GetString("tool")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			return runSync(cmd.Context(), cmd.OutOrStdout(), projectPath, toolName, dryRun)
		},
	}
	cmd.Flags().StringP("project", "p", "", "path to .kapi project file")
	cmd.Flags().String("tool", "ai-translate", "translation tool to invoke per missing locale")
	cmd.Flags().Bool("dry-run", false, "print the plan without running it")
	return cmd
}

// SyncStep is one archive + target-locale pair the sync orchestrator
// plans to fill in.
type SyncStep struct {
	Collection string
	Archive    string
	Locale     model.LocaleID
	// Reason is a human-readable description of why this step is in
	// the plan: "not translated", "5/1007 translated", etc.
	Reason string
}

// PlanSync walks a .kapi project's collections and returns every
// (archive, locale) pair that needs translation according to the
// per-locale coverage computed from the archive on disk.
func PlanSync(projectPath string) (*ProjectStatus, []SyncStep, error) {
	status, err := CollectProjectStatus(projectPath)
	if err != nil {
		return nil, nil, err
	}
	var plan []SyncStep
	for _, cs := range status.Collections {
		if cs.Archive == "" || !cs.ArchiveExists {
			continue
		}
		for _, loc := range cs.TargetLanguages {
			translated := cs.Coverage[loc]
			if translated >= cs.BlockCount {
				continue
			}
			reason := "not translated"
			if translated > 0 {
				reason = fmt.Sprintf("%d/%d translated", translated, cs.BlockCount)
			}
			plan = append(plan, SyncStep{
				Collection: cs.Name,
				Archive:    cs.Archive,
				Locale:     loc,
				Reason:     reason,
			})
		}
	}
	return status, plan, nil
}

func runSync(ctx context.Context, w io.Writer, projectPath, toolName string, dryRun bool) error {
	status, plan, err := PlanSync(projectPath)
	if err != nil {
		return err
	}
	projDir := filepath.Dir(projectPath)

	if len(plan) == 0 {
		fmt.Fprintf(w, "%s: all declared archives are fully translated — nothing to do.\n", status.ProjectPath)
		return nil
	}

	fmt.Fprintf(w, "%s: %d step(s) to bring translations up to date\n", status.ProjectPath, len(plan))
	for _, step := range plan {
		archivePath := filepath.Join(projDir, step.Archive)
		fmt.Fprintf(w, "  %s [%s] %s → kapi %s %s --target-lang %s\n",
			step.Collection, step.Locale, step.Reason, toolName, archivePath, step.Locale)
	}
	if dryRun {
		fmt.Fprintln(w, "\n--dry-run: not executing. Re-run without --dry-run to apply.")
		return nil
	}

	binary, err := os.Executable()
	if err != nil || binary == "" {
		binary = "kapi"
	}
	for _, step := range plan {
		archivePath := filepath.Join(projDir, step.Archive)
		cmd := exec.CommandContext(ctx, binary, toolName, archivePath, "--target-lang", string(step.Locale)) // #nosec G204 — binary path is os.Executable, args are project-derived
		cmd.Stdout = w
		cmd.Stderr = w
		fmt.Fprintf(w, "\n→ %s %s %s --target-lang %s\n", binary, toolName, archivePath, step.Locale)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s %s %s --target-lang %s: %w", binary, toolName, archivePath, step.Locale, err)
		}
	}
	return nil
}

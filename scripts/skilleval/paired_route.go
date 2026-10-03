package main

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// The write routes an attempt can take to change a task's files.
const (
	// pairedRouteContract is kapi's change contract: kapi apply, ksed -i or
	// the apply_edits tool.
	pairedRouteContract = "contract"
	// pairedRouteMerge is a bilingual file filled in and merged with kapi
	// merge.
	pairedRouteMerge = "merge"
	// pairedRouteNative is a write to a task file with the host's own tools:
	// an edit or write tool, a patch, or a shell command that rewrites it.
	pairedRouteNative = "native"
	// pairedRouteNone is an attempt that wrote nothing by any of them.
	pairedRouteNone = "none"
)

var (
	// pairedContractCommand is a kapi command that writes through the
	// change contract. --dry-run, --print-ops and --schema write nothing.
	pairedContractCommand = regexp.MustCompile(`(?:^|[\s;&|(/])(?:kapi(?:-files)?\s+(?:apply|ksed\s+[^|;&]*-i)|ksed\s+[^|;&]*-i)\b`)
	pairedPreviewFlag     = regexp.MustCompile(`--(?:dry-run|print-ops|schema)\b`)
	pairedMergeCommand    = regexp.MustCompile(`(?:^|[\s;&|(/])kapi(?:-files)?\s+merge\b`)
	// pairedShellWrite is a shell construct that rewrites a file: an
	// in-place edit, a redirection, tee, a patch, or a script that opens a
	// file for writing.
	pairedShellWrite = regexp.MustCompile(`\bsed\s+(?:-[a-zA-Z]+\s+)*-[a-zA-Z]*i|\bperl\s+-[a-zA-Z]*i|\btee\b|(?:^|[^0-9&>-])>>?\s*[^\s&|>]|\bapply_patch\b|\bpatch\s|write_text\(|writeFileSync|open\([^)]*['"][wa]`)
)

// pairedWritableFiles lists the files a task may change or add, which a native
// write names.
func pairedWritableFiles(task string) []string {
	t, err := findPairedTask(task)
	if err != nil {
		return nil
	}
	return slices.Concat(t.spec.Editable, t.spec.Creates)
}

// noteRoute records the write route one tool call takes, if it writes.
func (o *pairedObserver) noteRoute(tool string, input map[string]any) {
	route := pairedWriteRouteOf(tool, input, o.taskFiles, o.launch.Workspace)
	if route != "" {
		o.result.WriteRoutes = pairedUnique(o.result.WriteRoutes, route)
	}
}

// pairedWriteRouteOf is the write route of one tool call, or "" for a call
// that writes no task file.
func pairedWriteRouteOf(tool string, input map[string]any, files []string, workspace string) string {
	switch {
	case tool == "Bash" || tool == "shell":
		command := pairedString(input, "command")
		switch {
		case pairedMergeCommand.MatchString(command):
			return pairedRouteMerge
		case pairedContractCommand.MatchString(command) && !pairedPreviewFlag.MatchString(command):
			return pairedRouteContract
		case pairedNamesTaskFile(command, files) && pairedShellWrite.MatchString(command):
			return pairedRouteNative
		}
	case strings.HasPrefix(tool, "mcp__") && strings.HasSuffix(tool, "__apply_edits"):
		if mode := pairedString(input, "mode"); mode == "preview" {
			return ""
		}
		return pairedRouteContract
	case tool == "Edit" || tool == "Write" || tool == "MultiEdit" || tool == "NotebookEdit":
		if pairedIsTaskFile(pairedString(input, "file_path"), files, workspace) {
			return pairedRouteNative
		}
	case tool == "file_change":
		for _, raw := range pairedList(input["changes"]) {
			change, _ := raw.(map[string]any)
			if pairedIsTaskFile(pairedString(change, "path"), files, workspace) {
				return pairedRouteNative
			}
		}
	}
	return ""
}

// pairedNamesTaskFile reports whether a command names one of the task's
// files.
func pairedNamesTaskFile(command string, files []string) bool {
	for _, f := range files {
		if strings.Contains(command, f) || strings.Contains(command, filepath.Base(f)) {
			return true
		}
	}
	return false
}

// pairedIsTaskFile reports whether a path, absolute or relative to the
// workspace, is one of the task's files.
func pairedIsTaskFile(path string, files []string, workspace string) bool {
	if path == "" {
		return false
	}
	if filepath.IsAbs(path) && workspace != "" {
		rel, err := filepath.Rel(workspace, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			return false
		}
		path = rel
	}
	return slices.Contains(files, filepath.ToSlash(filepath.Clean(path)))
}

// pairedWriteRoute is an attempt's route as the summary groups it: the one
// route it took, the routes it combined joined with "+", or none.
func pairedWriteRoute(routes []string) string {
	if len(routes) == 0 {
		return pairedRouteNone
	}
	sorted := slices.Clone(routes)
	slices.Sort(sorted)
	return strings.Join(sorted, "+")
}

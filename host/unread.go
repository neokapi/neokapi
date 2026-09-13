package host

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/registry"
)

// UnreadSet collects the declared content a check over the project never
// opened, because no reader for its format is installed. A plugin can supply a
// format, so a recipe that reads on a machine with the plugin cannot be read on
// one without it. A check over the project's declared content checks what it
// can read and reports the rest by name: a warning for each file, a line on
// stderr for each format, and a did_not_run verdict for a check that read
// nothing at all.
//
// A nil set belongs to a check over files the user named, or read under a
// format the user named. A missing reader is an error there, and Skip leaves it
// to the caller.
type UnreadSet struct {
	// formats maps each unread file to the format declared for it.
	formats map[string]string
	// files are the unread files in the order the check met them.
	files []string
}

// NewUnreadSet returns an empty set for a check over the project's declared
// content, such as the desktop Checks panel's.
func NewUnreadSet() *UnreadSet {
	return &UnreadSet{formats: map[string]string{}}
}

// newUnreadSet returns the set a check over the project's declared content
// collects into, or nil when --format names the format every file is read
// under.
func (a *App) newUnreadSet() *UnreadSet {
	if a.FormatFlag != "" {
		return nil
	}
	return NewUnreadSet()
}

// Skip reports whether err says that no reader for the file's format is
// installed, and records the file when it does. Such a file was never opened,
// so nothing in it counts as checked. Any other error means the file was opened
// and is broken, and the caller returns it.
func (u *UnreadSet) Skip(err error, file, format string) bool {
	if u == nil || !errors.Is(err, registry.ErrUnknownFormat) {
		return false
	}
	if _, seen := u.formats[file]; !seen {
		u.formats[file] = format
		u.files = append(u.files, file)
	}
	return true
}

// skipUnit is Skip for a verify unit. It names the unit's source file relative
// to the project root, the file whose format has no reader, and adds it once to
// skipped when a gate keeps its own list.
func (u *UnreadSet) skipUnit(skipped *[]string, err error, root string, unit VerifyUnit) bool {
	file := relativeToRoot(root, unit.SourcePath)
	if !u.Skip(err, file, unit.SourceFormat) {
		return false
	}
	if skipped != nil && !slices.Contains(*skipped, file) {
		*skipped = append(*skipped, file)
	}
	return true
}

// unitsSkipped returns the source files of units the set records, each once,
// and whether it records every unit, so that nothing among them was read.
func (u *UnreadSet) unitsSkipped(root string, units []VerifyUnit) (skipped []string, readNothing bool) {
	if u == nil {
		return nil, false
	}
	readNothing = true
	for _, unit := range units {
		file := relativeToRoot(root, unit.SourcePath)
		if _, ok := u.formats[file]; !ok {
			readNothing = false
			continue
		}
		if !slices.Contains(skipped, file) {
			skipped = append(skipped, file)
		}
	}
	return skipped, readNothing
}

// warnings returns a WarningFormatNoReader warning for each unread file.
func (u *UnreadSet) warnings() []check.Warning {
	if u == nil {
		return nil
	}
	out := make([]check.Warning, 0, len(u.files))
	for _, file := range u.files {
		format := u.formats[file]
		out = append(out, check.Warning{
			Code:   check.WarningFormatNoReader,
			Source: file,
			Message: fmt.Sprintf("no reader for format %q is installed, so %s was not checked; "+
				"install the plugin that supplies it (kapi plugins install %s)", format, file, format),
		})
	}
	return out
}

// reasons says of each file that it was not checked, and why.
func (u *UnreadSet) reasons(files []string) []string {
	out := make([]string, 0, len(files))
	for _, file := range files {
		out = append(out, fmt.Sprintf("%s was not checked: no reader for format %q is installed", file, u.formats[file]))
	}
	return out
}

// Report adds the unread files to a check report as warnings. A report that
// checked no block did not run, and when content in its scope went unread the
// cause is that content was not checked, never that there was nothing to check.
func (u *UnreadSet) Report(r *check.Report) {
	if u == nil || len(u.files) == 0 {
		return
	}
	r.Warnings = check.MergeWarnings(r.Warnings, u.warnings())
	if r.Verdict == check.VerdictDidNotRun && r.DidNotRunCause == check.CauseNothingToCheck {
		r.DidNotRunCause = check.CauseContentNotChecked
		r.DidNotRun = u.reasons(u.files)
	}
}

// settle decides a gate that skipped files. When the gate read nothing and
// nothing it saw failed it, content in its scope went unchecked, so it did not
// run. A gate that failed on what it could see keeps its failure, and one that
// read some content is decided by that content.
func (u *UnreadSet) settle(g *verifyGateResult, skipped []string, readNothing bool) {
	if u == nil || len(skipped) == 0 || !readNothing || !g.Pass {
		return
	}
	g.Pass = false
	g.Verdict = check.VerdictDidNotRun
	g.DidNotRunCause = check.CauseContentNotChecked
	g.DidNotRun = u.reasons(skipped)
}

// warn prints one line on stderr for each format with no reader, naming the
// files declared in it. A report that leaves a collection out reads like one
// that checked it and found nothing, and this is the line a person at a
// terminal sees.
func (u *UnreadSet) warn(a *App, cmd Command) {
	if u == nil || len(u.files) == 0 || a.Quiet {
		return
	}
	byFormat := map[string][]string{}
	for _, file := range u.files {
		byFormat[u.formats[file]] = append(byFormat[u.formats[file]], file)
	}
	for _, format := range slices.Sorted(maps.Keys(byFormat)) {
		files := byFormat[format]
		named := strings.Join(files[:min(len(files), 3)], ", ")
		if len(files) > 3 {
			named += fmt.Sprintf(" and %d more", len(files)-3)
		}
		fmt.Fprintf(cmd.ErrOrStderr(),
			"warning: no reader for format %q, so %s was not checked; install the plugin that supplies it (kapi plugins install %s)\n",
			format, named, format)
	}
}

package host

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host/pluginhost"
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
	// readers maps each unread file to the reader it needs: its format's, or
	// the reader of its language's comments that a plugin supplies.
	readers map[string]missingReader
	// files are the unread files in the order the check met them.
	files []string
	// outcome is what happened to the content the set names, in the words of
	// the command that skipped it: "checked" for a check, "converged" for a
	// run, "priced" for a plan. Empty reads as "checked".
	outcome string
	// installed are the discovered plugins whose manifests name the plugin
	// that supplies a format, ahead of the formats kapi knows.
	installed []*pluginhost.Plugin
}

// missingReader is the reader an unread file needs and the plugin that
// supplies it.
type missingReader struct {
	// what names the reader: `format "sourcecode"`, or `TypeScript comments`.
	what string
	// plugin is the registry name of the plugin that supplies the reader, or
	// empty when kapi knows no plugin that does.
	plugin string
}

// missingReaderOf names the reader err reports missing, for a file declared in
// format, and the plugin that supplies it.
func missingReaderOf(err error, format string, installed []*pluginhost.Plugin) missingReader {
	if comments, ok := errors.AsType[*noCommentReaderError](err); ok {
		return missingReader{what: comments.hint.DisplayName + " comments", plugin: comments.hint.Plugin}
	}
	plugin, _ := pluginhost.FormatProvider(installed, format)
	return missingReader{what: fmt.Sprintf("format %q", format), plugin: plugin}
}

// installClause says how to get a reader: the command that installs plugin, or,
// when plugin is empty, that kapi knows no plugin that supplies it.
func installClause(plugin string) string {
	if plugin == "" {
		return "no known plugin supplies it"
	}
	return fmt.Sprintf("install the plugin that supplies it (kapi plugins install %s)", plugin)
}

// formatInstallClause is installClause for the plugin that supplies format.
func formatInstallClause(installed []*pluginhost.Plugin, format string) string {
	plugin, _ := pluginhost.FormatProvider(installed, format)
	return installClause(plugin)
}

// sourceGateUnreadMessage is the run log line for a format no installed reader
// opens, whose content the source gate therefore did not count.
func sourceGateUnreadMessage(installed []*pluginhost.Plugin, format string) string {
	return fmt.Sprintf("No reader for format %q: its content is not counted in the source gate. %s.",
		format, sentenceCase(formatInstallClause(installed, format)))
}

// sentenceCase returns s with its first letter upper-cased, for a clause that
// starts a sentence.
func sentenceCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// noReaderReason says that a file was not read, which reader it needs and the
// plugin to install.
func noReaderReason(err error, format string, installed ...*pluginhost.Plugin) string {
	r := missingReaderOf(err, format, installed)
	return fmt.Sprintf("no reader for %s is installed; %s", r.what, installClause(r.plugin))
}

// NewUnreadSet returns an empty set for a check over the project's declared
// content, such as the desktop Checks panel's. installed are the plugins the
// caller discovered.
func NewUnreadSet(installed ...*pluginhost.Plugin) *UnreadSet {
	return &UnreadSet{formats: map[string]string{}, readers: map[string]missingReader{}, installed: installed}
}

// NoReaderError is the error for a request that names one file, or one unit in
// it, when no reader for the file's format is installed: it keeps err in the
// chain and names the plugin to install. installed are the plugins the caller
// discovered. Any other error is returned as it is.
func NoReaderError(err error, file, format string, installed ...*pluginhost.Plugin) error {
	if !errors.Is(err, registry.ErrUnknownFormat) {
		return err
	}
	return fmt.Errorf("%s: no reader for format %q is installed; %s: %w",
		file, format, formatInstallClause(installed, format), err)
}

// discoveredPlugins returns the plugins this App discovered, or nil before
// plugin discovery.
func (a *App) discoveredPlugins() []*pluginhost.Plugin {
	if a.PluginHost == nil {
		return nil
	}
	return a.PluginHost.Plugins()
}

// newUnreadSet returns the set a check over the project's declared content
// collects into, or nil when --format names the format every file is read
// under.
func (a *App) newUnreadSet() *UnreadSet {
	return a.newUnreadSetFor("")
}

// newUnreadSetFor is newUnreadSet for a command whose messages report another
// outcome than "checked", such as "converged" for a run.
func (a *App) newUnreadSetFor(outcome string) *UnreadSet {
	if a.FormatFlag != "" {
		return nil
	}
	u := NewUnreadSet(a.discoveredPlugins()...)
	u.outcome = outcome
	return u
}

// setAside reports whether no installed reader opens the format rf declares,
// and records rf in unread when so. It asks the registry for a reader the way a
// read does, so a format a plugin loads on demand is not set aside. A nil set
// sets nothing aside.
func (a *App) setAside(unread *UnreadSet, root string, rf project.ResolvedFile) bool {
	if unread == nil || rf.Format == "" {
		return false
	}
	name, _, err := a.resolveFormatRef(rf.Format)
	if err != nil {
		return false
	}
	reader, err := a.FormatReg.NewReader(registry.FormatID(name))
	if err == nil {
		_ = reader.Close()
		return false
	}
	return unread.Skip(err, relativeToRoot(root, rf.Path), rf.Format)
}

// Skip reports whether err says that no reader for the file is installed, for
// its format or for the comments of its language, and records the file when it
// does. Such a file was never opened,
// so nothing in it counts as checked. Any other error means the file was opened
// and is broken, and the caller returns it.
func (u *UnreadSet) Skip(err error, file, format string) bool {
	if u == nil || !errors.Is(err, registry.ErrUnknownFormat) {
		return false
	}
	if _, seen := u.formats[file]; !seen {
		u.formats[file] = format
		u.readers[file] = missingReaderOf(err, format, u.installed)
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

// holds reports whether the set records file, named the way it was recorded.
func (u *UnreadSet) holds(file string) bool {
	if u == nil {
		return false
	}
	_, ok := u.formats[file]
	return ok
}

// empty reports whether the set records no file.
func (u *UnreadSet) empty() bool { return u == nil || len(u.files) == 0 }

// verb is the outcome the set's messages report, "checked" unless the command
// set another.
func (u *UnreadSet) verb() string {
	if u == nil || u.outcome == "" {
		return "checked"
	}
	return u.outcome
}

// byFormat groups the unread files by their format, with the formats sorted and
// each format's files in the order the command met them.
func (u *UnreadSet) byFormat() ([]string, map[string][]string) {
	files := map[string][]string{}
	if u == nil {
		return nil, files
	}
	for _, file := range u.files {
		files[u.formats[file]] = append(files[u.formats[file]], file)
	}
	return slices.Sorted(maps.Keys(files)), files
}

// byReader groups the unread files by the reader they need, sorted by what the
// reader reads, with each reader's files in the order the command met them.
func (u *UnreadSet) byReader() ([]missingReader, map[missingReader][]string) {
	files := map[missingReader][]string{}
	if u == nil {
		return nil, files
	}
	for _, file := range u.files {
		files[u.readers[file]] = append(files[u.readers[file]], file)
	}
	return slices.SortedFunc(maps.Keys(files), func(x, y missingReader) int { return strings.Compare(x.what, y.what) }), files
}

// named lists files for a message, the first three by name.
func named(files []string) string {
	s := strings.Join(files[:min(len(files), 3)], ", ")
	if len(files) > 3 {
		s += fmt.Sprintf(" and %d more", len(files)-3)
	}
	return s
}

// summary names every unread format with its files, the plugins to install,
// each once, and the readers no known plugin supplies, for a command that could
// read none of the project's content.
func (u *UnreadSet) summary() string {
	formats, files := u.byFormat()
	declared := make([]string, 0, len(formats))
	for _, format := range formats {
		declared = append(declared, fmt.Sprintf("%q (%s)", format, named(files[format])))
	}
	var installs, unknown []string
	readers, _ := u.byReader()
	for _, r := range readers {
		switch install := "kapi plugins install " + r.plugin; {
		case r.plugin == "":
			unknown = append(unknown, r.what)
		case !slices.Contains(installs, install):
			installs = append(installs, install)
		}
	}
	msg := "no installed reader opens any of this project's content, declared in format " + strings.Join(declared, ", ")
	if len(installs) > 0 {
		msg += fmt.Sprintf("; install the plugin that supplies it (%s)", strings.Join(installs, ", "))
	}
	if len(unknown) > 0 {
		msg += "; no known plugin supplies " + strings.Join(unknown, ", ")
	}
	return msg
}

// warnings returns a WarningFormatNoReader warning for each unread file.
func (u *UnreadSet) warnings() []check.Warning {
	if u == nil {
		return nil
	}
	out := make([]check.Warning, 0, len(u.files))
	for _, file := range u.files {
		r := u.readers[file]
		out = append(out, check.Warning{
			Code:    check.WarningFormatNoReader,
			Source:  file,
			Message: fmt.Sprintf("no reader for %s is installed, so %s was not %s; %s", r.what, file, u.verb(), installClause(r.plugin)),
		})
	}
	return out
}

// reasons says of each file that it was not checked, and why.
func (u *UnreadSet) reasons(files []string) []string {
	out := make([]string, 0, len(files))
	for _, file := range files {
		out = append(out, fmt.Sprintf("%s was not checked: no reader for %s is installed", file, u.readers[file].what))
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
	if u.empty() || a.Quiet {
		return
	}
	readers, files := u.byReader()
	for _, r := range readers {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"warning: no reader for %s, so %s was not %s; %s\n",
			r.what, named(files[r]), u.verb(), installClause(r.plugin))
	}
}

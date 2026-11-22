package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flagReads records every flag name a command's code looks up. pflag passes
// each lookup (Lookup, Changed and every Get) through the flag set's
// normalize function, so a recorder installed there sees them all.
type flagReads struct {
	mu    sync.Mutex
	names map[string]bool
}

func (r *flagReads) install(fs *pflag.FlagSet) {
	fs.SetNormalizeFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
		r.mu.Lock()
		if r.names != nil {
			r.names[name] = true
		}
		r.mu.Unlock()
		return pflag.NormalizedName(name)
	})
	// Installing the function normalizes every registered name once; those
	// are not reads.
	r.mu.Lock()
	r.names = map[string]bool{}
	r.mu.Unlock()
}

// appBoundFields are the App fields a flag binds to directly (StringVar and
// the like). The code reads the field, never the flag, so the walk counts a
// flag bound to one of these as read; TestRunFlagsBoundToAppFieldsAreHonoured
// runs each.
var appBoundFields = []string{"FormatFlag", "EngineFlag", "Encoding", "SourceLang", "TargetLang"}

// boundToApp reports whether setting flag on a fresh command changes one of
// the appBoundFields of its App.
func boundToApp(t *testing.T, newCmd func(*App) *cobra.Command, flag string) bool {
	t.Helper()
	a := &App{}
	cmd := newCmd(a)
	fields := func() []string {
		v := reflect.ValueOf(a).Elem()
		out := make([]string, len(appBoundFields))
		for i, name := range appBoundFields {
			out[i] = v.FieldByName(name).String()
		}
		return out
	}
	before := fields()
	f := cmd.Flags().Lookup(flag)
	value := "x"
	if flag == "target-lang" || flag == "source-lang" {
		value = "de"
	}
	if f.Value.Type() != "string" && f.Value.Type() != "targetLang" {
		return false
	}
	if err := cmd.Flags().Set(flag, value); err != nil {
		return false
	}
	return !slices.Equal(before, fields())
}

// runRecorded runs newCmd's RunE once per invocation and returns the union of
// the flags read across them. Each invocation sets its flags before the
// recorder is installed, so setting a flag is not counted as reading it.
func runRecorded(t *testing.T, newCmd func(*App) *cobra.Command, invocations []flagInvocation) map[string]bool {
	t.Helper()
	read := map[string]bool{}
	for _, inv := range invocations {
		a := processOnlyApp(t)
		cmd := newCmd(a)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetContext(t.Context())
		for i := 0; i+1 < len(inv.flags); i += 2 {
			require.NoError(t, cmd.Flags().Set(inv.flags[i], inv.flags[i+1]), "%s: set --%s", inv.name, inv.flags[i])
		}
		rec := &flagReads{}
		rec.install(cmd.Flags())
		err := cmd.RunE(cmd, inv.args)
		if inv.wantErr {
			require.Error(t, err, "%s:\n%s", inv.name, out.String())
		} else {
			require.NoError(t, err, "%s:\n%s", inv.name, out.String())
		}
		rec.mu.Lock()
		for n := range rec.names {
			read[n] = true
		}
		rec.mu.Unlock()
	}
	return read
}

type flagInvocation struct {
	name    string
	args    []string
	flags   []string // name, value pairs
	wantErr bool
}

// assertEveryFlagRead fails for each flag registered on the command that no
// invocation read and that binds to no App field.
func assertEveryFlagRead(t *testing.T, newCmd func(*App) *cobra.Command, read map[string]bool) {
	t.Helper()
	var unread []string
	newCmd(&App{}).Flags().VisitAll(func(f *pflag.Flag) {
		if read[f.Name] || boundToApp(t, newCmd, f.Name) {
			return
		}
		unread = append(unread, "--"+f.Name)
	})
	slices.Sort(unread)
	assert.Empty(t, unread, "flags the command registers and never reads: a flag it accepts must act, or the command must refuse it")
}

// fileFixture writes a JSON source file under a fresh directory and returns
// the directory and the file.
func fileFixture(t *testing.T) (dir, file string) {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	file = filepath.Join(d, "messages_en.json")
	require.NoError(t, os.WriteFile(file, []byte(`{"greeting": "Hello"}`), 0o644))
	return d, file
}

// Every flag `kapi up` registers is read by some run of it.
func TestUpReadsEveryFlagItRegisters(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	recipe, _ := convergeFixture(t, []model.LocaleID{"fr-FR"}, gate.Gate{"translated": {Pct: 100}})
	p := []string{"project", recipe}
	// The built-in default flow (no defaults.flow) recycles from the content
	// memory --memory names, then translates.
	builtin, _ := convergeFixture(t, []model.LocaleID{"fr-FR"}, gate.Gate{"translated": {Pct: 100}})
	proj, err := project.Load(builtin)
	require.NoError(t, err)
	proj.Defaults.Flow = ""
	require.NoError(t, project.Save(builtin, proj))
	t.Setenv("KAPI_HOME", t.TempDir())
	read := runRecorded(t, NewUpCmd, []flagInvocation{
		{name: "run", flags: append([]string{"provider", "demo", "trace", filepath.Join(t.TempDir(), "t.json")}, p...)},
		{name: "built-in default", flags: []string{"project", builtin, "provider", "demo", "memory", "named"}},
		{name: "plan", flags: append([]string{"plan", "true"}, p...)},
		{name: "json", flags: append([]string{"json", "true"}, p...)},
		{name: "recycle-only", flags: append([]string{"recycle-only", "true"}, p...)},
		{name: "print-ops", flags: append([]string{"print-ops", "true"}, p...)},
		{name: "timeout on a local run", flags: append([]string{"timeout", "1m"}, p...), wantErr: true},
		{name: "local and server", flags: append([]string{"local", "true", "server", "true"}, p...), wantErr: true},
	})
	assertEveryFlagRead(t, NewUpCmd, read)
}

// Every flag `kapi translate` registers is read by some run of it.
func TestTranslateReadsEveryFlagItRegisters(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	dir, file := fileFixture(t)
	second := filepath.Join(dir, "more_en.json")
	require.NoError(t, os.WriteFile(second, []byte(`{"farewell": "Goodbye"}`), 0o644))
	unknown := filepath.Join(dir, "notes.nosuchformat")
	require.NoError(t, os.WriteFile(unknown, []byte("\x00\x01"), 0o644))
	read := runRecorded(t, NewTranslateCmd, []flagInvocation{
		{name: "file", args: []string{file}, flags: []string{"target-lang", "fr", "provider", "demo", "trace", filepath.Join(dir, "t.json")}},
		{name: "files", args: []string{file, second}, flags: []string{"target-lang", "fr", "provider", "demo", "output", filepath.Join(dir, "out") + "/"}},
		{name: "strict over an unreadable file", args: []string{file, unknown}, flags: []string{"target-lang", "fr", "provider", "demo", "strict", "true"}, wantErr: true},
		{name: "explain", args: []string{file}, flags: []string{"target-lang", "fr", "explain", "true"}},
		{name: "pack without a package", args: []string{file}, flags: []string{"target-lang", "fr", "pack", "true"}, wantErr: true},
	})
	assertEveryFlagRead(t, NewTranslateCmd, read)
}

// Every flag `kapi run` registers is read by some run of it.
func TestRunReadsEveryFlagItRegisters(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "1")
	dir, file := fileFixture(t)
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, &project.KapiProject{
		Version:  project.CurrentVersion,
		Name:     "flagreads",
		Defaults: project.Defaults{SourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"}},
		Collections: []project.Collection{{
			Path: "messages_en.json", Target: "messages_{lang}.json",
		}},
		Flows: map[string]*flow.StepsSpec{
			"mine": {Steps: []flow.FlowStep{{Tool: "pseudo-translate"}}},
			"fill": {Steps: []flow.FlowStep{{Tool: "recycle"}}},
		},
	}))
	t.Setenv("KAPI_HOME", t.TempDir())
	unknown := filepath.Join(dir, "notes.nosuchformat")
	require.NoError(t, os.WriteFile(unknown, []byte("\x00\x01"), 0o644))
	read := runRecorded(t, NewRunCmd, []flagInvocation{
		{name: "built-in over files", args: []string{"pseudo-translate"}, flags: []string{"input", file, "target-lang", "fr"}},
		{name: "recipe flow", args: []string{"mine"}, flags: []string{"project", recipe, "provider", "demo"}},
		{name: "recipe flow with a named content memory", args: []string{"fill"}, flags: []string{"project", recipe, "memory", "named"}},
		{name: "strict over an unreadable file", args: []string{"pseudo-translate"}, flags: []string{"input", file, "input", unknown, "target-lang", "fr", "strict", "true"}, wantErr: true},
		{name: "explain", args: []string{"pseudo-translate"}, flags: []string{"input", file, "target-lang", "fr", "explain", "true"}},
	})
	assertEveryFlagRead(t, NewRunCmd, read)
}

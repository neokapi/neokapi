package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/memory"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every flag `kapi up`, `kapi run` and `kapi translate` register has an
// effect: setting it changes what the command does (its output, the files it
// leaves, the run configuration it resolves) or the command refuses it. Each
// flag has one row below, and a flag with no row fails the test, so a flag
// that is accepted and does nothing cannot land.

// outcome is what one invocation did, normalized so two runs that did the same
// thing compare equal: its output and error with the fixture's paths and every
// digit masked (durations and timestamps vary), the files it left under the
// fixture directory, and the run configuration the flags resolve to.
type outcome struct {
	Out, Err  string
	Files     map[string]string
	RunConfig map[string]any
}

var digits = regexp.MustCompile(`[0-9]+`)

// fixture is a directory and what an invocation needs in it.
type fixture struct {
	dir   string
	app   func() *App
	setup func(t *testing.T, a *App)
}

func (f fixture) newApp(t *testing.T) *App {
	a := processOnlyApp(t)
	if f.app != nil {
		a = f.app()
	}
	if f.setup != nil {
		f.setup(t, a)
	}
	return a
}

// invoke runs newCmd with args in a fresh copy of the fixture and returns
// what it did.
func invoke(t *testing.T, newCmd func(*App) *cobra.Command, fx func(t *testing.T) fixture, args []string) outcome {
	t.Helper()
	f := fx(t)
	a := f.newApp(t)
	cmd := newCmd(a)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetContext(t.Context())
	cmd.SetArgs(args)
	cmd.PersistentPreRunE = func(*cobra.Command, []string) error { return a.Init() }
	t.Chdir(f.dir)
	err := cmd.Execute()
	mask := func(s string) string {
		s = strings.ReplaceAll(s, f.dir, "DIR")
		return digits.ReplaceAllString(s, "#")
	}
	o := outcome{Out: mask(out.String()), Files: map[string]string{}, RunConfig: host.RunFlagToolConfig(cmd)}
	if err != nil {
		o.Err = mask(err.Error())
	}
	_ = filepath.WalkDir(f.dir, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil || d.IsDir() {
			if d != nil && d.IsDir() && d.Name() == ".kapi" {
				return filepath.SkipDir
			}
			return nil
		}
		data, rerr := os.ReadFile(p)
		if rerr == nil {
			rel, _ := filepath.Rel(f.dir, p)
			o.Files[rel] = fmt.Sprintf("%x", sha256.Sum256([]byte(mask(string(data)))))
		}
		return nil
	})
	return o
}

func (o outcome) String() string {
	b, _ := json.MarshalIndent(o, "", "  ")
	return string(b)
}

// flagRow is one flag's proof of effect. base is an invocation without the
// flag, with the flag's arguments appended; check, when set, replaces the
// comparison for a flag whose effect is not in the outcome (concurrency).
type flagRow struct {
	flag  string
	fx    func(t *testing.T) fixture
	base  []string
	with  []string
	check func(t *testing.T)
}

// assertEffect fails unless the flag changed the outcome or was refused. The
// baseline runs twice first, so a row cannot pass on an outcome that varies
// from run to run.
func assertEffect(t *testing.T, newCmd func(*App) *cobra.Command, r flagRow) {
	t.Helper()
	if r.check != nil {
		r.check(t)
		return
	}
	base := invoke(t, newCmd, r.fx, r.base)
	again := invoke(t, newCmd, r.fx, r.base)
	require.Equal(t, base, again, "the baseline for --%s is not deterministic", r.flag)
	got := invoke(t, newCmd, r.fx, append(slices.Clone(r.base), r.with...))
	if got.Err != "" && base.Err == "" {
		return // refused
	}
	assert.NotEqual(t, base, got, "--%s %v changed nothing:\n%s", r.flag, r.with, got)
}

// assertEveryFlagHasARow fails for a registered flag with no row.
func assertEveryFlagHasARow(t *testing.T, newCmd func(*App) *cobra.Command, rows []flagRow) {
	t.Helper()
	have := map[string]bool{}
	for _, r := range rows {
		have[r.flag] = true
	}
	var missing []string
	newCmd(&App{}).Flags().VisitAll(func(f *pflag.Flag) {
		if !have[f.Name] {
			missing = append(missing, "--"+f.Name)
		}
	})
	sort.Strings(missing)
	assert.Empty(t, missing, "flags with no row: each must show an effect or be removed")
}

func runRows(t *testing.T, newCmd func(*App) *cobra.Command, rows []flagRow) {
	t.Helper()
	assertEveryFlagHasARow(t, newCmd, rows)
	for _, r := range rows {
		t.Run(r.flag, func(t *testing.T) { assertEffect(t, newCmd, r) })
	}
}

// isolate pins every root an in-process run could inherit.
func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "1")
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_AI_PROVIDER", "")
}

// fileFixture is a directory holding messages_en.json and a recipe over it,
// whose flows are "mine" (pseudo-translate) and "fill" (recycle). latin1
// writes the source in ISO-8859-1.
func fileFixture(latin1 bool) func(t *testing.T) fixture {
	return func(t *testing.T) fixture {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)
		body := []byte(`{"greeting": "Hello", "farewell": "Goodbye"}`)
		if latin1 {
			body = []byte("{\"greeting\": \"Caf\xe9\"}")
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, "messages_en.json"), body, 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.nosuchformat"), []byte("\x00\x01"), 0o644))
		// A multilingual file that names no source language: --source-lang
		// picks the variant read as the source.
		require.NoError(t, os.WriteFile(filepath.Join(dir, "both.tmx"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4"><header creationtool="t" creationtoolversion="1" segtype="sentence" o-tmf="x" adminlang="en" srclang="*all*" datatype="plaintext"/>
<body><tu><tuv xml:lang="en"><seg>Hello</seg></tuv><tuv xml:lang="de"><seg>Hallo</seg></tuv></tu></body></tmx>
`), 0o644))
		require.NoError(t, project.Save(filepath.Join(dir, project.RecipeFileName), &project.KapiProject{
			Version:     project.CurrentVersion,
			Name:        "flageffects",
			Defaults:    project.Defaults{SourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"}},
			Collections: []project.Collection{{Path: "messages_en.json", Target: "messages_{lang}.json"}},
			Flows: map[string]*flow.StepsSpec{
				"mine": {Steps: []flow.FlowStep{{Tool: "pseudo-translate"}}},
			},
		}))
		return fixture{dir: dir, app: demoApp}
	}
}

// demoApp is an App whose stored AI provider is demo, the offline stub.
func demoApp() *App {
	a := &App{}
	a.InitRegistries()
	a.AssumeYes = true
	a.Config = config.NewAppConfig()
	a.Config.Set(config.KeyAIProvider, "demo")
	return a
}

// seedMemoryFile writes a content memory at path answering "Hello" in fr.
func seedMemoryFile(t *testing.T, path string) {
	t.Helper()
	tm, err := memory.NewSQLiteStore(path)
	require.NoError(t, err)
	defer tm.Close()
	require.NoError(t, tm.Add(t.Context(), memory.Entry{
		ID: "hello",
		Variants: map[model.LocaleID][]model.Run{
			"en": {model.TextR("Hello")},
			"fr": {model.TextR("Bonjour de la mémoire")},
		},
		HintSrcLang: "en",
	}))
}

// upFixture is a copy of convergeFixture (a pseudo-translate default flow)
// with the given recipe edit.
func upFixture(edit func(t *testing.T, dir string, p *project.KapiProject)) func(t *testing.T) fixture {
	return func(t *testing.T) fixture {
		recipe, root := convergeFixture(t, []model.LocaleID{"fr-FR"}, gate.Gate{"translated": {Pct: 100}})
		if edit != nil {
			p, err := project.Load(recipe)
			require.NoError(t, err)
			edit(t, root, p)
			require.NoError(t, project.Save(recipe, p))
		}
		return fixture{dir: root}
	}
}

// concurrencyProbe registers a tool under name that records the most blocks
// it handles at once, and returns that peak.
func concurrencyProbe(a *App, name string) *atomic.Int32 {
	var active, peak atomic.Int32
	a.ToolReg.Register(registry.ToolID(name), func() tool.Tool {
		return &tool.BaseTool{ToolName: name, Annotate: func(tool.BlockView) error {
			cur := active.Add(1)
			for {
				old := peak.Load()
				if cur <= old || peak.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			active.Add(-1)
			return nil
		}}
	})
	return &peak
}

// probePeak runs newCmd over a probe fixture with args and returns the peak.
func probePeak(t *testing.T, newCmd func(*App) *cobra.Command, f fixture, tool string, args []string) int32 {
	t.Helper()
	a := f.newApp(t)
	peak := concurrencyProbe(a, tool)
	cmd := newCmd(a)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetContext(t.Context())
	cmd.SetArgs(args)
	cmd.PersistentPreRunE = func(*cobra.Command, []string) error { return a.Init() }
	t.Chdir(f.dir)
	require.NoError(t, cmd.Execute(), out.String())
	return peak.Load()
}

// probeFiles is a directory of two eight-key JSON files.
func probeFiles(t *testing.T) (string, []string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	var b strings.Builder
	b.WriteString("{")
	for i := range 8 {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `"k%d": "Text %d"`, i, i)
	}
	b.WriteString("}")
	var files []string
	for _, n := range []string{"a_en.json", "b_en.json"} {
		p := filepath.Join(dir, n)
		require.NoError(t, os.WriteFile(p, []byte(b.String()), 0o644))
		files = append(files, p)
	}
	return dir, files
}

// fileRows are the rows `kapi run` and `kapi translate` share. files turns
// input files into the command's arguments (`--input f` for run, positional
// for translate); flow is run's flow name, empty for translate.
func fileRows(files func(...string) []string, flow string) []flagRow {
	fx := fileFixture(false)
	cmdline := func(flowName string, args ...string) []string {
		var out []string
		if flowName != "" {
			out = append(out, flowName)
		}
		return append(out, args...)
	}
	in := func(names ...string) []string {
		return cmdline(flow, append(files(names...), "--target-lang", "fr")...)
	}
	base := in("messages_en.json")
	// A step that reads a content memory or a terms store: run's built-in
	// translate flow (recycle, translate), and translate itself.
	storeFlow := flow
	if flow != "" {
		storeFlow = "translate"
	}
	storeBase := cmdline(storeFlow, append(files("messages_en.json"), "--target-lang", "fr")...)
	memFx := func(t *testing.T) fixture {
		f := fileFixture(false)(t)
		seedMemoryFile(t, filepath.Join(f.dir, "seed.db"))
		return f
	}
	cfg := func(flag, value string) flagRow {
		return flagRow{flag: flag, fx: fx, base: base, with: []string{"--" + flag, value}}
	}
	return []flagRow{
		cfg("credential", "saved"), cfg("api-key", "sk-x"), cfg("model", "m"),
		cfg("instruction", "formal"), cfg("context", "none"), cfg("batching", "single"),
		{flag: "provider", fx: fx, base: storeBase, with: []string{"--provider", "nosuch"}},
		{flag: "format", fx: fx, base: base, with: []string{"--format", "properties"}},
		{flag: "engine", fx: fx, base: base, with: []string{"--engine", "nosuch"}},
		{flag: "encoding", fx: fileFixture(true), base: base, with: []string{"--encoding", "latin1"}},
		{flag: "source-lang", fx: fx, base: in("both.tmx"), with: []string{"--source-lang", "de"}},
		{flag: "target-lang", fx: fx, base: cmdline(flow, files("messages_en.json")...), with: []string{"--target-lang", "de"}},
		{flag: "input", fx: fx, base: base, with: []string{"--input", "notes.nosuchformat", "--fail-on-unknown"}},
		{flag: "output", fx: fx, base: base, with: []string{"--output", "out/{name}_{lang}.json"}},
		{flag: "trace", fx: fx, base: base, with: []string{"--trace", "t.json"}},
		{flag: "pack", fx: fx, base: base, with: []string{"--pack"}},
		{flag: "memory", fx: memFx, base: storeBase, with: []string{"--memory", "./seed.db"}},
		{flag: "termstore", fx: fx, base: storeBase, with: []string{"--termstore", "nosuch"}},
		{flag: "explain", fx: fx, base: base, with: []string{"--explain"}},
		{flag: "fail-on-unknown", fx: fx, base: in("messages_en.json", "notes.nosuchformat"), with: []string{"--fail-on-unknown"}},
		{flag: "strict", fx: fx, base: in("messages_en.json", "notes.nosuchformat"), with: []string{"--strict"}},
		{flag: "print-ops", fx: fx, base: base, with: []string{"--print-ops"}},
		{flag: "project", fx: fx, base: base, with: []string{"--project", "kapi.yaml"}},
		{flag: "progress", fx: fx, base: base, with: []string{"--progress", "jsonl"}},
	}
}

// concurrencyRows prove --concurrency and --parallel-blocks by the peak
// number of blocks in flight. The probe is registered as probeTool; with a
// project, the recipe's "probe" flow runs it.
func concurrencyRows(newCmd func(*App) *cobra.Command, prefix []string, probeTool string, withProject bool) []flagRow {
	run := func(t *testing.T, extra ...string) int32 {
		dir, files := probeFiles(t)
		args := slices.Clone(prefix)
		if withProject {
			recipe := filepath.Join(dir, project.RecipeFileName)
			require.NoError(t, project.Save(recipe, &project.KapiProject{
				Version: project.CurrentVersion, Name: "probe",
				Defaults: project.Defaults{SourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"}},
				Flows:    map[string]*flow.StepsSpec{"probe": {Steps: []flow.FlowStep{{Tool: probeTool}}}},
			}))
			args = append(args, "--project", recipe)
		}
		args = append(args, "--input", files[0], "--input", files[1], "--target-lang", "fr",
			"--output", filepath.Join(dir, "out")+"/")
		return probePeak(t, newCmd, fixture{dir: dir}, probeTool, append(args, extra...))
	}
	return []flagRow{
		{flag: "concurrency", check: func(t *testing.T) {
			assert.Equal(t, int32(1), run(t, "--concurrency", "1", "--parallel-blocks", "1"))
			assert.Greater(t, run(t, "--concurrency", "2", "--parallel-blocks", "1"), int32(1))
		}},
		{flag: "parallel-blocks", check: func(t *testing.T) {
			assert.Equal(t, int32(1), run(t, "--concurrency", "1", "--parallel-blocks", "1"))
			assert.Greater(t, run(t, "--concurrency", "1", "--parallel-blocks", "4"), int32(1))
		}},
	}
}

func TestRunFlagsEachHaveAnEffect(t *testing.T) {
	isolate(t)
	inputs := func(names ...string) []string {
		var out []string
		for _, n := range names {
			out = append(out, "--input", n)
		}
		return out
	}
	// pseudo-translate needs no provider.
	rows := fileRows(inputs, "pseudo-translate")
	rows = append(rows, concurrencyRows(NewRunCmd, []string{"probe"}, "concurrency-probe", true)...)
	runRows(t, NewRunCmd, rows)
}

func TestTranslateFlagsEachHaveAnEffect(t *testing.T) {
	isolate(t)
	positional := func(names ...string) []string { return names }
	rows := fileRows(positional, "")
	// The probe stands in for the translate tool, so the porcelain's own flow
	// runs it.
	runRows(t, NewTranslateCmd, append(rows, concurrencyRows(NewTranslateCmd, nil, "translate", false)...))
}

func TestUpFlagsEachHaveAnEffect(t *testing.T) {
	isolate(t)
	fx := upFixture(nil)
	p := []string{"--project", "app.kapi"}
	cfg := func(flag, value string) flagRow {
		return flagRow{flag: flag, fx: fx, base: p, with: []string{"--" + flag, value}}
	}
	// A collection whose declared format no installed reader opens.
	unreadable := upFixture(func(t *testing.T, dir string, pr *project.KapiProject) {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "notes"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "notes", "n.nosuch"), []byte("x"), 0o644))
		pr.Collections = append(pr.Collections, project.Collection{
			Path: "notes/*.nosuch", Format: &project.FormatSpec{Name: "nosuch"}, Target: "notes/{lang}/*.nosuch",
		})
	})
	// Targets identical to their source fail the bound identical-target check.
	identical := upFixture(func(t *testing.T, dir string, _ *project.KapiProject) {
		for _, f := range []string{"a.json", "b.json"} {
			data, err := os.ReadFile(filepath.Join(dir, "src/locales/en", f))
			require.NoError(t, err)
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "src/locales/fr-FR"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "src/locales/fr-FR", f), data, 0o644))
		}
	})
	jobsPeak := func(t *testing.T, jobs string) int32 {
		f := upFixture(func(_ *testing.T, _ string, pr *project.KapiProject) {
			pr.Defaults.TargetLanguages = []model.LocaleID{"fr-FR", "de-DE", "nb-NO"}
			pr.Defaults.Flow = "probe"
			pr.Defaults.TranslateAfter = "none"
			pr.Flows["probe"] = &flow.StepsSpec{Steps: []flow.FlowStep{{Tool: "concurrency-probe"}}}
		})(t)
		return probePeak(t, NewUpCmd, f, "concurrency-probe", []string{"--project", "app.kapi", "--passes", "1", "--jobs", jobs})
	}
	rows := []flagRow{
		cfg("credential", "saved"), cfg("provider", "demo"), cfg("api-key", "sk-x"), cfg("model", "m"),
		cfg("instruction", "formal"), cfg("context", "none"), cfg("batching", "single"),
		{flag: "encoding", fx: fx, base: p, with: []string{"--encoding", "utf-16le"}},
		{flag: "termstore", fx: fx, base: p, with: []string{"--termstore", "nosuch"}},
		{flag: "trace", fx: fx, base: p, with: []string{"--trace", "t.json"}},
		{flag: "fail-on-unknown", fx: unreadable, base: p, with: []string{"--fail-on-unknown"}},
		{flag: "strict", fx: unreadable, base: p, with: []string{"--strict"}},
		{flag: "print-ops", fx: fx, base: p, with: []string{"--print-ops"}},
		{flag: "project", fx: fx, base: p, with: []string{"--project", "nowhere"}},
		{flag: "passes", fx: upFixture(func(_ *testing.T, _ string, pr *project.KapiProject) {
			pr.ShipGate = gate.Gate{"established": {Pct: 100}}
		}), base: p, with: []string{"--passes", "1"}},
		{flag: "jobs", check: func(t *testing.T) {
			// Files within a locale run concurrently too, so --jobs shows as
			// more blocks in flight than one language at a time allows.
			one := jobsPeak(t, "1")
			assert.Greater(t, jobsPeak(t, "3"), one)
		}},
		{flag: "no-extract", fx: fx, base: p, with: []string{"--no-extract"}},
		{flag: "no-checks", fx: identical, base: append(slices.Clone(p), "--recycle-only"), with: []string{"--no-checks"}},
		{flag: "recycle-only", fx: fx, base: p, with: []string{"--recycle-only"}},
		{flag: "materialize", fx: fx, base: p, with: []string{"--materialize"}},
		{flag: "pre-review", fx: fx, base: p, with: []string{"--pre-review"}},
		{flag: "plan", fx: fx, base: p, with: []string{"--plan"}},
		{flag: "json", fx: fx, base: p, with: []string{"--json"}},
		{flag: "local", fx: fx, base: append(slices.Clone(p), "--server"), with: []string{"--local"}},
		{flag: "server", fx: fx, base: p, with: []string{"--server"}},
		{flag: "timeout", fx: fx, base: p, with: []string{"--timeout", "1m"}},
		{flag: "fail-on-incomplete-watch", fx: fx, base: p, with: []string{"--fail-on-incomplete-watch"}},
	}
	runRows(t, NewUpCmd, rows)
}

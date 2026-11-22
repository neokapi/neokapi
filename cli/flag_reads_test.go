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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/memory"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every flag `kapi up`, `kapi run` and `kapi translate` register has an
// effect, observed where the value is used. Setting it changes the config an
// AI step was constructed with, the output or the files the run leaves, or
// the command refuses it with that flag's own error. A flag with no row fails
// the test.
//
// A row calls a flag inert only after trying it on a fixture where it could
// matter: a flow with an AI step for the AI flags, a seeded content memory for
// --memory, two files and several languages for the concurrency flags.

// aiKeys are the config keys an AI step is constructed with that the run
// flags set.
var aiKeys = []string{"provider", "model", "api_key", "credential", "instruction", "context", "batching"}

// recorder replaces the translate tool's construction with one that records
// the config the step was built with (after the run's preprocessor: stored
// default, credentials) and passes blocks through, so no provider is called.
type recorder struct {
	mu     sync.Mutex
	seen   []map[string]any
	blocks []string
}

func (r *recorder) install(a *App) {
	a.ToolReg.SetConfigFactory("translate", func(cfg map[string]any, _ string) (tool.Tool, error) {
		got := map[string]any{}
		for _, k := range aiKeys {
			if v, ok := cfg[k]; ok && v != "" {
				got[k] = v
			}
		}
		r.mu.Lock()
		r.seen = append(r.seen, got)
		r.mu.Unlock()
		return &tool.BaseTool{ToolName: "translate", Annotate: func(v tool.BlockView) error {
			r.mu.Lock()
			r.blocks = append(r.blocks, string(v.SourceLocale())+": "+v.SourceText())
			r.mu.Unlock()
			return nil
		}}, nil
	})
}

// constructed is the configs recorded, in a stable order: steps are built
// concurrently, one per file and language.
func (r *recorder) constructed() []map[string]any {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := slices.Clone(r.seen)
	key := func(m map[string]any) string { b, _ := json.Marshal(m); return string(b) }
	slices.SortFunc(out, func(x, y map[string]any) int { return strings.Compare(key(x), key(y)) })
	return out
}

// read is the source language and text of each block the step read, sorted.
func (r *recorder) read() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := slices.Clone(r.blocks)
	slices.Sort(out)
	return out
}

// outcome is what one invocation did: its output and error with the fixture's
// paths and every digit masked (durations vary), the files it left under the
// fixture directory, and the config each AI step was constructed with.
type outcome struct {
	Out, Err    string
	Files       map[string]string
	Constructed []map[string]any
	Read        []string
}

var digits = regexp.MustCompile(`[0-9]+`)

// fixture is a directory and how to adjust the App an invocation runs on.
type fixture struct {
	dir   string
	setup func(t *testing.T, a *App)
}

// invoke runs newCmd with args in a fresh copy of the fixture and returns
// what it did. The App's stored AI provider is demo, the offline stub, so a
// run with no provider flag reaches no network.
func invoke(t *testing.T, newCmd func(*App) *cobra.Command, fx func(t *testing.T) fixture, args []string) outcome {
	t.Helper()
	f := fx(t)
	a := processOnlyApp(t)
	if f.setup != nil {
		f.setup(t, a)
	}
	rec := &recorder{}
	cmd := newCmd(a)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetContext(t.Context())
	cmd.SetArgs(args)
	cmd.PersistentPreRunE = func(*cobra.Command, []string) error {
		if err := a.Init(); err != nil {
			return err
		}
		a.Config.Set(config.KeyAIProvider, "demo")
		rec.install(a)
		return nil
	}
	t.Chdir(f.dir)
	err := cmd.Execute()
	mask := func(s string) string {
		s = strings.ReplaceAll(s, f.dir, "DIR")
		return digits.ReplaceAllString(s, "#")
	}
	o := outcome{Out: mask(out.String()), Files: map[string]string{}, Constructed: rec.constructed(), Read: rec.read()}
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
		// A database file's bytes vary between identical runs.
		if strings.Contains(filepath.Base(p), ".db") {
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
// flag, and with is the flag and its value. refusal, when set, is text of
// that flag's own refusal. The row then passes only on that error. check,
// when set, replaces the comparison for an effect not in the outcome
// (concurrency).
type flagRow struct {
	flag    string
	fx      func(t *testing.T) fixture
	base    []string
	with    []string
	refusal string
	check   func(t *testing.T)
}

// assertEffect fails unless the flag changed the outcome with no new error,
// or was refused with its own error. The baseline runs twice first, so a row
// cannot pass on an outcome that varies from run to run.
func assertEffect(t *testing.T, newCmd func(*App) *cobra.Command, r flagRow) {
	t.Helper()
	if r.check != nil {
		r.check(t)
		return
	}
	base := invoke(t, newCmd, r.fx, r.base)
	again := invoke(t, newCmd, r.fx, r.base)
	require.Equal(t, base, again, "the baseline for --%s is not deterministic", r.flag)
	require.Empty(t, base.Err, "the baseline for --%s must succeed:\n%s", r.flag, base)
	got := invoke(t, newCmd, r.fx, append(slices.Clone(r.base), r.with...))
	if r.refusal != "" {
		assert.Contains(t, got.Err, r.refusal, "--%s %v is refused with its own error:\n%s", r.flag, r.with, got)
		return
	}
	require.Empty(t, got.Err, "--%s %v failed:\n%s", r.flag, r.with, got)
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
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
}

// fileFixture is a directory holding messages_en.json, more_en.json, a file
// no reader opens, a TMX holding en and de with no source language, a content
// memory answering "Hello" at seed.db, and a recipe over messages_en.json.
// latin1 writes messages_en.json in ISO-8859-1.
func fileFixture(latin1 bool) func(t *testing.T) fixture {
	return func(t *testing.T) fixture {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)
		body := []byte(`{"greeting": "Hello", "farewell": "Goodbye"}`)
		if latin1 {
			body = []byte("{\"greeting\": \"Caf\xe9\"}")
		}
		require.NoError(t, os.WriteFile(filepath.Join(dir, "messages_en.json"), body, 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "more_en.json"), []byte(`{"other": "More"}`), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.nosuchformat"), []byte("\x00\x01"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "both.tmx"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4"><header creationtool="t" creationtoolversion="1" segtype="sentence" o-tmf="x" adminlang="en" srclang="*all*" datatype="plaintext"/>
<body><tu><tuv xml:lang="en"><seg>Hello</seg></tuv><tuv xml:lang="de"><seg>Hallo</seg></tuv></tu></body></tmx>
`), 0o644))
		seedMemoryFile(t, filepath.Join(dir, "seed.db"))
		require.NoError(t, project.Save(filepath.Join(dir, project.RecipeFileName), &project.KapiProject{
			Version:     project.CurrentVersion,
			Name:        "flageffects",
			Defaults:    project.Defaults{SourceLanguage: "en", TargetLanguages: []model.LocaleID{"fr"}},
			Collections: []project.Collection{{Path: "messages_en.json", Target: "messages_{lang}.json"}},
		}))
		return fixture{dir: dir}
	}
}

// seedMemoryFile writes a content memory at path answering "Hello" and
// "Hello, world." in fr.
func seedMemoryFile(t *testing.T, path string) {
	t.Helper()
	tm, err := memory.NewSQLiteStore(path)
	require.NoError(t, err)
	defer tm.Close()
	for id, pair := range map[string][2]model.LocaleID{"hello": {"en", "fr"}, "hello-us": {"en-US", "fr-FR"}} {
		src := "Hello"
		if pair[0] == "en-US" {
			src = "Hello, world."
		}
		require.NoError(t, tm.Add(t.Context(), memory.Entry{
			ID:          id,
			Variants:    map[model.LocaleID][]model.Run{pair[0]: {model.TextR(src)}, pair[1]: {model.TextR("Bonjour de la mémoire")}},
			HintSrcLang: pair[0],
		}))
	}
}

// upFixture is a copy of convergeFixture (a pseudo-translate default flow)
// with the given recipe edit, and a seeded content memory at seed.db.
func upFixture(edit func(t *testing.T, dir string, p *project.KapiProject)) func(t *testing.T) fixture {
	return func(t *testing.T) fixture {
		recipe, root := convergeFixture(t, []model.LocaleID{"fr-FR"}, gate.Gate{"translated": {Pct: 100}})
		seedMemoryFile(t, filepath.Join(root, "seed.db"))
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
func probePeak(t *testing.T, newCmd func(*App) *cobra.Command, f fixture, probe string, args []string) int32 {
	t.Helper()
	a := processOnlyApp(t)
	cmd := newCmd(a)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetContext(t.Context())
	cmd.SetArgs(args)
	var peak *atomic.Int32
	cmd.PersistentPreRunE = func(*cobra.Command, []string) error {
		if err := a.Init(); err != nil {
			return err
		}
		peak = concurrencyProbe(a, probe)
		return nil
	}
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

// aiRows prove the AI run flags by the config the translate step was
// constructed with. base must run a flow with a translate step.
func aiRows(fx func(t *testing.T) fixture, base []string) []flagRow {
	row := func(flag, value string) flagRow {
		return flagRow{flag: flag, fx: fx, base: base, with: []string{"--" + flag, value}}
	}
	return []flagRow{
		{flag: "credential", fx: fx, base: base, with: []string{"--credential", "saved"}, refusal: `"saved"`},
		row("provider", "ollama"), row("api-key", "sk-x"), row("model", "m-flag"),
		row("instruction", "formal"), row("context", "none"), row("batching", "single"),
	}
}

// fileRows are the rows `kapi run` and `kapi translate` share. files turns
// input files into the command's arguments (`--input f` for run, positional
// for translate); flow is run's flow name, empty for translate.
func fileRows(files func(...string) []string, flowName string) []flagRow {
	fx := fileFixture(false)
	cmdline := func(name string, args ...string) []string {
		var out []string
		if name != "" {
			out = append(out, name)
		}
		return append(out, args...)
	}
	in := func(names ...string) []string {
		return cmdline(flowName, append(files(names...), "--target-lang", "fr")...)
	}
	base := in("messages_en.json")
	// A flow with a translate step (recycle, translate): run's built-in
	// translate flow, and translate itself.
	aiFlow := flowName
	if flowName != "" {
		aiFlow = "translate"
	}
	aiBase := cmdline(aiFlow, append(files("messages_en.json"), "--target-lang", "fr")...)
	rows := aiRows(fx, aiBase)
	return append(rows, []flagRow{
		{flag: "format", fx: fx, base: base, with: []string{"--format", "properties"}},
		{flag: "engine", fx: fx, base: base, with: []string{"--engine", "nosuch"}, refusal: `unknown format engine "nosuch"`},
		{flag: "encoding", fx: fileFixture(true), base: aiBase, with: []string{"--encoding", "iso-8859-1"}},
		{flag: "source-lang", fx: fx, base: in("both.tmx"), with: []string{"--source-lang", "de"}},
		{flag: "target-lang", fx: fx, base: base, with: []string{"--target-lang", "de"}},
		{flag: "input", fx: fx, base: base, with: []string{"--input", "more_en.json"}},
		{flag: "output", fx: fx, base: base, with: []string{"--output", "out/{name}_{lang}.json"}},
		{flag: "trace", fx: fx, base: base, with: []string{"--trace", "t.json"}},
		{flag: "pack", fx: fx, base: base, with: []string{"--pack"}, refusal: "--pack applies to a .kpz input"},
		{flag: "memory", fx: fx, base: aiBase, with: []string{"--memory", "./seed.db"}},
		{flag: "termstore", fx: fx, base: aiBase, with: []string{"--termstore", "nosuch"}},
		{flag: "explain", fx: fx, base: base, with: []string{"--explain"}},
		{flag: "fail-on-unknown", fx: fx, base: in("messages_en.json", "notes.nosuchformat"), with: []string{"--fail-on-unknown"}, refusal: "unable to detect format"},
		{flag: "strict", fx: fx, base: in("messages_en.json", "notes.nosuchformat"), with: []string{"--strict"}, refusal: "unable to detect format"},
		{flag: "print-ops", fx: fx, base: base, with: []string{"--print-ops"}},
		{flag: "project", fx: fx, base: base, with: []string{"--project", "kapi.yaml"}},
		{flag: "progress", fx: fx, base: base, with: []string{"--progress", "jsonl"}},
	}...)
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
	// The built-in default flow (recycle, then translate) runs an AI step.
	builtin := upFixture(func(_ *testing.T, _ string, pr *project.KapiProject) { pr.Defaults.Flow = "" })
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
	latin1 := upFixture(func(t *testing.T, dir string, pr *project.KapiProject) {
		pr.Defaults.Flow = ""
		require.NoError(t, os.WriteFile(filepath.Join(dir, "src/locales/en", "a.json"), []byte("{\"greeting\":\"Caf\xe9\"}"), 0o644))
	})
	// A recipe declaring a server venue, dispatched to a stub plugin.
	venue := func(t *testing.T) fixture {
		recipe := venueParityProject(t)
		plugins := venuePluginHost(t)
		return fixture{dir: filepath.Dir(recipe), setup: func(_ *testing.T, a *App) { a.PluginHost = plugins }}
	}
	probe := func(t *testing.T, flag, low, high string, edit func(*project.KapiProject)) {
		peak := func(value string) int32 {
			f := upFixture(func(_ *testing.T, _ string, pr *project.KapiProject) {
				pr.Defaults.Flow = "probe"
				pr.Defaults.TranslateAfter = "none"
				pr.Flows["probe"] = &flow.StepsSpec{Steps: []flow.FlowStep{{Tool: "concurrency-probe"}}}
				edit(pr)
			})(t)
			return probePeak(t, NewUpCmd, f, "concurrency-probe", []string{"--project", "app.kapi", "--passes", "1", "--" + flag, value})
		}
		one := peak(low)
		assert.Greater(t, peak(high), one, "--%s %s runs more blocks at once than --%s %s", flag, high, flag, low)
	}
	rows := aiRows(builtin, p)
	rows = append(rows, []flagRow{
		{flag: "encoding", fx: latin1, base: p, with: []string{"--encoding", "iso-8859-1"}},
		{flag: "memory", fx: fx, base: append(slices.Clone(p), "--recycle-only"), with: []string{"--memory", "./seed.db"}},
		{flag: "termstore", fx: fx, base: p, with: []string{"--termstore", "nosuch"}, refusal: `terms store "nosuch" does not exist`},
		{flag: "trace", fx: fx, base: p, with: []string{"--trace", "t.json"}},
		{flag: "fail-on-unknown", fx: unreadable, base: p, with: []string{"--fail-on-unknown"}, refusal: "nosuch"},
		{flag: "strict", fx: unreadable, base: p, with: []string{"--strict"}, refusal: "nosuch"},
		{flag: "print-ops", fx: fx, base: p, with: []string{"--print-ops"}},
		{flag: "project", fx: fx, base: p, with: []string{"--project", "nowhere"}, refusal: "-p nowhere does not exist"},
		{flag: "passes", fx: upFixture(func(_ *testing.T, _ string, pr *project.KapiProject) {
			pr.ShipGate = gate.Gate{"established": {Pct: 100}}
		}), base: p, with: []string{"--passes", "1"}},
		{flag: "jobs", check: func(t *testing.T) {
			probe(t, "jobs", "1", "3", func(pr *project.KapiProject) {
				pr.Defaults.TargetLanguages = []model.LocaleID{"fr-FR", "de-DE", "nb-NO"}
			})
		}},
		{flag: "concurrency", check: func(t *testing.T) {
			probe(t, "concurrency", "1", "2", func(*project.KapiProject) {})
		}},
		{flag: "no-extract", fx: fx, base: p, with: []string{"--no-extract"}},
		{flag: "no-checks", fx: identical, base: append(slices.Clone(p), "--recycle-only"), with: []string{"--no-checks"}},
		{flag: "recycle-only", fx: fx, base: p, with: []string{"--recycle-only"}},
		{flag: "materialize", fx: fx, base: p, with: []string{"--materialize"}},
		{flag: "pre-review", fx: fx, base: p, with: []string{"--pre-review"}},
		{flag: "plan", fx: fx, base: p, with: []string{"--plan"}},
		{flag: "json", fx: fx, base: p, with: []string{"--json"}},
		{flag: "local", fx: venue, base: []string{"--project", "kapi.yaml"}, with: []string{"--local"}},
		{flag: "server", fx: fx, base: p, with: []string{"--server"}, refusal: "server"},
		{flag: "timeout", fx: fx, base: p, with: []string{"--timeout", "1m"}, refusal: "--timeout applies to a server run"},
		{flag: "fail-on-incomplete-watch", fx: fx, base: p, with: []string{"--fail-on-incomplete-watch"}, refusal: "--fail-on-incomplete-watch applies to a server run"},
	}...)
	runRows(t, NewUpCmd, rows)
}

//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// The gate conformance suite holds the built binary to what the gate claims to
// catch and to leave alone. Each case under gate/testdata is a small project
// and an expect.yaml naming what `kapi check`, `kapi fix`, `kapi context` and
// the plugin's Stop hook do with it. An expectation the code does not meet yet
// carries `xfail: <reason>`, and the test fails once it passes. `make
// gate-conformance` prints the matrix; web/docs/contribute/testing.md describes
// the expect.yaml keys.

// gateCase is one fixture's expect.yaml.
type gateCase struct {
	Description string      `yaml:"description"`
	Stages      []gateStage `yaml:"stages"`
}

// gateStage is a set of steps and the expectations that hold after them. The
// stages of a case run in order against the same project.
type gateStage struct {
	Name  string       `yaml:"name"`
	Steps []gateStep   `yaml:"steps"`
	Files []fileExpect `yaml:"files"`
	Hook  *hookExpect  `yaml:"hook"`
}

// gateStep is one setup action. Exactly one of Kapi, Keep, Narrow and Write
// is set.
type gateStep struct {
	// Kapi runs kapi with these arguments in the project; -p kapi.yaml is
	// added. Actor is "person" (the default) or "agent".
	Kapi  []string `yaml:"kapi"`
	Actor string   `yaml:"actor"`
	// Keep keeps the waiting suggestion whose sentence `kapi rules --json`
	// prints as this, through `kapi rules --keep <id>`. Use and Advisory pass
	// --use and --advisory with it.
	Keep     string `yaml:"keep"`
	Use      string `yaml:"use"`
	Advisory bool   `yaml:"advisory"`
	// Narrow limits the kept rule whose sentence `kapi rules list --json`
	// prints as this to the collection or folder To.
	Narrow string `yaml:"narrow"`
	To     string `yaml:"to"`
	// Write replaces or extends a file, as an assistant's edit would.
	Write *fileWrite `yaml:"write"`
}

// fileWrite is an edit to one file in the project.
type fileWrite struct {
	Path    string `yaml:"path"`
	Content string `yaml:"content"`
	Append  string `yaml:"append"`
}

// fileExpect names what each surface does with one content file.
type fileExpect struct {
	Path    string         `yaml:"path"`
	Check   *checkExpect   `yaml:"check"`
	Fix     *fixExpect     `yaml:"fix"`
	Context *contextExpect `yaml:"context"`
}

// checkExpect is what `kapi check <path> --json` reports.
type checkExpect struct {
	// Args are added to the check command line.
	Args []string `yaml:"args"`
	// Failing and Reporting, when set, are the exact counts in the summary.
	Failing   *int `yaml:"failing"`
	Reporting *int `yaml:"reporting"`
	// Report lists findings the check must report; Absent lists findings it
	// must not.
	Report []findingExpect `yaml:"report"`
	Absent []findingExpect `yaml:"absent"`
}

// findingExpect matches a finding on every field it sets.
type findingExpect struct {
	Rule      string `yaml:"rule"`
	Block     string `yaml:"block"`
	Quote     string `yaml:"quote"`
	Fails     *bool  `yaml:"fails"`
	Suggested *bool  `yaml:"suggested"`
	Xfail     string `yaml:"xfail"`
}

// fixExpect is what `kapi fix` does.
type fixExpect struct {
	// Args replace the file as the fix command's arguments; an empty list
	// that is set ([]) runs kapi fix over the whole project.
	Args *[]string `yaml:"args"`
	// After is the file's exact bytes once the fix has run. Unset, the file
	// must keep the bytes it had before.
	After *string `yaml:"after"`
	// Unchanged names other files whose bytes the fix must not move.
	Unchanged []string `yaml:"unchanged"`
	// Failing, when set, is how many failing findings a second check of the
	// file reports.
	Failing *int   `yaml:"failing"`
	Xfail   string `yaml:"xfail"`
}

// contextExpect is what `kapi context <path>` prints.
type contextExpect struct {
	Names []string `yaml:"names"`
	Omits []string `yaml:"omits"`
	Xfail string   `yaml:"xfail"`
}

// hookExpect is what the plugin's Stop hook decides.
type hookExpect struct {
	// Edit is applied before the hook runs: the assistant's last change.
	Edit *fileWrite `yaml:"edit"`
	// Blocks says whether the hook keeps the assistant working.
	Blocks bool `yaml:"blocks"`
	// Names are substrings the block reason must carry.
	Names []string `yaml:"names"`
	Xfail string   `yaml:"xfail"`
}

// checkReport is the part of the kapi.check/v2 report the suite reads.
type checkReport struct {
	Summary struct {
		Failing   int `json:"failing"`
		Reporting int `json:"reporting"`
	} `json:"summary"`
	Findings []struct {
		Rule      string `json:"rule"`
		Fails     bool   `json:"fails"`
		Suggested bool   `json:"suggested"`
		Message   string `json:"message"`
		Location  struct {
			Block   string `json:"block"`
			Snippet string `json:"snippet"`
		} `json:"location"`
	} `json:"findings"`
}

// Surfaces, in the order the matrix prints them.
const (
	surfaceCheck   = "check"
	surfaceFix     = "fix"
	surfaceContext = "context"
	surfaceHook    = "hook"
)

var gateSurfaces = []string{surfaceCheck, surfaceFix, surfaceContext, surfaceHook}

// gateOutcome is one surface's result for one case.
type gateOutcome struct {
	ran      bool
	failed   bool
	expected []string // reasons of the expected failures that failed
}

// gateMatrix collects the outcome of every case and surface.
type gateMatrix map[string]map[string]*gateOutcome

func (m gateMatrix) outcome(name, surface string) *gateOutcome {
	if m[name] == nil {
		m[name] = map[string]*gateOutcome{}
	}
	if m[name][surface] == nil {
		m[name][surface] = &gateOutcome{}
	}
	return m[name][surface]
}

// String renders the matrix: one line per case and surface.
func (m gateMatrix) String() string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("Gate conformance\n")
	for _, name := range names {
		for _, surface := range gateSurfaces {
			o := m[name][surface]
			var status string
			switch {
			case o == nil || !o.ran:
				status = "not covered"
			case o.failed:
				status = "fail"
			case len(o.expected) > 0:
				status = "expected-fail: " + strings.Join(uniqueStrings(o.expected), "; ")
			default:
				status = "pass"
			}
			fmt.Fprintf(&b, "  %-31s %-8s %s\n", name, surface, status)
		}
	}
	return b.String()
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// TestGateConformance runs every case under gate/testdata and prints the
// matrix. KAPI_GATE_MATRIX names a file the matrix is also written to, which
// is how `make gate-conformance` prints it.
func TestGateConformance(t *testing.T) {
	root := filepath.Join(filepath.Dir(testdata), "gate", "testdata")
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	matrix := gateMatrix{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		t.Run(name, func(t *testing.T) {
			runGateCase(t, filepath.Join(root, name), matrix.outcome, name)
		})
	}
	text := matrix.String()
	t.Log("\n" + text)
	if out := os.Getenv("KAPI_GATE_MATRIX"); out != "" {
		require.NoError(t, os.WriteFile(out, []byte(text), 0o644))
	}
}

// gateRun is one case's project and the environment its kapi runs in.
type gateRun struct {
	dir string
	env []string
}

func runGateCase(t *testing.T, caseDir string, outcome func(name, surface string) *gateOutcome, name string) {
	raw, err := os.ReadFile(filepath.Join(caseDir, "expect.yaml"))
	require.NoError(t, err)
	var gc gateCase
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	require.NoError(t, dec.Decode(&gc), "read %s/expect.yaml", name)
	require.NotEmpty(t, gc.Stages, "%s/expect.yaml names no stage", name)

	base := t.TempDir()
	run := gateRun{dir: filepath.Join(base, "project")}
	copyFixture(t, caseDir, run.dir)
	// Each case gets a workspace of its own on top of the suite's isolation,
	// so one case's terms and decisions never reach another's.
	home := filepath.Join(base, "home")
	run.env = append(append([]string{}, isoEnv...),
		"KAPI_CONFIG_DIR="+filepath.Join(home, "config"),
		"KAPI_DATA_DIR="+filepath.Join(home, "data", "kapi"),
		"XDG_DATA_HOME="+filepath.Join(home, "data"),
		"XDG_CACHE_HOME="+filepath.Join(home, "cache"),
	)

	for _, stage := range gc.Stages {
		t.Run(stage.Name, func(t *testing.T) {
			for i, step := range stage.Steps {
				run.step(t, i, step)
			}
			for _, fe := range stage.Files {
				if fe.Check != nil {
					run.checkFile(t, fe.Path, *fe.Check, outcome(name, surfaceCheck))
				}
				if fe.Fix != nil {
					run.fixFile(t, fe.Path, *fe.Fix, fe.Check, outcome(name, surfaceFix))
				}
				if fe.Context != nil {
					run.contextFile(t, fe.Path, *fe.Context, outcome(name, surfaceContext))
				}
			}
			if stage.Hook != nil {
				run.hook(t, *stage.Hook, outcome(name, surfaceHook))
			}
		})
	}
}

// copyFixture copies the case's project into dir, leaving expect.yaml out.
func copyFixture(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		if rel == "expect.yaml" {
			return nil
		}
		dst := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	require.NoError(t, err)
}

// kapi runs kapi in the project with the case's environment and extra
// variables, and returns its output and exit code. A run that could not start
// fails the test.
func (r gateRun) kapi(t *testing.T, extra []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(kapiBin, args...)
	cmd.Dir = r.dir
	cmd.Env = append(append(os.Environ(), r.env...), extra...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &ee):
		code = ee.ExitCode()
	default:
		require.NoError(t, err, "start kapi %s", strings.Join(args, " "))
	}
	return out.String(), errb.String(), code
}

// kapiOK runs kapi and fails the test on a non-zero exit.
func (r gateRun) kapiOK(t *testing.T, extra []string, args ...string) string {
	t.Helper()
	out, errOut, code := r.kapi(t, extra, args...)
	require.Zero(t, code, "kapi %s exited %d:\n%s%s", strings.Join(args, " "), code, out, errOut)
	return out
}

// actorEnv is the environment that makes kapi record an operation as the
// person or as an assistant.
func actorEnv(actor string) []string {
	switch actor {
	case "", "person":
		return []string{"KAPI_ACTOR=person"}
	case "agent":
		return []string{"KAPI_ACTOR=agent", "KAPI_AGENT_NAME=claude-code", "KAPI_AGENT_SESSION=gate-conformance"}
	}
	panic("unknown actor " + actor)
}

func (r gateRun) step(t *testing.T, i int, s gateStep) {
	t.Helper()
	switch {
	case s.Kapi != nil:
		r.kapiOK(t, actorEnv(s.Actor), append(append([]string{}, s.Kapi...), "-p", "kapi.yaml")...)
	case s.Keep != "":
		id := r.waitingDecision(t, s.Keep)
		args := []string{"rules", "--keep", id}
		if s.Use != "" {
			args = append(args, "--use", s.Use)
		}
		if s.Advisory {
			args = append(args, "--advisory")
		}
		r.kapiOK(t, actorEnv("person"), append(args, "-p", "kapi.yaml")...)
	case s.Narrow != "":
		id := r.keptRule(t, s.Narrow)
		r.kapiOK(t, actorEnv("person"), "rules", "narrow", id, "--to", s.To, "-p", "kapi.yaml")
	case s.Write != nil:
		r.write(t, *s.Write)
	default:
		t.Fatalf("step %d names no action", i)
	}
}

func (r gateRun) write(t *testing.T, w fileWrite) {
	t.Helper()
	path := filepath.Join(r.dir, filepath.FromSlash(w.Path))
	body := w.Content
	if w.Append != "" {
		before, err := os.ReadFile(path)
		require.NoError(t, err)
		body = string(before) + w.Append
	}
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}

// waitingDecision returns the id of the suggestion `kapi rules --json` lists
// with this sentence.
func (r gateRun) waitingDecision(t *testing.T, sentence string) string {
	t.Helper()
	out := r.kapiOK(t, actorEnv("person"), "rules", "--json", "-p", "kapi.yaml")
	var view struct {
		Decisions []struct {
			ID       string `json:"id"`
			Sentence string `json:"sentence"`
		} `json:"decisions"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &view), out)
	for _, d := range view.Decisions {
		if d.Sentence == sentence {
			return d.ID
		}
	}
	t.Fatalf("no waiting suggestion reads %q:\n%s", sentence, out)
	return ""
}

// keptRule returns the id of the rule `kapi rules list --json` lists with
// this sentence.
func (r gateRun) keptRule(t *testing.T, sentence string) string {
	t.Helper()
	out := r.kapiOK(t, actorEnv("person"), "rules", "list", "--json", "-p", "kapi.yaml")
	var listing struct {
		Groups []struct {
			Rules []struct {
				ID       string `json:"id"`
				Sentence string `json:"sentence"`
			} `json:"rules"`
		} `json:"groups"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &listing), out)
	for _, g := range listing.Groups {
		for _, rule := range g.Rules {
			if rule.Sentence == sentence {
				return rule.ID
			}
		}
	}
	t.Fatalf("no kept rule reads %q:\n%s", sentence, out)
	return ""
}

// check runs `kapi check <path> --json` and decodes the report. Exit 0 and
// exit 3 (a failing finding) are results; anything else fails the test.
func (r gateRun) check(t *testing.T, path string, args []string) (checkReport, string) {
	t.Helper()
	cmdArgs := append([]string{"check", path, "--json", "-p", "kapi.yaml"}, args...)
	out, errOut, code := r.kapi(t, nil, cmdArgs...)
	require.Contains(t, []int{0, 3}, code, "kapi check %s exited %d:\n%s%s", path, code, out, errOut)
	// A failing check prints the report and then the gate error object.
	var rep checkReport
	require.NoError(t, json.NewDecoder(strings.NewReader(out)).Decode(&rep), "decode the check report:\n%s", out)
	return rep, out
}

// expectation runs one named assertion as a subtest and records it on the
// surface. ok is whether the behaviour holds; xfail is the reason it is known
// not to. detail is printed when the assertion fails.
func expectation(t *testing.T, o *gateOutcome, name string, ok bool, xfail, detail string) {
	t.Helper()
	o.ran = true
	t.Run(subtestName(name), func(t *testing.T) {
		switch {
		case ok && xfail == "":
		case !ok && xfail != "":
			o.expected = append(o.expected, xfail)
			t.Logf("expected failure: %s", xfail)
		case ok:
			o.failed = true
			t.Errorf("%s holds now: remove the xfail (%s) from expect.yaml", name, xfail)
		default:
			o.failed = true
			t.Errorf("%s does not hold\n%s", name, detail)
		}
	})
}

func subtestName(s string) string {
	return strings.NewReplacer(" ", "_", "/", "|").Replace(s)
}

func describeFinding(f findingExpect) string {
	var parts []string
	if f.Rule != "" {
		parts = append(parts, f.Rule)
	}
	if f.Quote != "" {
		parts = append(parts, fmt.Sprintf("%q", f.Quote))
	}
	if f.Block != "" {
		parts = append(parts, "at "+f.Block)
	}
	if f.Fails != nil {
		if *f.Fails {
			parts = append(parts, "fails")
		} else {
			parts = append(parts, "reports")
		}
	}
	if f.Suggested != nil && *f.Suggested {
		parts = append(parts, "suggested")
	}
	return strings.Join(parts, " ")
}

// matches counts the findings in rep that f describes.
func (f findingExpect) matches(rep checkReport) int {
	n := 0
	for _, got := range rep.Findings {
		switch {
		case f.Rule != "" && got.Rule != f.Rule:
		case f.Block != "" && got.Location.Block != f.Block:
		case f.Quote != "" && got.Location.Snippet != f.Quote:
		case f.Fails != nil && got.Fails != *f.Fails:
		case f.Suggested != nil && got.Suggested != *f.Suggested:
		default:
			n++
		}
	}
	return n
}

func (r gateRun) checkFile(t *testing.T, path string, ce checkExpect, o *gateOutcome) {
	t.Helper()
	rep, raw := r.check(t, path, ce.Args)
	detail := "kapi check " + path + " --json:\n" + raw
	prefix := path + " "
	if ce.Failing != nil {
		expectation(t, o, fmt.Sprintf("%sfailing=%d", prefix, *ce.Failing), rep.Summary.Failing == *ce.Failing, "", detail)
	}
	if ce.Reporting != nil {
		expectation(t, o, fmt.Sprintf("%sreporting=%d", prefix, *ce.Reporting), rep.Summary.Reporting == *ce.Reporting, "", detail)
	}
	for _, f := range ce.Report {
		expectation(t, o, prefix+"reports "+describeFinding(f), f.matches(rep) > 0, f.Xfail, detail)
	}
	for _, f := range ce.Absent {
		expectation(t, o, prefix+"does not report "+describeFinding(f), f.matches(rep) == 0, f.Xfail, detail)
	}
}

func (r gateRun) read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(r.dir, filepath.FromSlash(path)))
	require.NoError(t, err)
	return string(b)
}

func (r gateRun) fixFile(t *testing.T, path string, fe fixExpect, ce *checkExpect, o *gateOutcome) {
	t.Helper()
	before := r.read(t, path)
	others := map[string]string{}
	for _, p := range fe.Unchanged {
		others[p] = r.read(t, p)
	}
	args := []string{"fix", path}
	if fe.Args != nil {
		args = append([]string{"fix"}, *fe.Args...)
	}
	out, errOut, code := r.kapi(t, nil, append(args, "-p", "kapi.yaml")...)
	// kapi fix exits 1 when a fix did not land, which is a result here.
	require.Contains(t, []int{0, 1}, code, "kapi %s exited %d:\n%s%s", strings.Join(args, " "), code, out, errOut)
	got := r.read(t, path)
	fixOut := "kapi " + strings.Join(args, " ") + ":\n" + out + errOut

	want, what := before, path+" keeps its bytes"
	if fe.After != nil {
		want, what = *fe.After, path+" reads as expected after the fix"
	}
	expectation(t, o, what, got == want, fe.Xfail,
		fmt.Sprintf("%s\nwant:\n%s\ngot:\n%s", fixOut, want, got))
	for _, p := range fe.Unchanged {
		expectation(t, o, p+" keeps its bytes", r.read(t, p) == others[p], "", fixOut)
	}
	if fe.Failing != nil {
		var args []string
		if ce != nil {
			args = ce.Args
		}
		rep, raw := r.check(t, path, args)
		expectation(t, o, fmt.Sprintf("%s failing=%d after the fix", path, *fe.Failing),
			rep.Summary.Failing == *fe.Failing, fe.Xfail, fixOut+"\nkapi check "+path+" --json:\n"+raw)
	}
}

func (r gateRun) contextFile(t *testing.T, path string, ce contextExpect, o *gateOutcome) {
	t.Helper()
	out := r.kapiOK(t, nil, "context", path, "-p", "kapi.yaml")
	detail := "kapi context " + path + ":\n" + out
	for _, n := range ce.Names {
		expectation(t, o, path+" context names "+n, strings.Contains(out, n), ce.Xfail, detail)
	}
	for _, n := range ce.Omits {
		expectation(t, o, path+" context omits "+n, !strings.Contains(out, n), ce.Xfail, detail)
	}
}

// stopHookCommand reads the command the kapi Claude Code plugin registers
// for the Stop event.
func stopHookCommand(t *testing.T) string {
	t.Helper()
	path := filepath.Join(findRoot(), "packages", "kapi-claude-plugin", "plugins", "kapi", "hooks", "hooks.json")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var cfg struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(raw, &cfg))
	for _, group := range cfg.Hooks["Stop"] {
		for _, h := range group.Hooks {
			if h.Type == "command" {
				return h.Command
			}
		}
	}
	t.Fatalf("%s registers no Stop command", path)
	return ""
}

// hook runs the plugin's Stop command the way Claude Code runs it: through
// the shell, with kapi resolved on PATH, the Stop event's JSON on stdin and
// the session's directory as the payload's cwd.
func (r gateRun) hook(t *testing.T, he hookExpect, o *gateOutcome) {
	t.Helper()
	if he.Edit != nil {
		r.write(t, *he.Edit)
	}
	bin := t.TempDir()
	require.NoError(t, os.Symlink(kapiBin, filepath.Join(bin, "kapi")))
	payload, err := json.Marshal(map[string]any{
		"session_id":       "gate-conformance",
		"transcript_path":  filepath.Join(bin, "transcript.jsonl"),
		"cwd":              r.dir,
		"permission_mode":  "default",
		"hook_event_name":  "Stop",
		"stop_hook_active": false,
	})
	require.NoError(t, err)

	command := stopHookCommand(t)
	cmd := exec.Command("/bin/sh", "-c", command)
	// The hook starts outside the project, as Claude Code's may, and finds it
	// through the payload's cwd. Project discovery is on for this one run:
	// the case's project is the only recipe above that directory.
	cmd.Dir = bin
	cmd.Env = append(append(os.Environ(), r.env...),
		"KAPI_NO_PROJECT=",
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Stdin = bytes.NewReader(payload)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	runErr := cmd.Run()
	detail := fmt.Sprintf("%s (exit %v)\nstdout:\n%s\nstderr:\n%s", command, runErr, out.String(), errb.String())

	expectation(t, o, "hook exits 0", runErr == nil, "", detail)
	var dec struct {
		Decision      string `json:"decision"`
		Reason        string `json:"reason"`
		SystemMessage string `json:"systemMessage"`
	}
	if s := strings.TrimSpace(out.String()); s != "" {
		if err := json.Unmarshal([]byte(s), &dec); err != nil {
			dec.SystemMessage = "stdout is not JSON: " + s
		}
	}
	expectation(t, o, "hook ran its gates", dec.SystemMessage == "", "", detail)
	if he.Blocks {
		expectation(t, o, "hook blocks the stop", dec.Decision == "block", he.Xfail, detail)
		for _, n := range he.Names {
			expectation(t, o, "hook reason names "+n, strings.Contains(dec.Reason, n), he.Xfail, detail)
		}
	} else {
		expectation(t, o, "hook lets the assistant stop", dec.Decision == "" && out.Len() == 0, he.Xfail, detail)
	}
}

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProbeVersion identifies the report shape and the scoring rules below.
const ProbeVersion = 2

// Outcome is what one rung, or one rung test, came to. Only Met awards a rung.
type Outcome string

const (
	Met       Outcome = "met"
	NotMet    Outcome = "not-met"
	DidNotRun Outcome = "did-not-run"
)

// Presence says whether the build reads a subject at all.
//
// A format is present by registration: it is in the maturity universe because
// it has a reader. A language's presence comes from its presence tests,
// TestProseP0_<id>, which assert that the build contains a reader for it: a
// provider in the comment registry, a grammar in a plugin's list. Every
// presence test passing is present, and no presence test is absent. A presence
// test that fails, or that did not run, is did-not-run with its output kept in
// the reason: a failure cannot tell a removed reader from a broken test, and
// the test's own assertion is what turns CI red when a reader goes. Rung tests
// above P0 never establish presence, because a package can pass its P1 test
// while no binary links it.
type Presence string

const (
	Present          Presence = "present"
	Absent           Presence = "absent"
	PresenceUnproven Presence = "did-not-run"
)

// Subject kinds.
const (
	KindFormat   = "format"
	KindLanguage = "language"
)

// Test results, the reason recorded beside each outcome.
const (
	resultPassed          = "passed"
	resultFailed          = "failed"
	resultSkipped         = "skipped"
	resultSubtestsSkipped = "every subtest skipped"
	resultBuildFailed     = "build failed"
	resultNoResult        = "no result"
	reasonNoTest          = "no test"
	reasonNoPresenceTest  = "no presence test"
)

// outputLines is how much of a test's output a report keeps.
const outputLines = 12

// Rungs is the ladder above P0. P0 has no requirement, so a P0 test is
// presence evidence for a language and awards nothing.
var Rungs = []string{"P1", "P2", "P3", "P4"}

// canaryPrefix marks the fixture subjects in ./canary. Every one of them is
// scored on every run, and none is ever reported as a subject.
const canaryPrefix = "canary"

// canaryPackage is where the canaries may live. A test naming a canary
// anywhere else is refused, so a real package cannot borrow the exemption.
const canaryPackage = "github.com/neokapi/neokapi/scripts/proseprobe/canary"

// canaryEnv is set on every `go test` the probe starts. The canaries read it
// so that their deliberately failing tests fail only under the probe and skip
// under an ordinary `go test ./...`.
const canaryEnv = "PROSE_PROBE_CANARY"

// canaryBrokenMessage is the assertion TestProseP0_canarybroken fails with.
const canaryBrokenMessage = "canary: the presence assertion failed"

// expectedRung is one rung of a canary's only acceptable scoring.
type expectedRung struct {
	Outcome Outcome
	Reason  string
}

var unclaimed = expectedRung{NotMet, reasonNoTest}

// canarySpec is the only acceptable scoring of one canary subject.
type canarySpec struct {
	Presence Presence
	Level    string
	// Reason must appear in the subject's presence reason.
	Reason string
	Rungs  map[string]expectedRung
}

// canaries is what each fixture subject in ./canary must score. Each one is a
// way a rung or a presence could be awarded without evidence.
var canaries = map[string]canarySpec{
	// A skipped P1, a pass above that missing rung, a failure, and a pass whose
	// only subtest skipped. Present through its presence test, so exactly P0.
	"canary": {Present, "P0", "TestProseP0_canary passed", map[string]expectedRung{
		"P1": {DidNotRun, resultSkipped},
		"P2": {Met, resultPassed},
		"P3": {NotMet, resultFailed},
		"P4": {DidNotRun, resultSubtestsSkipped},
	}},
	// A passing P1 with no presence test: a package that passes its rung tests
	// while no binary links it. It must not be present.
	"canaryunlinked": {Absent, "", reasonNoPresenceTest, map[string]expectedRung{
		"P1": {Met, resultPassed}, "P2": unclaimed, "P3": unclaimed, "P4": unclaimed,
	}},
	// Present, with a P1 that fails. It must score exactly P0.
	"canarypresent": {Present, "P0", "TestProseP0_canarypresent passed", map[string]expectedRung{
		"P1": {NotMet, resultFailed}, "P2": unclaimed, "P3": unclaimed, "P4": unclaimed,
	}},
	// A presence test that fails, beside a passing P1. It must come to
	// did-not-run with the failure's output in its reason, never to absent.
	"canarybroken": {PresenceUnproven, "", canaryBrokenMessage, map[string]expectedRung{
		"P1": {Met, resultPassed}, "P2": unclaimed, "P3": unclaimed, "P4": unclaimed,
	}},
}

// A name the loose pattern finds is a claim on the Prose axis. The strict
// pattern is the naming contract. A claim that breaks the contract is reported
// rather than ignored, so a misspelt rung test cannot silently count for
// nothing.
var (
	looseTestRe  = regexp.MustCompile(`(?m)^func\s+(TestProseP[0-9]\w*)\s*\(`)
	strictTestRe = regexp.MustCompile(`^TestProseP([0-4])_([a-z][a-z0-9]*)$`)
)

// Module is one Go module the probe searches.
type Module struct {
	// Dir is relative to the repository root, slash-separated.
	Dir string
	// Path is the module path from go.mod.
	Path string
	// Workspace is true for a module go.work names. The plugin modules sit
	// outside the workspace and are built with GOWORK=off, as their make
	// targets build them.
	Workspace bool
}

// RungTest is one discovered test and what it came to.
type RungTest struct {
	Module  string `json:"module"`
	Package string `json:"package"`
	Name    string `json:"name"`
	Result  string `json:"result"`
	// Output is the tail of what the test printed when it did not pass: its
	// assertions, or the compiler's output when its package did not build.
	Output string `json:"output,omitempty"`

	subject string
	rung    string
}

// RungResult is one rung of one subject.
type RungResult struct {
	Outcome Outcome    `json:"outcome"`
	Reason  string     `json:"reason"`
	Tests   []RungTest `json:"tests,omitempty"`
}

// Subject is one row of the axis.
type Subject struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Name     string   `json:"name,omitempty"`
	Provider string   `json:"provider,omitempty"`
	Presence Presence `json:"presence"`
	// PresenceReason says why a language has the presence it has, carrying a
	// failing presence test's output.
	PresenceReason string `json:"presence_reason,omitempty"`
	// Level is empty when the subject is absent or its presence did not run.
	Level         string                `json:"level,omitempty"`
	Rungs         map[string]RungResult `json:"rungs"`
	PresenceTests []RungTest            `json:"presence_tests,omitempty"`
	// Languages lists the registered languages a format reads. Each is scored
	// on its own row, because the axis is scored per language.
	Languages []string `json:"languages,omitempty"`
}

// ModuleRun records one `go test` invocation.
type ModuleRun struct {
	Dir      string `json:"dir"`
	Packages int    `json:"packages"`
	Tests    int    `json:"tests"`
	// Error is set when the go command produced no test events at all, so
	// every test in the module did not run.
	Error string `json:"error,omitempty"`
}

// Report is the probe's output. A report exists only for a valid run.
type Report struct {
	ProbeVersion int         `json:"probe_version"`
	Tags         string      `json:"tags"`
	Modules      []ModuleRun `json:"modules"`
	Canaries     []Subject   `json:"canaries"`
	Subjects     []Subject   `json:"subjects"`
}

// GoRunner runs the go command in dir and returns its standard output and
// standard error. A non-zero exit is not an error to the probe: failing tests
// exit non-zero and are read from the event stream like any other result.
type GoRunner func(ctx context.Context, dir string, env, args []string) (stdout, stderr []byte, err error)

// ExecGo is the GoRunner that starts the real go command.
func ExecGo(ctx context.Context, dir string, env, args []string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	cmd.Env = env
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	if _, ok := errors.AsType[*exec.ExitError](err); ok {
		err = nil
	}
	return out.Bytes(), errOut.Bytes(), err
}

// Options configures a probe run.
type Options struct {
	Root    string
	Tags    string
	Timeout string
	Run     GoRunner
}

// Probe discovers, runs and scores every rung test under opts.Root. The
// returned problems make the run invalid: a canary that did not score exactly
// as built, a rung test that breaks the naming contract, or a rung test for a
// subject that is neither a format nor a registered language.
func Probe(ctx context.Context, opts Options) (*Report, []string, error) {
	mods, err := DiscoverModules(opts.Root)
	if err != nil {
		return nil, nil, err
	}
	formats, err := RealFormatDirs(opts.Root)
	if err != nil {
		return nil, nil, err
	}
	langs, err := LoadLanguages(opts.Root)
	if err != nil {
		return nil, nil, err
	}

	tests, problems, err := DiscoverTests(opts.Root, mods)
	if err != nil {
		return nil, nil, err
	}
	isFormat := map[string]bool{}
	for _, f := range formats {
		isFormat[f] = true
	}
	for _, t := range tests {
		_, isCanary := canaries[t.subject]
		switch {
		case strings.HasPrefix(t.subject, canaryPrefix) && t.Package != canaryPackage:
			problems = append(problems, fmt.Sprintf("%s in %s names a canary subject, which may only live in %s", t.Name, t.Package, canaryPackage))
		case strings.HasPrefix(t.subject, canaryPrefix) && !isCanary:
			problems = append(problems, fmt.Sprintf("%s names %q, which is not one of the probe's canaries", t.Name, t.subject))
		case !strings.HasPrefix(t.subject, canaryPrefix) && !isFormat[t.subject] && langs[t.subject] == (Language{}):
			problems = append(problems, fmt.Sprintf("%s in %s names %q, which is neither a format under core/formats nor a language in core/formats/prose.yaml", t.Name, t.Package, t.subject))
		}
	}

	runs, err := runTests(ctx, opts, mods, tests)
	if err != nil {
		return nil, nil, err
	}

	bySubject := map[string][]RungTest{}
	for _, t := range tests {
		bySubject[t.subject] = append(bySubject[t.subject], t)
	}

	rep := &Report{ProbeVersion: ProbeVersion, Tags: opts.Tags, Modules: runs}
	for _, id := range canaryIDs() {
		c := Score(id, KindLanguage, bySubject[id])
		rep.Canaries = append(rep.Canaries, c)
		problems = append(problems, CheckCanary(c)...)
	}

	ids := make([]string, 0, len(langs))
	for id := range langs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	provides := map[string][]string{}
	for _, id := range ids {
		if p := langs[id].Provider; p != "" {
			provides[p] = append(provides[p], id)
		}
	}

	for _, f := range formats {
		s := Score(f, KindFormat, bySubject[f])
		s.Languages = provides[f]
		rep.Subjects = append(rep.Subjects, s)
	}
	for _, id := range ids {
		s := Score(id, KindLanguage, bySubject[id])
		s.Name = langs[id].Name
		s.Provider = langs[id].Provider
		rep.Subjects = append(rep.Subjects, s)
	}
	return rep, problems, nil
}

func canaryIDs() []string {
	ids := make([]string, 0, len(canaries))
	for id := range canaries {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// DiscoverModules returns the modules go.work names, followed by every plugin
// module under plugins/.
func DiscoverModules(root string) ([]Module, error) {
	work, err := os.ReadFile(filepath.Join(root, "go.work"))
	if err != nil {
		return nil, fmt.Errorf("read go.work: %w", err)
	}
	var mods []Module
	for _, dir := range parseWorkUse(string(work)) {
		p, err := modulePath(filepath.Join(root, filepath.FromSlash(dir)))
		if err != nil {
			return nil, err
		}
		mods = append(mods, Module{Dir: dir, Path: p, Workspace: true})
	}
	plugins, err := filepath.Glob(filepath.Join(root, "plugins", "*", "go.mod"))
	if err != nil {
		return nil, err
	}
	sort.Strings(plugins)
	for _, gomod := range plugins {
		dir := filepath.Dir(gomod)
		p, err := modulePath(dir)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			return nil, err
		}
		mods = append(mods, Module{Dir: filepath.ToSlash(rel), Path: p})
	}
	return mods, nil
}

// parseWorkUse returns the directories of every use directive, in both the
// single-line and the block form.
func parseWorkUse(work string) []string {
	var dirs []string
	inBlock := false
	for line := range strings.SplitSeq(work, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		switch {
		case inBlock && line == ")":
			inBlock = false
		case inBlock && line != "":
			dirs = append(dirs, cleanUse(line))
		case line == "use (":
			inBlock = true
		case strings.HasPrefix(line, "use "):
			dirs = append(dirs, cleanUse(strings.TrimPrefix(line, "use ")))
		}
	}
	return dirs
}

func cleanUse(s string) string {
	return path.Clean(strings.Trim(strings.TrimSpace(s), `"`))
}

func modulePath(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("read module: %w", err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`), nil
		}
	}
	return "", fmt.Errorf("%s/go.mod declares no module", dir)
}

// DiscoverTests finds every rung test in the modules' test files. It returns
// the tests that meet the naming contract and a problem for each claim that
// does not.
//
// A file walk rather than `go list` finds a test the default build would not
// compile, such as one behind a build tag. That test then produces no result
// and is reported as did-not-run, where `go list` would have hidden it.
func DiscoverTests(root string, mods []Module) ([]RungTest, []string, error) {
	var tests []RungTest
	var problems []string
	for _, m := range mods {
		modDir := filepath.Join(root, filepath.FromSlash(m.Dir))
		err := filepath.WalkDir(modDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if p != modDir && skipDir(p, d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(d.Name(), "_test.go") {
				return nil
			}
			src, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(modDir, filepath.Dir(p))
			if err != nil {
				return err
			}
			pkg := m.Path
			if rel != "." {
				pkg = m.Path + "/" + filepath.ToSlash(rel)
			}
			for _, match := range looseTestRe.FindAllStringSubmatch(string(src), -1) {
				name := match[1]
				sm := strictTestRe.FindStringSubmatch(name)
				if sm == nil {
					problems = append(problems, fmt.Sprintf("%s in %s claims the Prose axis but does not match TestProseP<0-4>_<subject>, where the subject is lowercase letters and digits", name, pkg))
					continue
				}
				tests = append(tests, RungTest{
					Module: m.Dir, Package: pkg, Name: name,
					subject: sm[2], rung: "P" + sm[1],
				})
			}
			return nil
		})
		if err != nil {
			return nil, nil, fmt.Errorf("walk %s: %w", m.Dir, err)
		}
	}
	return tests, problems, nil
}

// skipDir reports the directories a Go build never compiles tests from, and
// nested modules, which are searched as modules of their own.
func skipDir(p, name string) bool {
	switch {
	case strings.HasPrefix(name, "."), strings.HasPrefix(name, "_"),
		name == "testdata", name == "vendor", name == "node_modules":
		return true
	}
	_, err := os.Stat(filepath.Join(p, "go.mod"))
	return err == nil
}

// runTests starts one `go test` per module that holds rung tests and records
// each test's result, and the output of each test that did not pass, on it.
func runTests(ctx context.Context, opts Options, mods []Module, tests []RungTest) ([]ModuleRun, error) {
	var runs []ModuleRun
	for _, m := range mods {
		var idx []int
		pkgs := map[string]bool{}
		names := map[string]bool{}
		for i, t := range tests {
			if t.Module == m.Dir {
				idx = append(idx, i)
				pkgs[t.Package] = true
				names[t.Name] = true
			}
		}
		if len(idx) == 0 {
			continue
		}
		args := []string{"test", "-json", "-count=1"}
		if opts.Timeout != "" {
			args = append(args, "-timeout", opts.Timeout)
		}
		if m.Workspace && opts.Tags != "" {
			args = append(args, "-tags", opts.Tags)
		}
		args = append(args, "-run", "^("+strings.Join(sortedKeys(names), "|")+")$")
		for _, p := range sortedKeys(pkgs) {
			args = append(args, "./"+strings.TrimPrefix(strings.TrimPrefix(p, m.Path), "/"))
		}
		env := append(os.Environ(), canaryEnv+"=1")
		if !m.Workspace {
			env = append(env, "GOWORK=off", "CGO_ENABLED=1")
		}
		stdout, stderr, err := opts.Run(ctx, filepath.Join(opts.Root, filepath.FromSlash(m.Dir)), env, args)
		if err != nil {
			return nil, fmt.Errorf("run go test in %s: %w", m.Dir, err)
		}
		results, events := FoldEvents(bytes.NewReader(stdout))
		run := ModuleRun{Dir: m.Dir, Packages: len(pkgs), Tests: len(idx)}
		if events == 0 {
			run.Error = lastLines(string(stderr), 5)
		}
		for _, i := range idx {
			pr := results[tests[i].Package]
			tests[i].Result = Classify(pr, tests[i].Name)
			if tests[i].Result == resultPassed {
				continue
			}
			tests[i].Output = TestOutput(pr, tests[i].Name, tests[i].Result)
			if tests[i].Output == "" {
				tests[i].Output = run.Error
			}
		}
		runs = append(runs, run)
	}
	return runs, nil
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// PackageResult is what one package's tests came to.
type PackageResult struct {
	BuildFailed bool
	// Tests maps a test name, subtests included, to its terminal action:
	// pass, fail or skip.
	Tests map[string]string
	// Output maps a top-level test name to the lines it and its subtests
	// printed.
	Output map[string][]string
	// BuildOutput is what the compiler printed when the package did not build.
	BuildOutput []string
}

type testEvent struct {
	Action      string
	Package     string
	ImportPath  string
	Test        string
	Output      string
	FailedBuild string
}

// FoldEvents reads a `go test -json` stream into per-package results, and
// counts the events it understood. Lines that are not events are ignored.
func FoldEvents(r io.Reader) (map[string]*PackageResult, int) {
	out := map[string]*PackageResult{}
	result := func(pkg string) *PackageResult {
		pr := out[pkg]
		if pr == nil {
			pr = &PackageResult{Tests: map[string]string{}, Output: map[string][]string{}}
			out[pkg] = pr
		}
		return pr
	}
	events := 0
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			var ev testEvent
			if json.Unmarshal(line, &ev) == nil && ev.Action != "" {
				events++
				if ev.Action == "build-output" && ev.ImportPath != "" {
					// "pkg [pkg.test]" names the package whose test binary failed.
					pkg, _, _ := strings.Cut(ev.ImportPath, " ")
					pr := result(pkg)
					pr.BuildOutput = append(pr.BuildOutput, ev.Output)
				}
				if ev.Package != "" {
					pr := result(ev.Package)
					switch {
					case ev.Test == "" && ev.Action == "fail" && ev.FailedBuild != "":
						pr.BuildFailed = true
					case ev.Test != "" && (ev.Action == "pass" || ev.Action == "fail" || ev.Action == "skip"):
						pr.Tests[ev.Test] = ev.Action
					case ev.Test != "" && ev.Action == "output":
						top, _, _ := strings.Cut(ev.Test, "/")
						pr.Output[top] = append(pr.Output[top], ev.Output)
					}
				}
			}
		}
		if err != nil {
			return out, events
		}
	}
}

// Classify turns one test's events into a result. A pass counts only when a
// test did its own checking: a parent test whose every leaf subtest skipped
// reports pass to `go test`, and did not run here.
func Classify(pr *PackageResult, name string) string {
	if pr == nil {
		return resultNoResult
	}
	switch pr.Tests[name] {
	case "pass":
		if everyLeafSkipped(pr, name) {
			return resultSubtestsSkipped
		}
		return resultPassed
	case "fail":
		return resultFailed
	case "skip":
		return resultSkipped
	}
	if pr.BuildFailed {
		return resultBuildFailed
	}
	return resultNoResult
}

// TestOutput is the tail of what a test that did not pass printed, with the
// runner's own framing lines dropped. For a package that did not build it is
// the compiler's output.
func TestOutput(pr *PackageResult, name, result string) string {
	if pr == nil {
		return ""
	}
	lines := pr.Output[name]
	if result == resultBuildFailed {
		lines = pr.BuildOutput
	}
	var kept []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" || l == "PASS" || l == "FAIL" || strings.HasPrefix(l, "=== ") || strings.HasPrefix(l, "--- ") {
			continue
		}
		kept = append(kept, l)
	}
	if len(kept) > outputLines {
		kept = kept[len(kept)-outputLines:]
	}
	return strings.Join(kept, "\n")
}

func everyLeafSkipped(pr *PackageResult, name string) bool {
	prefix := name + "/"
	var descendants []string
	for t := range pr.Tests {
		if strings.HasPrefix(t, prefix) {
			descendants = append(descendants, t)
		}
	}
	leaves := 0
	for _, d := range descendants {
		leaf := true
		for _, other := range descendants {
			if strings.HasPrefix(other, d+"/") {
				leaf = false
				break
			}
		}
		if !leaf {
			continue
		}
		leaves++
		if pr.Tests[d] != "skip" {
			return false
		}
	}
	return leaves > 0
}

// OutcomeOf maps a test result to the outcome it gives a rung.
func OutcomeOf(result string) Outcome {
	switch result {
	case resultPassed:
		return Met
	case resultFailed:
		return NotMet
	default:
		return DidNotRun
	}
}

// FoldRung combines the tests claiming one rung. The rung is met only when
// every one of them passed. One failure makes it not met, and otherwise any
// test that did not run makes the rung did-not-run. A rung no test claims is
// not met.
func FoldRung(tests []RungTest) RungResult {
	if len(tests) == 0 {
		return RungResult{Outcome: NotMet, Reason: reasonNoTest}
	}
	res := RungResult{Outcome: Met, Reason: resultPassed, Tests: tests}
	for _, t := range tests {
		switch OutcomeOf(t.Result) {
		case NotMet:
			res.Outcome, res.Reason = NotMet, t.Result
			return res
		case DidNotRun:
			if res.Outcome == Met {
				res.Outcome, res.Reason = DidNotRun, t.Result
			}
		}
	}
	return res
}

// Level is the highest rung reached with every rung beneath it met. A rung
// met above an unmet one awards nothing.
func Level(rungs map[string]RungResult) string {
	level := "P0"
	for _, r := range Rungs {
		if rungs[r].Outcome != Met {
			break
		}
		level = r
	}
	return level
}

// Score builds one subject's row from the rung tests naming it.
func Score(id, kind string, tests []RungTest) Subject {
	s := Subject{ID: id, Kind: kind, Rungs: map[string]RungResult{}}
	byRung := map[string][]RungTest{}
	for _, t := range tests {
		if t.rung == "P0" {
			s.PresenceTests = append(s.PresenceTests, t)
			continue
		}
		byRung[t.rung] = append(byRung[t.rung], t)
	}
	for _, r := range Rungs {
		s.Rungs[r] = FoldRung(byRung[r])
	}

	s.Presence = Present
	if kind == KindLanguage {
		s.Presence, s.PresenceReason = languagePresence(s.PresenceTests)
	}
	if s.Presence == Present {
		s.Level = Level(s.Rungs)
	}
	return s
}

// languagePresence folds a language's presence tests into its presence and the
// reason for it. Only a pass is present. A presence test that failed or did not
// run leaves the language did-not-run, and its output goes into the reason so a
// broken test reads differently from a language with no presence test at all.
func languagePresence(p0 []RungTest) (Presence, string) {
	if len(p0) == 0 {
		return Absent, reasonNoPresenceTest
	}
	var passed []string
	for _, t := range p0 {
		if OutcomeOf(t.Result) != Met {
			reason := t.Name + " " + t.Result
			if t.Output != "" {
				reason += ": " + t.Output
			}
			return PresenceUnproven, reason
		}
		passed = append(passed, t.Name)
	}
	return Present, strings.Join(passed, ", ") + " passed"
}

// CheckCanary returns a problem for every way a canary's scoring departs from
// the one acceptable result for that canary.
func CheckCanary(c Subject) []string {
	want, ok := canaries[c.ID]
	if !ok {
		return []string{fmt.Sprintf("%q is not one of the probe's canaries", c.ID)}
	}
	var problems []string
	if c.Presence != want.Presence {
		problems = append(problems, fmt.Sprintf("%s presence came to %s; it must come to %s", c.ID, c.Presence, want.Presence))
	}
	if c.Level != want.Level {
		problems = append(problems, fmt.Sprintf("%s scored %q; it must score %q", c.ID, c.Level, want.Level))
	}
	if !strings.Contains(c.PresenceReason, want.Reason) {
		problems = append(problems, fmt.Sprintf("%s presence reason %q does not carry %q", c.ID, c.PresenceReason, want.Reason))
	}
	for _, r := range Rungs {
		w := want.Rungs[r]
		got := c.Rungs[r]
		if got.Outcome != w.Outcome || got.Reason != w.Reason {
			problems = append(problems, fmt.Sprintf("%s %s came to %s (%s); it must come to %s (%s)", c.ID, r, got.Outcome, got.Reason, w.Outcome, w.Reason))
		}
	}
	return problems
}

// Language is one entry of core/formats/prose.yaml.
type Language struct {
	Name     string `yaml:"name"`
	Provider string `yaml:"provider"`
}

// LoadLanguages reads the languages the axis tracks without a format of their
// own. The registry adds rows and never a rung: a language it names with no
// presence test is reported absent.
func LoadLanguages(root string) (map[string]Language, error) {
	data, err := os.ReadFile(filepath.Join(root, "core", "formats", "prose.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read language registry: %w", err)
	}
	var doc struct {
		Languages map[string]Language `yaml:"languages"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse core/formats/prose.yaml: %w", err)
	}
	return doc.Languages, nil
}

// nonFormats mirrors the exclusions of realFormatDirs in
// core/formats/maturity_test.go and scripts/format-ops/lib.mjs.
var nonFormats = map[string]bool{"exec": true, "jsx": true, "memorytest": true}

// RealFormatDirs is the maturity universe: a directory under core/formats that
// ships a reader.go or a structure.yaml. It mirrors realFormatDirs in
// core/formats/maturity_test.go and scripts/format-ops/lib.mjs.
func RealFormatDirs(root string) ([]string, error) {
	base := filepath.Join(root, "core", "formats")
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, fmt.Errorf("read core/formats: %w", err)
	}
	var ids []string
	for _, e := range entries {
		if !e.IsDir() || nonFormats[e.Name()] {
			continue
		}
		for _, marker := range []string{"reader.go", "structure.yaml"} {
			if info, err := os.Stat(filepath.Join(base, e.Name(), marker)); err == nil && !info.IsDir() {
				ids = append(ids, e.Name())
				break
			}
		}
	}
	sort.Strings(ids)
	return ids, nil
}

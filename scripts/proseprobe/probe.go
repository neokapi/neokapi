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
const ProbeVersion = 1

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
// TestProseP0_<id>, which assert what the build contains: a provider in the
// comment registry, a grammar in a plugin's list. Every presence test passing
// is present, one failing is absent, one that did not run is did-not-run, and
// no presence test is absent. Rung tests above P0 never establish presence,
// because a package can pass its P1 test while no binary links it.
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
)

// Rungs is the ladder above P0. P0 has no requirement, so a P0 test is
// presence evidence for a language and awards nothing.
var Rungs = []string{"P1", "P2", "P3", "P4"}

// CanaryID names the fixture subject in ./canary. It is scored on every run
// and is never reported as a subject.
const CanaryID = "canary"

// canaryPackage is where the canary may live. A test naming the canary
// anywhere else is refused, so a real package cannot borrow its exemption.
const canaryPackage = "github.com/neokapi/neokapi/scripts/proseprobe/canary"

// canaryEnv is set on every `go test` the probe starts. The canary reads it so
// that its deliberately failing test fails only under the probe and skips
// under an ordinary `go test ./...`.
const canaryEnv = "PROSE_PROBE_CANARY"

// canaryExpected is the only acceptable scoring of the canary. Each rung is
// built to exercise one refusal: a skipped test, a pass above a missing rung,
// a failure, and a pass whose only subtest skipped.
var canaryExpected = map[string]struct {
	Outcome Outcome
	Reason  string
}{
	"P1": {DidNotRun, resultSkipped},
	"P2": {Met, resultPassed},
	"P3": {NotMet, resultFailed},
	"P4": {DidNotRun, resultSubtestsSkipped},
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
	Canary       Subject     `json:"canary"`
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
// as expected, a rung test that breaks the naming contract, or a rung test for
// a subject that is neither a format nor a registered language.
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
		switch {
		case t.subject == CanaryID && t.Package != canaryPackage:
			problems = append(problems, fmt.Sprintf("%s in %s names the canary, which may only live in %s", t.Name, t.Package, canaryPackage))
		case t.subject != CanaryID && !isFormat[t.subject] && langs[t.subject] == (Language{}):
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
	rep.Canary = Score(CanaryID, KindLanguage, bySubject[CanaryID])
	problems = append(problems, CheckCanary(rep.Canary)...)

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
// each test's result on it.
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
			tests[i].Result = Classify(results[tests[i].Package], tests[i].Name)
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
}

type testEvent struct {
	Action      string
	Package     string
	Test        string
	FailedBuild string
}

// FoldEvents reads a `go test -json` stream into per-package results, and
// counts the events it understood. Lines that are not events are ignored.
func FoldEvents(r io.Reader) (map[string]*PackageResult, int) {
	out := map[string]*PackageResult{}
	events := 0
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			var ev testEvent
			if json.Unmarshal(line, &ev) == nil && ev.Action != "" {
				events++
				if ev.Package != "" {
					pr := out[ev.Package]
					if pr == nil {
						pr = &PackageResult{Tests: map[string]string{}}
						out[ev.Package] = pr
					}
					switch {
					case ev.Test == "" && ev.Action == "fail" && ev.FailedBuild != "":
						pr.BuildFailed = true
					case ev.Test != "" && (ev.Action == "pass" || ev.Action == "fail" || ev.Action == "skip"):
						pr.Tests[ev.Test] = ev.Action
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
		s.Presence = languagePresence(s.PresenceTests)
	}
	if s.Presence == Present {
		s.Level = Level(s.Rungs)
	}
	return s
}

// languagePresence folds a language's presence tests. A failure means the
// build does not contain the reader the test looks for, so it outranks a test
// that did not run.
func languagePresence(p0 []RungTest) Presence {
	if len(p0) == 0 {
		return Absent
	}
	presence := Present
	for _, t := range p0 {
		switch OutcomeOf(t.Result) {
		case NotMet:
			return Absent
		case DidNotRun:
			presence = PresenceUnproven
		}
	}
	return presence
}

// CheckCanary returns a problem for every way the canary's scoring departs
// from the one acceptable result.
func CheckCanary(c Subject) []string {
	var problems []string
	if c.Presence != Present {
		problems = append(problems, fmt.Sprintf("canary presence is %s; its tests did not run, so this run proves nothing", c.Presence))
	}
	if c.Level != "P0" {
		problems = append(problems, fmt.Sprintf("canary scored %q; it must score P0", c.Level))
	}
	for _, r := range Rungs {
		want := canaryExpected[r]
		got := c.Rungs[r]
		if got.Outcome != want.Outcome || got.Reason != want.Reason {
			problems = append(problems, fmt.Sprintf("canary %s came to %s (%s); it must come to %s (%s)", r, got.Outcome, got.Reason, want.Outcome, want.Reason))
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
// own. The registry adds rows and never a rung: a language it names that no
// rung test covers is reported absent.
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

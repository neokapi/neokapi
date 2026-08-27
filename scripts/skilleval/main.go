// Command skilleval measures the shipped kapi Agent Skill: whether it fires on
// the tasks it should, stays quiet on the ones it should not, and drives the
// job to a green gate when it does fire.
//
// It runs on a maintainer's machine and never in CI. It drives a real
// interactive agent through `claude -p`, which costs money and needs local
// credentials, so the committed dataset is the only thing a build ever sees.
// That makes the date on the dataset the real currency of the numbers, and the
// dashboard says so rather than printing a score with no age.
//
//	make skill-eval                  # triggering, all scenarios, 3 repeats
//	make skill-eval-completion       # completion, positives only (slow, metered)
//	go run ./scripts/skilleval -only p04-cross-format-sweep -keep
//
// Triggering is stochastic. A single pass tells you almost nothing, which is
// why -repeat defaults above one and why the dashboard shows the spread rather
// than a lone boolean.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	modeTrigger    = "trigger"
	modeCompletion = "completion"
)

// DefaultOut is the committed dataset the dashboard reads.
const DefaultOut = "web/src/pages/skill-eval/_skilleval.json"

// Options is everything the run needs that is not a scenario.
type Options struct {
	Mode           string
	Surface        string
	RepoRoot       string
	ClaudeBin      string
	KapiBin        string
	Model          string
	Repeat         int
	Concurrency    int
	TriggerTurnCap int
	// Control runs the unaided arm alongside each scenario.
	Control bool
	// CompletionTurnFloor is the least room a completion run gets, whatever the
	// scenario's own cap says. Triggering and finishing are different budgets.
	CompletionTurnFloor int
	Keep                bool
}

func main() {
	var (
		mode        = flag.String("mode", modeTrigger, "trigger or completion")
		surface     = flag.String("surface", "", "limit to one surface: skill or mcp")
		out         = flag.String("out", DefaultOut, "where to write the dataset")
		only        = flag.String("only", "", "run one scenario by id")
		repeat      = flag.Int("repeat", 3, "passes per scenario; triggering is stochastic")
		concurrency = flag.Int("concurrency", 4, "scenarios in flight")
		model       = flag.String("model", "", "model for the driven agent (default: the CLI's own)")
		turnCap     = flag.Int("trigger-turns", 4, "hard turn cap in trigger mode")
		compTurns   = flag.Int("completion-turns", 40, "minimum turns a completion run gets")
		keep        = flag.Bool("keep", false, "keep the scenario workspaces for inspection")
		control     = flag.Bool("control", false, "also run each scenario with no skill and no kapi on PATH, to measure what kapi adds")
		timeout     = flag.Duration("timeout", 30*time.Minute, "whole-run deadline")
	)
	flag.Parse()

	if *mode != modeTrigger && *mode != modeCompletion {
		fail("mode must be trigger or completion")
	}
	root, err := repoRoot()
	if err != nil {
		fail(err.Error())
	}
	claudeBin, err := exec.LookPath("claude")
	if err != nil {
		fail("the claude CLI is not on PATH; this eval drives a real agent and cannot run without it")
	}

	opts := Options{
		Mode: *mode, Surface: *surface, RepoRoot: root, ClaudeBin: claudeBin,
		KapiBin: findKapi(root), Model: *model, Repeat: *repeat,
		Concurrency: *concurrency, TriggerTurnCap: *turnCap,
		CompletionTurnFloor: *compTurns, Keep: *keep, Control: *control,
	}
	if opts.Mode == modeCompletion && opts.KapiBin == "" {
		fail("completion mode needs a built kapi: run `make build` first")
	}

	set := selectScenarios(*only, opts.Mode, *surface)
	if len(set) == 0 {
		fail("no scenarios selected")
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	report := execute(ctx, set, opts)
	report.stamp(opts, claudeBin)

	target := *out
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		fail(err.Error())
	}
	if err := checkDownstreamClean(report); err != nil {
		fail(err.Error())
	}
	if err := merge(target, report); err != nil {
		fail(err.Error())
	}
	fmt.Printf("skilleval: %s\n", report.Summary.Line())
	fmt.Printf("skilleval: → %s\n", target)
}

// selectScenarios narrows to what this mode can actually score. Completion runs
// positives only: a negative that never fires has nothing to complete.
func selectScenarios(only, mode, surface string) []Scenario {
	var out []Scenario
	for _, sc := range scenarios {
		if only != "" && sc.ID != only {
			continue
		}
		if surface != "" && surfaceOf(sc) != surface {
			continue
		}
		if mode == modeCompletion && sc.Kind != positive {
			continue
		}
		// An MCP scenario is scored on which tool the agent picked, and that
		// is visible in trigger mode. Completion mode has nothing extra to
		// tell it, so it would only spend money to learn the same thing.
		if mode == modeCompletion && surfaceOf(sc) == surfaceMCP {
			continue
		}
		out = append(out, sc)
	}
	return out
}

// execute runs the selected scenarios, bounded by concurrency.
func execute(ctx context.Context, set []Scenario, opts Options) *Report {
	report := &Report{Mode: opts.Mode, Surface: opts.Surface, Repeat: opts.Repeat}
	results := make([]Result, len(set))

	sem := make(chan struct{}, max(1, opts.Concurrency))
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0

	for i := range set {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			sc := set[i]
			res := Result{Scenario: sc}
			for range opts.Repeat {
				if ctx.Err() != nil {
					break
				}
				res.Runs = append(res.Runs, runScenario(ctx, &sc, opts, armSkill))
			}
			if opts.Control {
				// One control pass rather than Repeat: the question it answers
				// is "could this be done without kapi at all", which does not
				// need the same statistical weight as "does the skill fire".
				if ctx.Err() == nil {
					res.Unaided = append(res.Unaided, runScenario(ctx, &sc, opts, armUnaided))
				}
			}
			res.Scenario = sc // fixture sizes were filled in during the run
			res.score(opts.Mode)
			results[i] = res

			mu.Lock()
			done++
			fmt.Fprintf(os.Stderr, "[%d/%d] %-26s %s\n", done, len(set), sc.ID, res.Verdict)
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	report.Results = results
	report.Summary = summarize(results)
	return report
}

// Report is the committed dataset.
type Report struct {
	Generated string   `json:"generated"`
	Mode      string   `json:"mode"`
	Surface   string   `json:"surface,omitempty"`
	Repeat    int      `json:"repeat"`
	Runner    Runner   `json:"runner"`
	Summary   Summary  `json:"summary"`
	Results   []Result `json:"results"`
}

// Key names this report in the committed dataset. Surface is part of it: a
// skill sweep and an MCP sweep are different evals, and keying on mode alone
// would let whichever ran last erase the other.
func (r *Report) Key() string {
	surface := r.Surface
	if surface == "" {
		surface = surfaceSkill
	}
	return surface + ":" + r.Mode
}

// Runner records what produced the numbers. Everything here is a field the
// evaluation-card literature finds missing almost everywhere, and without it a
// score is an assertion with a table around it.
type Runner struct {
	ClaudeVersion string `json:"claudeVersion"`
	Model         string `json:"model"`
	SkillCommit   string `json:"skillCommit"`
	SkillModified string `json:"skillModified"`
	Host          string `json:"host"`
	Settings      string `json:"settings"`
	// Kapi is the binary the driven agent could reach. It changes the result:
	// an agent that finds no kapi may give up before doing anything the
	// detector would score, so which build was on PATH is part of the run.
	Kapi        string `json:"kapi"`
	KapiVersion string `json:"kapiVersion"`
}

// Result is one scenario and every pass over it.
type Result struct {
	Scenario Scenario `json:"scenario"`
	Runs     []Run    `json:"runs"`
	// Fired counts passes where kapi was reached for at all.
	Fired int `json:"fired"`
	// FoundTool counts passes that reached the MCP tool the scenario names.
	// Only meaningful on an MCP scenario, and it is the number that matters
	// there: reaching kapi is free when the tool list is already in context.
	FoundTool int `json:"foundTool,omitempty"`
	// WrongTool lists MCP tools the agent reached for instead, deduplicated.
	// A near miss is the most informative result this eval produces, because it
	// names the two descriptions that are not telling each other apart.
	WrongTool []string `json:"wrongTool,omitempty"`
	// Verdict is pass, fail, flaky, "no gate" or "not run".
	//
	// Flaky is its own answer rather than a rounded pass: a skill that fires
	// two times in three is a different problem from one that always fires, and
	// averaging hides it.
	//
	// "no gate" is the other one that must not round up. In completion mode a
	// scenario with no gate has no definition of done, so nothing about it was
	// verified. Scoring those on triggering read as "17 pass" on a sweep where
	// only three scenarios were checked at all and two of the three failed.
	Verdict string `json:"verdict"`
	// GatePassed counts passes whose completion gate went green.
	GatePassed int `json:"gatePassed,omitempty"`

	// Unaided is the control arm: the same prompt and workspace with no skill,
	// no MCP server, and no kapi anywhere on PATH.
	//
	// It exists because a scenario note claimed of a .pptx that "the agent has
	// no other way to read it", and an unaided agent answered correctly in
	// three calls with unzip. A .pptx is a zip of XML. The suite was full of
	// assertions like that, and the only thing that settles them is the run.
	Unaided []Run `json:"unaided,omitempty"`
	// UnaidedGatePassed counts control passes whose gate went green.
	UnaidedGatePassed int `json:"unaidedGatePassed,omitempty"`
	// Contribution is what kapi added here, measured: enabled, eased, neither,
	// or unknown when there is no gate to compare outcomes on.
	Contribution Contribution `json:"contribution,omitempty"`
}

func (r *Result) score(mode string) {
	wrong := map[string]bool{}
	for _, run := range r.Runs {
		if run.Triggered {
			r.Fired++
		}
		if run.Gate != nil && run.Gate.ExitCode == 0 {
			r.GatePassed++
		}
		for _, tool := range run.MCPTools {
			switch {
			case tool == r.Scenario.ExpectTool:
				r.FoundTool++
			case !wrong[tool]:
				wrong[tool] = true
				r.WrongTool = append(r.WrongTool, tool)
			}
		}
	}
	sortStrings(r.WrongTool)

	r.UnaidedGatePassed = 0
	for _, run := range r.Unaided {
		if run.Gate != nil && run.Gate.ExitCode == 0 {
			r.UnaidedGatePassed++
		}
	}
	if len(r.Unaided) > 0 {
		r.Contribution = contribution(r.Runs, r.Unaided, r.Scenario.CompletionGate != "")
	}

	n := len(r.Runs)

	// Completion mode asks a different question, so it is scored on a different
	// number. Triggering is necessary and nowhere near sufficient: an agent
	// that reached for kapi and left the job half done has not completed it.
	if mode == modeCompletion {
		switch {
		case n == 0:
			r.Verdict = "not run"
		case r.Scenario.CompletionGate == "":
			// No definition of done, so nothing was verified. Saying "pass"
			// here would be the page's biggest lie.
			r.Verdict = "no gate"
		case r.GatePassed == n:
			r.Verdict = "pass"
		case r.GatePassed == 0:
			r.Verdict = "fail"
		default:
			r.Verdict = "flaky"
		}
		return
	}

	// On an MCP scenario the agent already holds the tool list, so "did it
	// reach kapi" is nearly free and says almost nothing. The scored quantity
	// is whether it reached the RIGHT tool.
	hits := r.Fired
	if surfaceOf(r.Scenario) == surfaceMCP && r.Scenario.ExpectTool != "" {
		hits = r.FoundTool
	}

	want := r.Scenario.Kind == positive
	switch {
	case n == 0:
		r.Verdict = "not run"
	case want && hits >= n, !want && hits == 0:
		r.Verdict = "pass"
	case want && hits == 0, !want && hits >= n:
		r.Verdict = "fail"
	default:
		r.Verdict = "flaky"
	}
}

func sortStrings(xs []string) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

// Summary is the top of the dashboard.
type Summary struct {
	Scenarios int `json:"scenarios"`
	Pass      int `json:"pass"`
	Flaky     int `json:"flaky"`
	Fail      int `json:"fail"`
	Positives int `json:"positives"`
	Negatives int `json:"negatives"`
	// FalseTriggers is the count that matters most on the negative side: a
	// skill that fires on a code task is worse than one that misses a positive,
	// because it costs every user of the assistant, not only kapi's.
	FalseTriggers int `json:"falseTriggers"`
	GatesRun      int `json:"gatesRun,omitempty"`
	GatesPassed   int `json:"gatesPassed,omitempty"`
	// WrongToolPicks counts MCP scenarios where the agent reached for kapi and
	// chose a tool other than the one the task called for.
	WrongToolPicks int `json:"wrongToolPicks,omitempty"`
	// Ungated counts scenarios completion mode could not check because they
	// carry no definition of done. It belongs beside the pass count, not
	// hidden behind it.
	Ungated int `json:"ungated,omitempty"`

	// Contributions counts what kapi added across the suite, when the control
	// arm ran. "neither" is the number worth reading first: it says how much
	// of this suite is not evidence for kapi.
	Contributions map[string]int `json:"contributions,omitempty"`
}

func (s Summary) Line() string {
	line := fmt.Sprintf("%d scenarios: %d pass, %d flaky, %d fail (%d false triggers)",
		s.Scenarios, s.Pass, s.Flaky, s.Fail, s.FalseTriggers)
	if s.Ungated > 0 {
		line += fmt.Sprintf(", %d with no gate to check", s.Ungated)
	}
	if n := s.Contributions; len(n) > 0 {
		line += fmt.Sprintf("; kapi enabled %d, eased %d, neither %d, unknown %d",
			n[string(ContributionEnabled)], n[string(ContributionEased)],
			n[string(ContributionNeither)], n[string(ContributionUnknown)])
	}
	return line
}

func summarize(rs []Result) Summary {
	var s Summary
	for _, r := range rs {
		s.Scenarios++
		switch r.Scenario.Kind {
		case positive:
			s.Positives++
		case negative:
			s.Negatives++
			if r.Fired > 0 {
				s.FalseTriggers++
			}
		}
		if r.Contribution != "" {
			if s.Contributions == nil {
				s.Contributions = map[string]int{}
			}
			s.Contributions[string(r.Contribution)]++
		}
		if surfaceOf(r.Scenario) == surfaceMCP && len(r.WrongTool) > 0 {
			s.WrongToolPicks++
		}
		switch r.Verdict {
		case "pass":
			s.Pass++
		case "flaky":
			s.Flaky++
		case "fail":
			s.Fail++
		case "no gate":
			s.Ungated++
		}
		for _, run := range r.Runs {
			if run.Gate != nil {
				s.GatesRun++
				if run.Gate.ExitCode == 0 {
					s.GatesPassed++
				}
			}
		}
	}
	return s
}

// stamp records the conditions of the run.
func (r *Report) stamp(opts Options, claudeBin string) {
	r.Generated = time.Now().UTC().Format(time.RFC3339)
	model := opts.Model
	if model == "" {
		model = "the claude CLI's default, not pinned"
	}
	r.Runner = Runner{
		ClaudeVersion: firstLine(run(claudeBin, "--version")),
		Model:         model,
		SkillCommit:   firstLine(run("git", "-C", opts.RepoRoot, "log", "-1", "--format=%h", "--", "cli/skills/data/kapi")),
		SkillModified: firstLine(run("git", "-C", opts.RepoRoot, "log", "-1", "--format=%ad", "--date=short", "--", "cli/skills/data/kapi")),
		Host:          firstLine(run("uname", "-sm")),
		Kapi:          strings.TrimPrefix(opts.KapiBin, opts.RepoRoot+"/"),
		KapiVersion:   scrubPaths(firstLine(run(opts.KapiBin, "version"))),
		Settings:      settingsLine(opts),
	}
}

// merge writes the new report while keeping the other mode's results, so a
// trigger sweep does not erase the completion numbers or the reverse.
func merge(target string, fresh *Report) error {
	combined := map[string]*Report{}
	if old, err := os.ReadFile(target); err == nil {
		var prior map[string]*Report
		if json.Unmarshal(old, &prior) == nil {
			// Reports written before the MCP surface existed are keyed by mode
			// alone. They were skill sweeps, so name them that way rather than
			// leaving the dataset with two spellings of the same thing.
			for k, r := range prior {
				if k == modeTrigger || k == modeCompletion {
					delete(prior, k)
					prior[surfaceSkill+":"+k] = r
				}
			}
			combined = prior
		}
	}
	// A partial run must not quietly shrink the published dataset. `-only` is
	// the everyday way to re-check one scenario, and writing its one result
	// over a full sweep leaves the dashboard showing a suite of one with
	// nothing to say the rest was ever measured.
	if prior, ok := combined[fresh.Key()]; ok && len(prior.Results) > len(fresh.Results) {
		return fmt.Errorf(
			"refusing to replace %d recorded scenarios with %d: re-run the full sweep, "+
				"or pass -out to write this partial run somewhere else",
			len(prior.Results), len(fresh.Results))
	}
	combined[fresh.Key()] = fresh

	data, err := json.MarshalIndent(combined, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(target, append(data, '\n'), 0o644)
}

func repoRoot() (string, error) {
	out := strings.TrimSpace(run("git", "rev-parse", "--show-toplevel"))
	if out == "" {
		return "", errors.New("not in a git checkout")
	}
	return out, nil
}

// findKapi prefers the repo's own build, so a run measures this checkout rather
// than whatever kapi the developer has installed.
func findKapi(root string) string {
	local := filepath.Join(root, "bin", "kapi")
	if _, err := os.Stat(local); err == nil {
		return local
	}
	if p, err := exec.LookPath("kapi"); err == nil {
		return p
	}
	return ""
}

// run captures a short command's stdout, or "" if it fails. Used only for the
// provenance strings on a report — a version, a commit — so a failure means the
// field is blank rather than that the run is in trouble.
func run(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func firstLine(s string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return first
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "skilleval:", msg)
	os.Exit(1)
}

// settingsLine records the budget the run actually had, because it is the
// difference between "the agent could not do it" and "the agent was stopped".
func settingsLine(opts Options) string {
	if opts.Mode == modeCompletion {
		return fmt.Sprintf("claude -p, bypassPermissions, %d repeat(s), at least %d turns per scenario. "+
			"Sampling follows the CLI's defaults and is not pinned.", opts.Repeat, opts.CompletionTurnFloor)
	}
	return fmt.Sprintf("claude -p, bypassPermissions, %d repeat(s), trigger cap %d turns "+
		"(MCP scenarios use their own, since picking a tool can take a step or two). "+
		"Sampling follows the CLI's defaults and is not pinned.", opts.Repeat, opts.TriggerTurnCap)
}

// downstreamName is the platform this framework must not mention in its own
// docs. See scripts/check-docs-bowrain-clean.sh for the contract.
const downstreamName = "bowrain"

// checkDownstreamClean refuses to publish a dataset that would fail the
// docs guard.
//
// The dataset lands in web/src, which the neokapi docs site serves and which is
// held to zero mentions of the downstream platform. Scenario text is under this
// file's control and a companion test keeps it clean, but a TRANSCRIPT is not:
// the shipped skill's own description names the platform, so an agent can
// repeat it in a closing message on any scenario at all. That is how this
// landed a red build once already.
//
// Failing here, naming the scenario, is the useful behaviour. Editing the
// transcript would be the alternative, and quietly rewriting recorded evidence
// to get past a lint is not something an evidence page should do.
func checkDownstreamClean(r *Report) error {
	body, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if !strings.Contains(strings.ToLower(string(body)), downstreamName) {
		return nil
	}

	var offenders []string
	for _, res := range r.Results {
		one, err := json.Marshal(res)
		if err != nil {
			continue
		}
		if strings.Contains(strings.ToLower(string(one)), downstreamName) {
			offenders = append(offenders, res.Scenario.ID)
		}
	}
	return fmt.Errorf(
		"this run mentions %q and the dataset is published into the docs site, which must not: %s.\n"+
			"Reword the scenario so the agent is not led there, or drop it from the set published here.\n"+
			"See scripts/check-docs-bowrain-clean.sh",
		downstreamName, strings.Join(offenders, ", "))
}

package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
)

// The comparison evaluation.
//
// It asks whether a project's context, held in kapi, makes coding agents write
// better than the same rules held the way a project without kapi holds them.
// The same writing tasks run in three arms, on each agent host, with the same
// prompt and model:
//
//   - kapi: the project as `kapi init --agents all` leaves it (the MCP entry
//     and the skill for each host), with the rules read into kapi's store by
//     a person. The context files are removed after the import, because a
//     kapi project keeps its context in the store, so the agent learns the
//     rules from kapi or from the files around it.
//   - kapifiles: the same kapi project, with the rules files kapi writes
//     (a delimited section of AGENTS.md and CLAUDE.md at the root and in each
//     folder whose rules differ) as `kapi init` and `kapi context sync
//     --files-only` leave them. The kapi arm opts out of them with
//     `--no-rules-files`, so the two differ only in those files.
//   - rulesfile: the same rules written as a style guide in CLAUDE.md and
//     AGENTS.md, the instruction files each host loads on its own. This is
//     the realistic alternative, and the strongest one available without
//     kapi: the host puts the file in every session's context.
//   - bare: the content files and nothing else, which measures what an agent
//     infers from the neighbouring files alone.
//
// Each project holds four kinds of rule: a rename in force in one part of the
// project and not another, the house voice, a competitor's name and banned
// claims, plus a few house words. The tasks are ordinary writing work (a help
// section, a release note, a support reply, interface strings, a rename
// update) whose prompts name no rule and no kapi surface, and several of
// which use the old or forbidden word themselves, as a person asking would.
//
// Scoring is separate from running. Deterministic graders count rule
// violations in the text each attempt added (compare_grade.go); `kapi check`
// over the same text is reported beside them. Two judges from different model
// families score voice fit against a yes/no rubric, and the judged score is
// reported as validated only when the judges agree at kappa >= 0.6
// (compare_judge.go). The report gives every rate with a 95% interval
// (compare_report.go).

const compareSchema = 1

// The arms.
const (
	compareArmKapi      = "kapi"
	compareArmKapiFiles = "kapifiles"
	compareArmRules     = "rulesfile"
	compareArmBare      = "bare"
)

var compareArms = []string{compareArmKapi, compareArmKapiFiles, compareArmRules, compareArmBare}

// compareKapiArm reports an arm whose cell is a kapi project.
func compareKapiArm(arm string) bool { return arm == compareArmKapi || arm == compareArmKapiFiles }

// The phases. Preflight prepares every cell of a phase and probes it with no
// model call; pilot and run start live sessions; grade, judge and report read
// what the sessions left.
const (
	comparePhasePreflight = "preflight"
	comparePhasePilot     = "pilot"
	comparePhaseRun       = "run"
	comparePhaseGrade     = "grade"
	comparePhaseJudge     = "judge"
	comparePhaseReport    = "report"
)

// The kapi verbs the harness itself runs as a person, while it sets up a
// project: reading the voice into the store, noting each held rename, and
// keeping the note as a rule. They are named here and nowhere else, so a
// change to the CLI is one edit; the transcript reader is separate.
var (
	compareVerbImport = []string{"store", "import"}
	compareVerbRecord = []string{"context", "note"}
	compareVerbKeep   = []string{"context", "review", "--keep"}
)

// CompareManifest freezes a study's inputs.
type CompareManifest struct {
	Schema int    `json:"schema"`
	Study  string `json:"study"`
	// Hosts are the agent hosts under test.
	Hosts []PairedAgentSpec `json:"hosts"`
	// Arms are the conditions, a subset of compareArms.
	Arms []string `json:"arms"`
	// Tasks run in the full phase; PilotTasks in the pilot.
	Tasks      []string `json:"tasks"`
	PilotTasks []string `json:"pilot_tasks"`
	// Repeats is how often each task runs per arm and host in the full phase,
	// and PilotRepeats in the pilot.
	Repeats      int `json:"repeats"`
	PilotRepeats int `json:"pilot_repeats"`
	// Judges score voice fit. Two from different model families.
	Judges                []PairedAgentSpec `json:"judges"`
	AttemptTimeoutSeconds int               `json:"attempt_timeout_seconds"`
	MaxTurns              int               `json:"max_turns"`
	Billing               string            `json:"billing"`
	// Seed fixes the order sessions run in, which interleaves the arms so a
	// slow hour or a rate limit falls on all of them alike.
	Seed uint64 `json:"seed"`
}

// CompareCheck is one deterministic grader. A project's house checks have no
// kind: each forbids a pattern in the prose an attempt added to any content
// file, outside the files it exempts. A task's own checks name a kind and a
// file.
type CompareCheck struct {
	ID       string   `json:"id"`
	Category string   `json:"category"`
	Kind     string   `json:"kind,omitempty"`
	File     string   `json:"file,omitempty"`
	Pattern  string   `json:"pattern,omitempty"`
	Exempt   []string `json:"exempt,omitempty"`
	Min      int      `json:"min,omitempty"`
	// Raw matches the file as written, code included, rather than its prose.
	Raw  bool   `json:"raw,omitempty"`
	Note string `json:"note,omitempty"`
}

// Check kinds.
const (
	compareCheckForbidAdded   = ""                    // house check: pattern absent from added prose
	compareCheckForbidFinal   = "forbid_final"        // pattern absent from the file's final prose
	compareCheckRequireFinal  = "require_final"       // pattern present in the file's final prose
	compareCheckJSONValid     = "json_valid"          // the file parses as JSON
	compareCheckJSONKeysKept  = "json_keys_preserved" // every baseline key is still there
	compareCheckJSONNewKeys   = "json_new_keys"       // at least Min new keys match Pattern
	compareCategoryTask       = "task"
	compareCategoryRenameScop = "rename-scope"
)

// compareCategories are the rule categories the report breaks violations into,
// in the order it prints them. "task" checks say whether the work was done and
// are reported apart from rule violations.
var compareCategories = []string{"rename", compareCategoryRenameScop, "competitor", "claim", "term", "voice-rule"}

// CompareTask is one writing task.
type CompareTask struct {
	ID      string         `json:"id"`
	Project string         `json:"project"`
	Kind    string         `json:"kind"`
	Prompt  string         `json:"prompt"`
	Outputs []string       `json:"outputs"`
	Checks  []CompareCheck `json:"checks,omitempty"`
}

// CompareHeld is one rule a person holds through kapi's context verbs, at the
// point of the file it was seen in: a rename in force there and, through the
// recipe's profiles, nowhere the profile does not reach.
type CompareHeld struct {
	Summary   string   `json:"summary"`
	Term      string   `json:"term"`
	InsteadOf []string `json:"instead_of"`
	SeenIn    string   `json:"seen_in"`
	Quote     string   `json:"quote"`
	// WidenTo is the axis the person drops as they keep the rule, so it holds
	// across every channel of the profile it was seen in: a note holds only at
	// the point it was seen until a person widens it.
	WidenTo string `json:"widen_to,omitempty"`
}

// CompareProject is one sample project: its content, its recipe, the context
// kapi holds, and the same rules as a style guide.
type CompareProject struct {
	Name   string            `json:"name"`
	Files  map[string][]byte `json:"-"`
	Recipe []byte            `json:"-"`
	Voice  []byte            `json:"-"`
	Held   []CompareHeld     `json:"held"`
	Rules  []byte            `json:"-"`
	Checks []CompareCheck    `json:"checks"`
}

// CompareAttempt is one live session: one host doing one task in one arm, in
// a cell of its own.
type CompareAttempt struct {
	ID      string          `json:"id"`
	Phase   string          `json:"phase"`
	Task    string          `json:"task"`
	Project string          `json:"project"`
	Arm     string          `json:"arm"`
	Host    PairedAgentSpec `json:"host"`
	Repeat  int             `json:"repeat"`
}

//go:embed all:testdata/compare
var compareFixtures embed.FS

const compareRoot = "testdata/compare"

func compareTasks() ([]CompareTask, error) {
	data, err := compareFixtures.ReadFile(compareRoot + "/tasks.json")
	if err != nil {
		return nil, err
	}
	var tasks []CompareTask
	if err := compareStrictJSON(data, &tasks); err != nil {
		return nil, fmt.Errorf("tasks.json: %w", err)
	}
	return tasks, nil
}

func findCompareTask(id string) (CompareTask, error) {
	tasks, err := compareTasks()
	if err != nil {
		return CompareTask{}, err
	}
	for _, task := range tasks {
		if task.ID == id {
			return task, nil
		}
	}
	return CompareTask{}, fmt.Errorf("unknown comparison task %q", id)
}

func loadCompareProject(name string) (CompareProject, error) {
	base := compareRoot + "/projects/" + name
	p := CompareProject{Name: name, Files: map[string][]byte{}}
	var err error
	if p.Recipe, err = compareFixtures.ReadFile(base + "/kapi.yaml"); err != nil {
		return p, err
	}
	if p.Voice, err = compareFixtures.ReadFile(base + "/context/voice.yaml"); err != nil {
		return p, err
	}
	if p.Rules, err = compareFixtures.ReadFile(base + "/context/rules.md"); err != nil {
		return p, err
	}
	held, err := compareFixtures.ReadFile(base + "/context/held.json")
	if err != nil {
		return p, err
	}
	if err := compareStrictJSON(held, &p.Held); err != nil {
		return p, fmt.Errorf("%s held.json: %w", name, err)
	}
	checks, err := compareFixtures.ReadFile(base + "/checks.json")
	if err != nil {
		return p, err
	}
	if err := compareStrictJSON(checks, &p.Checks); err != nil {
		return p, fmt.Errorf("%s checks.json: %w", name, err)
	}
	content := base + "/content"
	err = fs.WalkDir(compareFixtures, content, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		data, err := compareFixtures.ReadFile(name)
		if err != nil {
			return err
		}
		p.Files[strings.TrimPrefix(name, content+"/")] = data
		return nil
	})
	return p, err
}

func compareStrictJSON(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("must contain one JSON value")
	}
	return nil
}

func readCompareManifest(path string) (CompareManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CompareManifest{}, err
	}
	var m CompareManifest
	if err := compareStrictJSON(data, &m); err != nil {
		return m, fmt.Errorf("decode comparison manifest: %w", err)
	}
	return m, validateCompareManifest(m)
}

func validateCompareManifest(m CompareManifest) error {
	if m.Schema != compareSchema {
		return fmt.Errorf("unsupported comparison schema %d", m.Schema)
	}
	if !pairedIDPattern.MatchString(m.Study) {
		return errors.New("study must be a lowercase path-safe identifier")
	}
	if m.Billing != "subscription-only" {
		return errors.New("billing must be subscription-only; API spending is disabled")
	}
	if len(m.Hosts) == 0 || len(m.Arms) == 0 {
		return errors.New("name at least one host and one arm")
	}
	for _, host := range append(slices.Clone(m.Hosts), m.Judges...) {
		if host.Host != "claude" && host.Host != "codex" {
			return fmt.Errorf("unsupported host %q", host.Host)
		}
		if host.Model == "" || host.Effort == "" {
			return errors.New("each host and judge needs an explicit model and effort")
		}
	}
	for _, arm := range m.Arms {
		if !slices.Contains(compareArms, arm) {
			return fmt.Errorf("unknown arm %q", arm)
		}
	}
	for _, list := range [][]string{m.Tasks, m.PilotTasks} {
		for _, id := range list {
			if _, err := findCompareTask(id); err != nil {
				return err
			}
		}
	}
	if len(m.Tasks) == 0 || len(m.PilotTasks) == 0 {
		return errors.New("name the tasks and the pilot tasks")
	}
	if m.Repeats < 1 || m.PilotRepeats < 1 {
		return errors.New("repeats must be positive")
	}
	if m.AttemptTimeoutSeconds < 60 || m.AttemptTimeoutSeconds > 3600 {
		return errors.New("attempt_timeout_seconds must be between 60 and 3600")
	}
	if m.MaxTurns < 1 {
		return errors.New("max_turns must be positive")
	}
	return nil
}

func (m CompareManifest) attemptTimeout() time.Duration {
	return time.Duration(m.AttemptTimeoutSeconds) * time.Second
}

// compareSchedule lists a phase's attempts, shuffled by the manifest's seed so
// the arms and hosts interleave in time.
func compareSchedule(m CompareManifest, phase string) ([]CompareAttempt, error) {
	tasks, repeats := m.Tasks, m.Repeats
	switch phase {
	case comparePhasePilot:
		tasks, repeats = m.PilotTasks, m.PilotRepeats
	case comparePhaseRun:
	default:
		return nil, fmt.Errorf("phase %q has no live schedule", phase)
	}
	attempts := []CompareAttempt{}
	for _, id := range tasks {
		task, err := findCompareTask(id)
		if err != nil {
			return nil, err
		}
		for repeat := 1; repeat <= repeats; repeat++ {
			for _, arm := range m.Arms {
				for _, host := range m.Hosts {
					attempts = append(attempts, CompareAttempt{
						ID:    fmt.Sprintf("%s-%s-%s-r%d", task.ID, arm, host.Host, repeat),
						Phase: phase, Task: task.ID, Project: task.Project, Arm: arm, Host: host, Repeat: repeat,
					})
				}
			}
		}
	}
	shuffle := rand.New(rand.NewPCG(m.Seed, 0x6b617069))
	shuffle.Shuffle(len(attempts), func(i, j int) { attempts[i], attempts[j] = attempts[j], attempts[i] })
	return attempts, nil
}

// compareKapiMention finds a prompt that names kapi or one of its surfaces.
// Prompts are what a person would type, so none may point at the mechanism
// under test.
var compareKapiMention = regexp.MustCompile(`(?i)\bkapi\b|\bcontext_|\bmcp\b|\bstyle guide\b|CLAUDE\.md|AGENTS\.md|\bvoice\b`)

// compareContentPath reports whether a repository path is the project's
// content rather than an arm's wiring. Only content is graded and compared.
func compareContentPath(name string) bool {
	name = path.Clean(name)
	switch name {
	case "kapi.yaml", ".mcp.json":
		return false
	}
	switch path.Base(name) {
	case "CLAUDE.md", "AGENTS.md":
		return false
	}
	first, _, _ := strings.Cut(name, "/")
	switch first {
	case ".git", ".claude", ".agents", ".codex", ".cursor", ".vscode", ".kapi":
		return false
	}
	return true
}

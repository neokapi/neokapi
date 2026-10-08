package main

import (
	"maps"
	"math"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestCompareManifestAndFixtures(t *testing.T) {
	m, err := readCompareManifest("testdata/compare-study.json")
	require.NoError(t, err)
	tasks, err := compareTasks()
	require.NoError(t, err)
	assert.Len(t, m.Tasks, len(tasks), "the full run names every task")
	for _, task := range tasks {
		t.Run(task.ID, func(t *testing.T) {
			assert.False(t, compareKapiMention.MatchString(task.Prompt), "a prompt names no mechanism under test")
			project, err := loadCompareProject(task.Project)
			require.NoError(t, err)
			require.NotEmpty(t, task.Outputs)
			for _, check := range task.Checks {
				_, err := regexp.Compile(check.Pattern)
				require.NoError(t, err, check.ID)
				_, known := project.Files[check.File]
				assert.True(t, known || slices.Contains(task.Outputs, check.File), "%s names a file the project or the task holds", check.ID)
			}
		})
	}
}

// Every house check must see the probe sentence, or a rule the graders cannot
// see would count as followed.
func TestCompareProbeBreaksEveryHouseCheck(t *testing.T) {
	for _, name := range []string{"teamboard", "ledgerly", "harbor"} {
		project, err := loadCompareProject(name)
		require.NoError(t, err)
		probe := compareProbeText(project)
		require.NotEmpty(t, probe)
		for _, check := range project.Checks {
			assert.Regexp(t, check.Pattern, probe, "%s/%s", name, check.ID)
		}
	}
}

// The two arms hold the same rules. Every word a rule avoids, in kapi's store
// or the held renames, is named in the writing guide the rulesfile arm reads,
// and every banned pattern's example is too.
func TestCompareArmsHoldTheSameRules(t *testing.T) {
	for _, name := range []string{"teamboard", "ledgerly", "harbor"} {
		project, err := loadCompareProject(name)
		require.NoError(t, err)
		guide := strings.ToLower(string(project.Rules))
		var voice struct {
			Terms []struct {
				Term        string `yaml:"term"`
				Replacement string `yaml:"replacement"`
			} `yaml:"terms"`
		}
		require.NoError(t, yaml.Unmarshal(project.Voice, &voice))
		for _, term := range voice.Terms {
			assert.Contains(t, guide, strings.ToLower(term.Term), "%s: the guide names %q", name, term.Term)
			assert.Contains(t, guide, strings.ToLower(term.Replacement), "%s: the guide names %q", name, term.Replacement)
		}
		for _, held := range project.Held {
			assert.Contains(t, guide, strings.ToLower(held.Term), name)
			for _, avoided := range held.InsteadOf {
				assert.Contains(t, guide, strings.ToLower(avoided), name)
			}
		}
	}
}

func TestCompareRulesFilesAreNotContent(t *testing.T) {
	assert.False(t, compareContentPath("legal/AGENTS.md"))
	assert.False(t, compareContentPath("CLAUDE.md"))
	assert.True(t, compareContentPath("legal/service-terms.md"))
}

func TestCompareProseDropsCode(t *testing.T) {
	text := "Use `harbor whitelist` here.\n\n```toml\nwhitelist = true\n```\n\nSee [the list](whitelist.md).\n"
	assert.NotContains(t, compareProse("docs/x.md", text), "whitelist")
	values := compareProse("locales/en.json", `{"autopay.title": "Scheduled payments"}`)
	assert.Equal(t, "Scheduled payments", values)
}

func TestCompareAddedReadsOnlyNewText(t *testing.T) {
	before := []byte("# Title\n\nOld line about Workspace.\n")
	after := []byte("# Title\n\nOld line about Workspace.\n\nNew line about your Space.\n")
	raw, prose := compareAdded("help/x.md", before, after, false)
	assert.Equal(t, "New line about your Space.", raw)
	assert.Equal(t, "New line about your Space.", prose)
	jsonBefore := []byte(`{"a": "One", "b": "Two"}`)
	jsonAfter := []byte(`{"a": "One", "b": "Two changed", "c": "Three"}`)
	raw, prose = compareAdded("locales/en.json", jsonBefore, jsonAfter, false)
	assert.Equal(t, "\"b\": \"Two changed\"\n\"c\": \"Three\"", raw)
	assert.Equal(t, "Two changed\nThree", prose)
}

func gradeFor(t *testing.T, taskID string, edit func(files map[string][]byte)) CompareGrade {
	t.Helper()
	task, err := findCompareTask(taskID)
	require.NoError(t, err)
	project, err := loadCompareProject(task.Project)
	require.NoError(t, err)
	final := map[string][]byte{}
	maps.Copy(final, project.Files)
	edit(final)
	return gradeCompareAttempt(project, task, CompareAttempt{Task: taskID}, final)
}

func TestCompareGradeScopedRename(t *testing.T) {
	right := gradeFor(t, "teamboard-rename-update", func(files map[string][]byte) {
		files["help/invite-people.md"] = []byte(strings.NewReplacer("workspace", "Space", "Workspace", "Space").Replace(string(files["help/invite-people.md"])))
	})
	assert.Zero(t, right.Violations, right.Checks)
	assert.True(t, right.TaskDone)

	over := gradeFor(t, "teamboard-rename-update", func(files map[string][]byte) {
		for _, name := range []string{"help/invite-people.md", "api/workspaces.md"} {
			files[name] = []byte(strings.NewReplacer("workspaces", "spaces", "workspace", "Space", "Workspace", "Space", "Workspaces", "Spaces").Replace(string(files[name])))
		}
	})
	assert.Contains(t, over.Broken, compareCategoryRenameScop)

	missed := gradeFor(t, "teamboard-rename-update", func(files map[string][]byte) {
		files["help/invite-people.md"] = append(files["help/invite-people.md"], []byte("\nAsk an Admin if you cannot log in.\n")...)
	})
	assert.ElementsMatch(t, []string{"rename", "term"}, missed.Broken)
}

func TestCompareGradeHouseRulesOnANewFile(t *testing.T) {
	grade := gradeFor(t, "harbor-support-dockyard", func(files map[string][]byte) {
		files["support/replies/dockyard-question.md"] = []byte("Hi,\n\nLike Dockyard, Harbor Cloud gives you zero-downtime deploys. Simply run `harbor deploy`!\n")
	})
	assert.ElementsMatch(t, []string{"rename", "competitor", "claim", "voice-rule"}, grade.Broken)
	assert.True(t, grade.TaskDone)
	clean := gradeFor(t, "harbor-support-dockyard", func(files map[string][]byte) {
		files["support/replies/dockyard-question.md"] = []byte("Hi,\n\nHarbor Hosted moves traffic to a new release only after it passes its health check.\n")
	})
	assert.Zero(t, clean.Violations, clean.Checks)
	notDone := gradeFor(t, "harbor-support-dockyard", func(map[string][]byte) {})
	assert.False(t, notDone.TaskDone)
}

func TestCompareGradeStringKeys(t *testing.T) {
	renamedKeys := gradeFor(t, "ledgerly-rename-update", func(files map[string][]byte) {
		files["locales/en.json"] = []byte(strings.ReplaceAll(strings.ReplaceAll(string(files["locales/en.json"]), "Autopay", "Scheduled payments"), "autopay.", "scheduled_payments."))
		files["help/autopay.md"] = []byte(strings.ReplaceAll(string(files["help/autopay.md"]), "Autopay", "Scheduled payments"))
	})
	assert.Contains(t, renamedKeys.Broken, compareCategoryRenameScop)
	right := gradeFor(t, "ledgerly-rename-update", func(files map[string][]byte) {
		files["locales/en.json"] = []byte(strings.ReplaceAll(string(files["locales/en.json"]), "\"Autopay", "\"Scheduled payments"))
		files["locales/en.json"] = []byte(strings.ReplaceAll(string(files["locales/en.json"]), " Autopay", " scheduled payments"))
		files["help/autopay.md"] = []byte(strings.ReplaceAll(string(files["help/autopay.md"]), "Autopay", "Scheduled payments"))
	})
	assert.Zero(t, right.Violations, right.Checks)
}

func TestCompareKappaAndWilson(t *testing.T) {
	assert.InDelta(t, 1.0, compareKappa([]bool{true, false, true, false}, []bool{true, false, true, false}), 1e-9)
	assert.InDelta(t, -1.0, compareKappa([]bool{true, false}, []bool{false, true}), 1e-9)
	assert.True(t, math.IsNaN(compareKappa([]bool{true, true}, []bool{true, true})))
	lo, hi := wilson(5, 10)
	assert.InDelta(t, 0.237, lo, 0.001)
	assert.InDelta(t, 0.763, hi, 0.001)
}

func TestCompareBootstrapStaysWithinTasks(t *testing.T) {
	a := stratum{"t1": {0, 0, 0}, "t2": {1, 1, 1}}
	b := stratum{"t1": {1, 1, 1}, "t2": {2, 2, 2}}
	diff, lo, hi := bootstrapDiff(a, b, 1)
	assert.InDelta(t, -1.0, diff, 1e-9)
	assert.InDelta(t, -1.0, lo, 1e-9)
	assert.InDelta(t, -1.0, hi, 1e-9)
}

func TestCompareParseVerdict(t *testing.T) {
	answers, reason, err := compareParseVerdict("Here: {\"address\": true, \"lead\": true, \"plain\": false, \"faithful\": true, \"short\": false, \"reason\": \"blazing\"}")
	require.NoError(t, err)
	assert.False(t, answers["plain"])
	assert.Equal(t, "blazing", reason)
	_, _, err = compareParseVerdict("{\"address\": true}")
	assert.Error(t, err)
}

func TestCompareScheduleIsCompleteAndInterleaved(t *testing.T) {
	m, err := readCompareManifest("testdata/compare-study.json")
	require.NoError(t, err)
	pilot, err := compareSchedule(m, comparePhasePilot)
	require.NoError(t, err)
	assert.Len(t, pilot, len(m.PilotTasks)*m.PilotRepeats*len(m.Arms)*len(m.Hosts))
	seen := map[string]bool{}
	for _, attempt := range pilot {
		assert.False(t, seen[attempt.ID])
		seen[attempt.ID] = true
	}
	full, err := compareSchedule(m, comparePhaseRun)
	require.NoError(t, err)
	assert.Len(t, full, len(m.Tasks)*m.Repeats*len(m.Arms)*len(m.Hosts))
	assert.NotEqual(t, full[0].Arm+full[1].Arm+full[2].Arm, strings.Repeat(full[0].Arm, 3), "the order interleaves the arms")
}

func TestCompareCodexRunsInWorkspaceWrite(t *testing.T) {
	ep := EvalPrepared{Session: EvalSession{Host: PairedAgentSpec{Host: "codex", Model: "m", Effort: "medium"}},
		Paths: comparePaths("/cells/a", "teamboard"), Env: []string{"PATH=/cells/a/bin"}}
	config := compareCodexConfig(ep, "/tmp/kpe-x")
	assert.Contains(t, config, "sandbox_mode = \"workspace-write\"")
	assert.NotContains(t, config, "danger-full-access")
	assert.Contains(t, config, "writable_roots = [\"/cells/a\", \"/tmp/kpe-x\"]")
	assert.Contains(t, config, "network_access = false")
}

func TestCompareRefusesACellTheSandboxCannotRead(t *testing.T) {
	_, err := compareDenyRead("/private/tmp/claude-" + strconv.Itoa(os.Getuid()) + "/cells/a")
	require.Error(t, err)
	_, err = compareDenyRead("/private/tmp/kapi-compare-cells/a")
	assert.NoError(t, err)
}

func TestCompareJudgeSeesChangedFilesBefore(t *testing.T) {
	project, err := loadCompareProject("harbor")
	require.NoError(t, err)
	before := compareFilesBefore(project, "### docs/hosted/overview.md (changed)\n\nHarbor Hosted runs in three regions.\n\n### support/replies/new.md (new)\n\nHi")
	assert.Contains(t, before, "### docs/hosted/overview.md (before)")
	assert.Contains(t, before, "Harbor Cloud runs in three regions")
	assert.NotContains(t, before, "support/replies/new.md")
}

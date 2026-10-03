package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The WP5 families, one task each (docs/internals/edit-model.md, WP5).
var pairedWP5Families = []string{
	"bilingual-po", "gate-recovery", "insert-key", "new-edition", "plural-branch", "stale-recovery", "wording-link",
}

func TestPairedCorpusMaterialization(t *testing.T) {
	tasks := pairedTasks()
	require.Len(t, tasks, 11)
	families := []string{}
	seen := map[string]bool{}
	for _, task := range tasks {
		t.Run(task.ID, func(t *testing.T) {
			require.False(t, seen[task.ID], "task IDs must be unique")
			seen[task.ID] = true
			families = append(families, task.Family)
			dir := t.TempDir()
			require.NoError(t, materializePairedTask(dir, task))
			_, err := project.Load(filepath.Join(dir, "kapi.yaml"))
			require.NoError(t, err, "recipe must be usable by the shipped engine")
			voice, err := os.ReadFile(filepath.Join(dir, ".kapi", "voice.yaml"))
			require.NoError(t, err)
			_, err = profile.LoadProfileYAML(bytes.NewReader(voice))
			require.NoError(t, err)
			files, err := pairedTaskFiles(task)
			require.NoError(t, err)
			count := 0
			err = filepath.WalkDir(dir, func(name string, entry fs.DirEntry, walkErr error) error {
				require.NoError(t, walkErr)
				if entry.IsDir() {
					return nil
				}
				count++
				rel, err := filepath.Rel(dir, name)
				require.NoError(t, err)
				expected, exists := files[filepath.ToSlash(rel)]
				require.True(t, exists, "private scoring files must not leak into the workspace")
				actual, err := os.ReadFile(name)
				require.NoError(t, err)
				assert.Equal(t, expected, actual)
				return nil
			})
			require.NoError(t, err)
			assert.Len(t, files, count)
			assert.NotEmpty(t, task.spec.Criteria)
			assert.NotEmpty(t, task.spec.HumanReviewRubric)
			assert.NotEmpty(t, task.spec.Scope)
			for _, name := range slices.Concat(task.spec.Editable, task.spec.Creates) {
				assert.Contains(t, task.spec.Scope, pairedDirOf(name), "%s lies outside every scoped directory", name)
			}
			for _, name := range task.spec.Editable {
				assert.Contains(t, files, name, "an editable file must be a fixture file")
			}
			for _, criterion := range task.spec.Criteria {
				if criterion.Kind == "no_override" {
					continue
				}
				assert.True(t, slices.Contains(task.spec.Editable, criterion.Path) || slices.Contains(task.spec.Creates, criterion.Path),
					"criterion %q names a path the task neither edits nor creates", criterion.ID)
			}
			if task.spec.Interference != nil {
				assert.Contains(t, task.spec.Editable, task.spec.Interference.Path)
				assert.Contains(t, string(files[task.spec.Interference.Path]), task.spec.Interference.Find)
			}
		})
	}
	slices.Sort(families)
	assert.Equal(t, pairedWP5Families, slices.Compact(families), "the variants add tasks to the WP5 families, and no family")
	hash, err := pairedCorpusHash()
	require.NoError(t, err)
	assert.Len(t, hash, 64)
	again, err := pairedCorpusHash()
	require.NoError(t, err)
	assert.Equal(t, hash, again)
}

func TestPairedMaterializationRejectsUnsafeDestinations(t *testing.T) {
	task := pairedTasks()[0]
	t.Run("existing file", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "STYLE.md"), []byte("keep"), 0o600))
		require.Error(t, materializePairedTask(dir, task))
		body, err := os.ReadFile(filepath.Join(dir, "STYLE.md"))
		require.NoError(t, err)
		assert.Equal(t, "keep", string(body))
	})
	t.Run("linked root", func(t *testing.T) {
		link := filepath.Join(t.TempDir(), "workspace")
		require.NoError(t, os.Symlink(t.TempDir(), link))
		require.Error(t, materializePairedTask(link, task))
	})
	t.Run("linked parent", func(t *testing.T) {
		dir, outside := t.TempDir(), t.TempDir()
		require.NoError(t, os.Symlink(outside, filepath.Join(dir, ".kapi")))
		require.Error(t, materializePairedTask(dir, task))
		entries, err := os.ReadDir(outside)
		require.NoError(t, err)
		assert.Empty(t, entries)
	})
	t.Run("task traversal", func(t *testing.T) {
		task.ID = "../common"
		require.Error(t, materializePairedTask(t.TempDir(), task))
	})
}

// This tests the authored fixture rules, not the product checker or prose
// quality: the voice every task's project binds forbids unsupported
// assurances, and its terms forbid "portal" in favour of "overview page",
// which STYLE.md states for an agent that reads files.
func TestPairedFixtureGovernanceMatchesItsStyleGuide(t *testing.T) {
	body, err := pairedFixtures.ReadFile("testdata/paired/common/.kapi/voice.yaml")
	require.NoError(t, err)
	voice, err := profile.LoadProfileYAML(bytes.NewReader(body))
	require.NoError(t, err)
	var expression string
	for _, constraint := range voice.Constraints {
		if constraint.ID == "harbor-help/no-unsupported-assurance" {
			expression = constraint.Regex
		}
	}
	require.NotEmpty(t, expression)
	pattern, err := regexp.Compile(expression)
	require.NoError(t, err)
	for _, statement := range []string{
		"Harbor Help is guaranteed to solve your problem.",
		"Harbor Help guarantees a result.",
		"Harbor Help is always safe.",
		"Harbor Help is risk-free.",
	} {
		assert.True(t, pattern.MatchString(statement), statement)
	}
	assert.False(t, pattern.MatchString("Harbor Help offers scheduled video appointments."))
	// The rule against portal is the gate task's late context: the voice
	// file and the style guide gain it together, once the agent has read
	// the context without it.
	assert.NotContains(t, string(body), "portal")
	late := map[string]string{}
	for _, a := range pairedTaskByID(t, "recover-gate-refusal").spec.LateContext.Append {
		late[a.Path] = a.Text
	}
	withRule, err := profile.LoadProfileYAML(strings.NewReader(string(body) + late[".kapi/voice.yaml"]))
	require.NoError(t, err)
	rules := withRule.CarriedTerms().Rules
	require.Len(t, rules, 1)
	assert.Equal(t, "portal", rules[0].Term)
	assert.Equal(t, "overview page", rules[0].Replacement)
	assert.Contains(t, late["STYLE.md"], `write "overview page", never "portal"`)
}

// The gate task's forbidden word appears in no skill an arm installs and in
// no MCP tool description, so no arm is primed for or against it.
func TestPairedForbiddenTermPrimesNoArm(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skipf("not in a git checkout: %v", err)
	}
	forbidden := regexp.MustCompile(`(?i)\bportal`)
	sources := []string{}
	for _, dir := range []string{filepath.Join(root, "cli", "skills", "data", "kapi")} {
		require.NoError(t, filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err == nil && !entry.IsDir() {
				sources = append(sources, path)
			}
			return err
		}))
	}
	descriptions, err := filepath.Glob(filepath.Join(root, "host", "mcp*.go"))
	require.NoError(t, err)
	sources = append(sources, descriptions...)
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		body, err := os.ReadFile(source)
		require.NoError(t, err)
		assert.False(t, forbidden.Match(body), "%s names the gate task's forbidden word", source)
	}
	require.NoError(t, fs.WalkDir(pairedFixtures, "testdata/paired/skills", func(name string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			body, readErr := pairedFixtures.ReadFile(name)
			require.NoError(t, readErr)
			assert.False(t, forbidden.Match(body), "%s names the gate task's forbidden word", name)
		}
		return err
	}))
}

// The project-free skill carries the shipped skill's guidance with the names
// changed and what needs a project left out. Each example it gives is one the
// shipped skill gives, and it names nothing of the tasks, so a difference
// between the arms is the surface's, not the skill text's.
func TestPairedProjectFreeSkillMirrorsTheShippedSkill(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Skipf("not in a git checkout: %v", err)
	}
	normalize := func(line string) string {
		for _, name := range pairedToolboxNames {
			line = strings.ReplaceAll(line, pairedFilesAlias+" "+name, name)
		}
		line = strings.ReplaceAll(line, pairedFilesAlias+"'", "kapi's")
		line = strings.ReplaceAll(line, pairedFilesAlias, "kapi")
		return strings.Join(strings.Fields(line), " ")
	}
	// The one example line a catalog without a project changes: outside a
	// project a catalog holds one edition, and a second is refused.
	noProject := map[string]bool{`"editions": {"en": {"text": "Checkout"}}},`: true}
	for _, name := range []string{"edit.md", "toolbox.md"} {
		shipped, err := os.ReadFile(filepath.Join(root, "cli", "skills", "data", "kapi", "references", name))
		require.NoError(t, err)
		shippedLines := map[string]bool{}
		for _, line := range pairedFenceLines(string(shipped)) {
			shippedLines[normalize(line)] = true
		}
		mirror, err := pairedFixtures.ReadFile("testdata/paired/skills/" + pairedFilesAlias + "/references/" + name)
		require.NoError(t, err)
		for _, line := range pairedFenceLines(string(mirror)) {
			if noProject[normalize(line)] {
				continue
			}
			assert.True(t, shippedLines[normalize(line)], "%s: an example the shipped skill does not give: %s", name, line)
		}
	}
	// Every paragraph of the shipped references that is about a file, and not
	// about a project, MCP, a check, a term or a translation, is in the
	// project-free references too, so the skills cannot drift apart unseen.
	projectOnly := regexp.MustCompile(`(?i)project|\bMCP\b|apply_edits|read_blocks|describe_format|\bcheck|verify|voice|\bterms?\b|translat|\bgate|recipe|comment|decision|person|on-brand|create\.md`)
	// What the alias says its own way: its toolbox runs as its commands, not
	// as programs installed beside it.
	ownWay := regexp.MustCompile(`^They install with the kapi CLI`)
	for _, name := range []string{"edit.md", "toolbox.md"} {
		shipped, err := os.ReadFile(filepath.Join(root, "cli", "skills", "data", "kapi", "references", name))
		require.NoError(t, err)
		mirror, err := pairedFixtures.ReadFile("testdata/paired/skills/" + pairedFilesAlias + "/references/" + name)
		require.NoError(t, err)
		mirrored := map[string]bool{}
		for _, paragraph := range pairedProseParagraphs(string(mirror)) {
			mirrored[normalize(paragraph)] = true
		}
		for _, paragraph := range pairedProseParagraphs(string(shipped)) {
			if projectOnly.MatchString(paragraph) || ownWay.MatchString(paragraph) {
				continue
			}
			assert.True(t, mirrored[normalize(paragraph)], "%s: a paragraph about a file the project-free skill lacks: %s", name, paragraph)
		}
	}

	// Both skills name the edit topic, so an agent in either arm is one step
	// from the edit guidance.
	pointer := "Before you change content inside a file, read `references/edit.md`"
	shippedSkill, err := os.ReadFile(filepath.Join(root, "cli", "skills", "data", "kapi", "SKILL.md"))
	require.NoError(t, err)
	mirrorSkill, err := pairedFixtures.ReadFile("testdata/paired/skills/" + pairedFilesAlias + "/SKILL.md")
	require.NoError(t, err)
	for name, body := range map[string][]byte{"shipped": shippedSkill, "project-free": mirrorSkill} {
		assert.Contains(t, strings.Join(strings.Fields(string(body)), " "), pointer, "the %s skill names the edit topic", name)
	}

	taskHints := regexp.MustCompile(`app_en\.arb|inboxCount|unread message|messages\.po|--target-lang nb|welcome\.md|upgrade\.md|reports\.md|help\.html|exportData|importData|Harbor|overview page|file of its own`)
	require.NoError(t, fs.WalkDir(pairedFixtures, "testdata/paired/skills", func(name string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			body, readErr := pairedFixtures.ReadFile(name)
			require.NoError(t, readErr)
			assert.Empty(t, taskHints.FindAllString(string(body), -1), "%s names a task", name)
		}
		return err
	}))
}

// pairedProseParagraphs lists a Markdown page's paragraphs outside its code
// fences and headings: the runs of lines between blank lines, a list counting
// as one.
func pairedProseParagraphs(text string) []string {
	var out, current []string
	flush := func() {
		if len(current) > 0 {
			out = append(out, strings.Join(current, "\n"))
			current = nil
		}
	}
	inside := false
	for line := range strings.SplitSeq(text, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "```"):
			flush()
			inside = !inside
		case inside:
		case trimmed == "" || strings.HasPrefix(trimmed, "#") || trimmed == "---":
			flush()
		default:
			current = append(current, line)
		}
	}
	flush()
	return out
}

// pairedFenceLines lists the lines inside a Markdown page's code fences.
func pairedFenceLines(text string) []string {
	var lines []string
	inside := false
	for line := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inside = !inside
			continue
		}
		if inside && strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

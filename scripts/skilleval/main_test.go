package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The scenarios are authored, and the one thing that rots silently is a fixture
// path. A moved .docx does not fail a build; it makes buildWorkspace error at
// run time, in a goroutine, on a maintainer's machine, halfway through a metered
// sweep. So every path is resolved here, where it costs nothing.
//
// The scoring is worth testing too. `flaky` exists precisely so a two-in-three
// result is not rounded up, and a rounding bug in it would be invisible on the
// dashboard and flattering in exactly one direction.

func root(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	return p
}

func TestEveryFixtureExists(t *testing.T) {
	r := root(t)
	for _, sc := range scenarios {
		t.Run(sc.ID, func(t *testing.T) {
			for _, f := range sc.Fixture {
				if f.From == "" {
					assert.NotEmpty(t, f.Body, "fixture %s has neither a source nor a body", f.As)
					continue
				}
				assert.NotEmpty(t, f.As, "a fixture must say where it lands")
				info, err := os.Stat(filepath.Join(r, f.From))
				if assert.NoError(t, err, "fixture source %s does not exist", f.From) {
					assert.Positive(t, info.Size(), "fixture %s is empty, so it tests nothing", f.From)
				}
			}
		})
	}
}

// TestAPromptNamesItsFiles.
//
// The lesson EVALS.md records twice: a prompt about pitch.pptx in an empty
// directory tests nothing, and a scenario whose files are all plain Markdown
// will correctly NOT trigger, because native grep is the better tool there.
// Both look like skill bugs and neither is.
//
// So when a prompt names a file, that file must be in the workspace.
func TestAPromptNamesItsFiles(t *testing.T) {
	for _, sc := range scenarios {
		t.Run(sc.ID, func(t *testing.T) {
			placed := map[string]bool{}
			for _, f := range sc.Fixture {
				placed[f.As] = true
				placed[filepath.Base(f.As)] = true
				// A prompt may name the directory rather than the file.
				for dir := filepath.Dir(f.As); dir != "." && dir != "/"; dir = filepath.Dir(dir) {
					placed[dir] = true
					placed[dir+"/"] = true
				}
			}
			for _, tok := range strings.Fields(sc.Prompt) {
				tok = strings.Trim(tok, ".,?!'\"")
				if !looksLikePath(tok) {
					continue
				}
				assert.True(t, placed[tok] || placed[filepath.Base(tok)],
					"the prompt names %q and the fixture does not provide it, so this scenario tests nothing it claims to", tok)
			}
		})
	}
}

func looksLikePath(tok string) bool {
	if strings.HasSuffix(tok, "/") {
		return true
	}
	ext := filepath.Ext(tok)
	switch ext {
	case ".pptx", ".docx", ".xlsx", ".json", ".md", ".html", ".xml", ".yaml", ".yml", ".go", ".ts", ".tsx", ".jsx", ".csv", ".log":
		return true
	}
	return false
}

// TestNegativesCarryNoContentWork: a negative that ships a .docx is not a
// negative. The whole point is that nothing in the workspace is content the
// editor cannot open itself.
func TestNegativesCarryNoContentWork(t *testing.T) {
	opaque := map[string]bool{".docx": true, ".pptx": true, ".xlsx": true, ".pdf": true}
	for _, sc := range scenarios {
		if sc.Kind != negative {
			continue
		}
		t.Run(sc.ID, func(t *testing.T) {
			for _, f := range sc.Fixture {
				assert.False(t, opaque[filepath.Ext(f.As)],
					"%s is a format only kapi can open, so firing here would be correct and the scenario is mis-specified", f.As)
			}
		})
	}
}

func TestScenarioIDsAreUniqueAndKinded(t *testing.T) {
	seen := map[string]bool{}
	pos, neg := 0, 0
	for _, sc := range scenarios {
		assert.False(t, seen[sc.ID], "duplicate scenario id %q", sc.ID)
		seen[sc.ID] = true
		assert.NotEmpty(t, sc.Why, "a scenario that cannot say why it exists cannot be scored")
		assert.Positive(t, sc.Turns, "a scenario needs a turn cap or a positive runs away into metered work")
		switch sc.Kind {
		case positive:
			pos++
		case negative:
			neg++
		default:
			t.Errorf("scenario %q has kind %q", sc.ID, sc.Kind)
		}
	}
	assert.Positive(t, pos)
	assert.Positive(t, neg, "without negatives the suite cannot see a description that fires on everything")
}

// TestFlakyIsNotRoundedUp is the scoring test that matters. Two fires in three
// on a positive is a real and different problem from three in three, and a
// verdict that called it `pass` would hide the class of regression this suite
// exists to catch.
func TestFlakyIsNotRoundedUp(t *testing.T) {
	cases := []struct {
		name  string
		kind  string
		fired []bool
		want  string
	}{
		{"positive always fires", positive, []bool{true, true, true}, "pass"},
		{"positive mostly fires", positive, []bool{true, true, false}, "flaky"},
		{"positive rarely fires", positive, []bool{true, false, false}, "flaky"},
		{"positive never fires", positive, []bool{false, false, false}, "fail"},
		{"negative stays quiet", negative, []bool{false, false, false}, "pass"},
		{"negative sometimes fires", negative, []bool{false, true, false}, "flaky"},
		{"negative always fires", negative, []bool{true, true, true}, "fail"},
		{"nothing ran", positive, nil, "not run"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := Result{Scenario: Scenario{Kind: c.kind}}
			for _, f := range c.fired {
				r.Runs = append(r.Runs, Run{Triggered: f})
			}
			r.score(modeTrigger)
			assert.Equal(t, c.want, r.Verdict)
		})
	}
}

// TestMentionsKapi guards the activation detector. Scoring only the Skill tool
// would miss an agent that read SKILL.md once and then worked from it, so a
// toolbox binary counts too — and a word that merely contains "kapi" must not.
func TestMentionsKapi(t *testing.T) {
	yes := []string{
		"kapi version",
		"kcat pitch.pptx",
		"./bin/kapi check --ship",
		"/opt/homebrew/bin/kgrep -r utilize docs/",
		"cd docs && ksed 's/utilize/use/g' *.docx",
	}
	no := []string{
		"grep -r utilize docs/",
		"echo 'this mentions kapifornia'",
		"python3 kapi_helper_name_only.py",
		"ls -la",
	}
	for _, c := range yes {
		assert.True(t, mentionsKapi(c), "should count as activation: %s", c)
	}
	for _, c := range no {
		assert.False(t, mentionsKapi(c), "should not count as activation: %s", c)
	}
}

// TestDiffWorkspaceSeesEveryKindOfChange: the dashboard renders what this
// returns, so a missed change is a diff a reader never sees.
func TestDiffWorkspaceSeesEveryKindOfChange(t *testing.T) {
	before := map[string][]byte{
		"kept.md":    []byte("same\n"),
		"edited.md":  []byte("old\n"),
		"removed.md": []byte("gone\n"),
	}
	after := map[string][]byte{
		"kept.md":   []byte("same\n"),
		"edited.md": []byte("new\n"),
		"added.md":  []byte("fresh\n"),
		"binary.db": {0x00, 0x01, 0x02},
	}

	got := map[string]FileChange{}
	for _, c := range diffWorkspace(before, after) {
		got[c.Path] = c
	}

	assert.NotContains(t, got, "kept.md", "an unchanged file is not a change")
	assert.Equal(t, "edited", got["edited.md"].Kind)
	assert.Equal(t, "old\n", got["edited.md"].Before)
	assert.Equal(t, "new\n", got["edited.md"].After)
	assert.Equal(t, "added", got["added.md"].Kind)
	assert.Empty(t, got["added.md"].Before, "an added file has no before")
	assert.Equal(t, "removed", got["removed.md"].Kind)
	assert.True(t, got["binary.db"].Binary, "a binary diff is meaningless as text")
	assert.Empty(t, got["binary.db"].After)
}

// TestIsolationEnvHonoursTheContract: an in-repo kapi that has not opted out of
// project discovery walks up and binds to the dogfood recipe at the repo root,
// then acts on it. The failure is silent, which is why it is asserted here.
func TestIsolationEnvHonoursTheContract(t *testing.T) {
	env := isolationEnv(t.TempDir())
	joined := strings.Join(env, "\n")
	for _, want := range []string{
		"KAPI_NO_PROJECT=1",
		"KAPI_CONFIG_DIR=",
		"XDG_DATA_HOME=",
		"XDG_CACHE_HOME=",
		"KAPI_PLUGINS_DIR_ONLY=1",
	} {
		assert.Contains(t, joined, want)
	}
}

// TestTheSkillIsWhereWeCopyItFrom: buildWorkspace copies the shipped skill into
// each workspace. If that directory moves, every scenario silently measures an
// agent with no skill installed, and every positive fails for the wrong reason.
func TestTheSkillIsWhereWeCopyItFrom(t *testing.T) {
	r := root(t)
	skill := filepath.Join(r, "cli", "skills", "data", "kapi", "SKILL.md")
	_, err := os.Stat(skill)
	require.NoError(t, err, "the shipped skill is not at %s", skill)

	body, err := os.ReadFile(skill)
	require.NoError(t, err)
	assert.Contains(t, string(body), "description:",
		"triggering is driven by the description, and it is the only field loaded at agent startup")
}

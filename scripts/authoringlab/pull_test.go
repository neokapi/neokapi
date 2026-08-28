package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// The pulled arm's whole claim is that the agent can fetch the same guide the
// pushed arm is handed. If it fetches something else, or nothing, the page shows
// a comparison between two governances while saying it compares two deliveries.

// TestResolvedProfileSurvivesYAML.
//
// The workspace binds THIS point's profile, resolved, so `kapi voice guide` with
// no flags prints what the pushed arm gets. That only holds if the resolved
// profile survives being written out and read back — a field without a yaml tag
// would be dropped on the way to disk, and the arm would quietly fetch a
// weaker guide than the one it is being compared against.
func TestResolvedProfileSurvivesYAML(t *testing.T) {
	base, err := loadProfile()
	require.NoError(t, err)

	for _, p := range points {
		t.Run(p.Audience, func(t *testing.T) {
			resolved := coreprofile.ResolveProfile(base, "", "", p.Persona)
			require.NotNil(t, resolved)
			want := coreprofile.RenderVoiceGuide(resolved)
			require.NotEmpty(t, want)

			body, err := yaml.Marshal(resolved)
			require.NoError(t, err)
			var back coreprofile.VoiceProfile
			require.NoError(t, yaml.Unmarshal(body, &back))

			assert.Equal(t, want, coreprofile.RenderVoiceGuide(&back),
				"the guide the pulled arm would fetch differs from the one the pushed arm is given")
		})
	}
}

// TestPulledWorkspaceServesTheGuide is the measurement rather than the schema:
// it builds the workspace and runs the command the skill tells an assistant to
// run. Skipped without the archive or the binary, because both are built by
// targets a plain `go test` does not run.
func TestPulledWorkspaceServesTheGuide(t *testing.T) {
	root := testRepoRoot(t)
	if _, err := pristineTar(root); err != nil {
		t.Skip("no subject archive: ./scripts/fetch-lab-repo.sh")
	}
	kapiBin, err := findKapi(root)
	if err != nil {
		t.Skip("no kapi binary: make build")
	}

	base, err := loadProfile()
	require.NoError(t, err)
	p := points[0]
	guide, err := guideFor(base, p)
	require.NoError(t, err)

	require.NoError(t, checkPull(context.Background(), root, kapiBin,
		coreprofile.ResolveProfile(base, "", "", p.Persona), guide))
}

// TestPulledWorkspaceCarriesTheSkillAndNothingElseDoes.
//
// The arms differ in the workspace, so what each workspace holds IS the
// independent variable. A skill or a recipe leaking into the bare tree would
// make the comparison meaningless while every number still looked fine.
func TestPulledWorkspaceCarriesTheSkillAndNothingElseDoes(t *testing.T) {
	root := testRepoRoot(t)
	if _, err := pristineTar(root); err != nil {
		t.Skip("no subject archive: ./scripts/fetch-lab-repo.sh")
	}
	base, err := loadProfile()
	require.NoError(t, err)
	profile := coreprofile.ResolveProfile(base, "", "", points[0].Persona)

	for _, tc := range []struct {
		name string
		arm  armSetup
		want bool
	}{
		{"bare", armSetup{}, false},
		{"pulled", armSetup{pull: true, profile: profile}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			tree, err := prepareWorkspace(root, home, tc.arm)
			require.NoError(t, err)

			// The source tree itself, in both.
			assert.FileExists(t, filepath.Join(tree, "README.md"))

			for _, path := range []string{
				filepath.Join(tree, "kapi.yaml"),
				filepath.Join(tree, ".kapi", "voice.yaml"),
				filepath.Join(tree, ".claude", "skills", "kapi", "SKILL.md"),
			} {
				_, err := os.Stat(path)
				if tc.want {
					assert.NoError(t, err, "the pulled arm needs %s", filepath.Base(path))
				} else {
					assert.ErrorIs(t, err, os.ErrNotExist,
						"%s must not be in a tree the arm was not given it in", filepath.Base(path))
				}
			}
		})
	}
}

// TestEachRunGetsItsOwnTree. The arms shared one directory once, and an agent
// with a shell left 402MB of build output in it that every later run read.
func TestEachRunGetsItsOwnTree(t *testing.T) {
	root := testRepoRoot(t)
	if _, err := pristineTar(root); err != nil {
		t.Skip("no subject archive: ./scripts/fetch-lab-repo.sh")
	}
	first, err := prepareWorkspace(root, t.TempDir(), armSetup{})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(first, "LEFTOVER"), []byte("x"), 0o644))

	second, err := prepareWorkspace(root, t.TempDir(), armSetup{})
	require.NoError(t, err)
	assert.NotEqual(t, first, second)
	assert.NoFileExists(t, filepath.Join(second, "LEFTOVER"),
		"what one run writes must not be in the tree the next one reads")
}

func TestKapiCalls(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command string
		want    []string
	}{
		{"the command the skill recommends", "kapi voice guide", []string{"kapi voice guide"}},
		{"after a cd", "cd crates && kapi context README.md", []string{"kapi context README.md"}},
		{"piped", "kapi voice guide | head -40", []string{"kapi voice guide"}},
		{"two of them", "kapi voice profiles; kapi voice guide", []string{"kapi voice profiles", "kapi voice guide"}},
		{"an absolute path to it", "/usr/local/bin/kapi voice guide", []string{"/usr/local/bin/kapi voice guide"}},
		// `&` both separates segments and sits inside `2>&1`, so the split leaves
		// a redirect fragment behind. The command is shown to a reader, and
		// `kapi voice guide 2>` is not a command anyone ran.
		{"redirecting stderr", "kapi voice guide 2>&1", []string{"kapi voice guide"}},
		{"redirecting and piping", "kapi context README.md 2>&1 | head", []string{"kapi context README.md"}},
		// The word appearing in a command is not the command being run, and an
		// arm credited with asking because it grepped for the word would be the
		// measurement reporting its own bug.
		{"grepping for the word", "rg kapi .claude", nil},
		{"reading the skill instead", "cat .claude/skills/kapi/SKILL.md", nil},
		{"nothing", "ls crates", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, kapiCalls(tc.command))
		})
	}
}

func TestWithKapiOnPath(t *testing.T) {
	env := withKapiOnPath([]string{"HOME=/h", "PATH=/usr/bin"}, "/repo/bin/kapi")
	assert.Contains(t, env, "PATH=/repo/bin:/usr/bin")
	assert.Len(t, env, 2, "the PATH is replaced, not appended: a child reads the first one")

	assert.Equal(t, []string{"PATH=/usr/bin"}, withKapiOnPath([]string{"PATH=/usr/bin"}, ""),
		"an arm with no binary is left alone")
}

// TestPullEnvNamesTheProjectInsteadOfDisablingDiscovery.
//
// The isolation contract says an in-repo agent must not bind to the dogfood
// recipe, and KAPI_NO_PROJECT is how every other harness here meets it. This arm
// cannot use it — discovery is the thing being measured — so it meets the same
// contract by naming the project, which is consulted before the upward walk.
func TestPullEnvNamesTheProjectInsteadOfDisablingDiscovery(t *testing.T) {
	env := pullEnv("/tmp/home", "/tmp/home/workspace/ripgrep")

	assert.Contains(t, env, "KAPI_PROJECT=/tmp/home/workspace/ripgrep/kapi.yaml")
	for _, kv := range env {
		assert.False(t, strings.HasPrefix(kv, "KAPI_NO_PROJECT="),
			"KAPI_NO_PROJECT would opt out of the discovery this arm exists to measure")
	}
	// Everything else the contract asks for is still there.
	for _, name := range []string{"KAPI_CONFIG_DIR", "XDG_DATA_HOME", "XDG_CACHE_HOME", "KAPI_PLUGINS_DIR_ONLY"} {
		assert.True(t, hasEnv(env, name), "%s is still part of the contract", name)
	}
}

func hasEnv(env []string, name string) bool {
	for _, kv := range env {
		if strings.HasPrefix(kv, name+"=") {
			return true
		}
	}
	return false
}

func testRepoRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

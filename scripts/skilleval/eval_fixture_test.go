package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateEvalFixtureAgreesWithItsKey(t *testing.T) {
	fixture, err := generateEvalFixture()
	require.NoError(t, err)
	assert.Empty(t, validateEvalFixture(fixture))
	pages := 0
	for _, name := range fixture.evalFileNames() {
		if strings.HasPrefix(name, "docs/") {
			pages++
		}
	}
	assert.Contains(t, fixture.Files, "README.md")
	assert.GreaterOrEqual(t, pages, 5)
	assert.LessOrEqual(t, pages, 8)
}

func TestGenerateEvalFixtureIsDeterministic(t *testing.T) {
	first, err := generateEvalFixture()
	require.NoError(t, err)
	second, err := generateEvalFixture()
	require.NoError(t, err)
	assert.Equal(t, first.Files, second.Files)
	firstHash, err := evalFixtureHash(first)
	require.NoError(t, err)
	secondHash, err := evalFixtureHash(second)
	require.NoError(t, err)
	assert.Equal(t, firstHash, secondHash)
}

func TestEvalKeyPlantsElevenAndOneDecoy(t *testing.T) {
	key := evalKey()
	assert.Len(t, key.planted(), 11)
	assert.Len(t, key.Conventions, 12, "twelve entries: eleven conventions and the decoy")
	recalled := 0
	for _, c := range key.planted() {
		assert.NotEmpty(t, c.Rules, c.ID)
		if c.recalled() {
			recalled++
		}
	}
	assert.Equal(t, 7, recalled, "four names and three spellings")
	decoy := key.decoy()
	assert.Equal(t, evalCategoryDecoy, decoy.Category)
	assert.Empty(t, decoy.Rules, "the decoy must never become a rule")
}

// The validation is what keeps the fixture from contradicting itself, so each
// kind of contradiction is shown to be caught.
func TestValidateEvalFixtureCatchesAContradiction(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(f *EvalFixture)
		want   string
	}{
		{"an avoided word appears", func(f *EvalFixture) {
			f.Files["docs/plans.md"] = append(f.Files["docs/plans.md"], []byte("\nEvery employee is welcome.\n")...)
		}, `"employee" appears`},
		{"a name is spelt the other way", func(f *EvalFixture) {
			f.Files["docs/plans.md"] = append(f.Files["docs/plans.md"], []byte("\nSee Full House.\n")...)
		}, `"Full House" appears`},
		{"an exclamation mark", func(f *EvalFixture) {
			f.Files["README.md"] = append(f.Files["README.md"], []byte("\nWelcome!\n")...)
		}, `"!" appears`},
		{"the decoy loses its balance", func(f *EvalFixture) {
			f.Files["docs/timesheets.md"] = append(f.Files["docs/timesheets.md"],
				[]byte("\ntimesheet timesheet timesheet timesheet timesheet timesheet\n")...)
		}, "not roughly even"},
		{"a prompt names a convention", func(f *EvalFixture) {
			f.Tasks[0].Prompt += " Mention the Fullhouse plan."
		}, `names "Fullhouse"`},
		{"a prompt asks for kapi", func(f *EvalFixture) {
			f.Tasks[1].Prompt += " Use kapi."
		}, `mentions "kapi"`},
		{"a page is written in the third person", func(f *EvalFixture) {
			f.Files["docs/new.md"] = []byte("# New\n\nA manager publishes the rota.\n")
		}, "does not address the reader as you"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture, err := generateEvalFixture()
			require.NoError(t, err)
			files := map[string][]byte{}
			for name, data := range fixture.Files {
				files[name] = append([]byte(nil), data...)
			}
			fixture.Files = files
			fixture.Tasks = append([]EvalTask(nil), fixture.Tasks...)
			tc.mutate(&fixture)
			assert.Contains(t, strings.Join(validateEvalFixture(fixture), "\n"), tc.want)
		})
	}
}

func TestEvalCount(t *testing.T) {
	tests := []struct {
		text, form string
		fold       bool
		want       int
	}{
		{"users and the user", "user", true, 1},
		{"the user's rota", "the user", true, 1},
		{"sign in, sign-in, signed in", "sign in", true, 1},
		{"Sign in here", "sign in", false, 0},
		{"Sign in here", "sign in", true, 1},
		{"Great! Really!", "!", false, 2},
		{"timesheets and timesheet", "timesheet", true, 1},
		{"e-mail, E-mail", "e-mail", true, 2},
		{"", "x", true, 0},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, evalCount(tc.text, tc.form, tc.fold), "%q in %q", tc.form, tc.text)
	}
}

func TestMaterializeEvalRepo(t *testing.T) {
	fixture, err := generateEvalFixture()
	require.NoError(t, err)
	dir := filepath.Join(t.TempDir(), evalRepoName)
	require.NoError(t, materializeEvalRepo(dir, fixture))
	for _, name := range fixture.evalFileNames() {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		require.NoError(t, err)
		assert.Equal(t, fixture.Files[name], data)
	}
	require.Error(t, materializeEvalRepo(dir, fixture), "an existing file is never overwritten")
}

func TestEvalRecipeMapping(t *testing.T) {
	mapped, err := evalRecipeMapping([]byte("version: v1\ncollections: []\n"))
	require.NoError(t, err)
	assert.Contains(t, string(mapped), `path: "docs/**/*.md"`)
	_, err = evalRecipeMapping([]byte("version: v1\n"))
	require.Error(t, err)
}

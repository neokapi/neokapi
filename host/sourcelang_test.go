package host

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The precedence itself, over all three of its sources at once, rather than
// through one command's output: an explicit --source-lang beats the recipe's
// `defaults.source_language`, which beats the built-in default. Asserted here
// because that order is what a run's every content-memory lookup is keyed to,
// and it was wrong on the ONLY surface nothing asserted — a flag whose registered
// default sat in the field before any command parsed anything (#2074).

func TestSourceLangPrecedence(t *testing.T) {
	cases := []struct {
		name string
		// flag is what the user typed after --source-lang; empty means the flag
		// was not typed at all.
		flag string
		// recipe is the project's `defaults.source_language`; empty is either a
		// project that declares none or no project at all.
		recipe model.LocaleID
		want   string
	}{
		{
			name:   "an explicit flag beats the recipe",
			flag:   "de-AT",
			recipe: "en-GB",
			want:   "de-AT",
		},
		{
			name:   "the recipe beats the built-in default",
			recipe: "en-GB",
			want:   "en-GB",
		},
		{
			name: "the built-in default stands when neither names one",
			want: DefaultSourceLang,
		},
		{
			name: "an explicit flag stands with no recipe to consult — the ad-hoc shape",
			flag: "ja",
			want: "ja",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &App{}
			cmd := NewEnvCommand(context.Background(), "run")
			a.AddSourceLangFlag(cmd.Flags())

			// Registration must leave the field empty. This is the whole defect:
			// pflag writes a flag's default into the bound field HERE, not at
			// parse, so a literal default is indistinguishable from a typed one
			// and no recipe can ever be reached.
			// assert, not require: when this does fail, the precedence assertions
			// below are the ones that say what it cost.
			assert.Empty(t, a.SourceLang,
				"registering --source-lang must not write a language into the field")
			assert.Empty(t, cmd.Flags().Lookup(sourceLangFlag).DefValue,
				"--source-lang must register with an empty default")

			if tc.flag != "" {
				require.NoError(t, cmd.Flags().Set(sourceLangFlag, tc.flag))
			}

			assert.Equal(t, tc.want, a.ResolveSourceLang(tc.recipe),
				"resolution settles on the highest-ranked source that named a language")
			assert.Equal(t, tc.want, a.SourceLocale(),
				"and every read of the run's source language agrees with it")
		})
	}
}

// A run that never resolves a project — `kapi translate notes.md --target-lang
// fr` outside any recipe, and every file-only verb in the toolbox — still reads
// its input as English. That is the built-in default doing its one job, and it
// is why the flag can register empty at all.
func TestSourceLocale_AdHocRunWithNoProject(t *testing.T) {
	a := &App{}
	cmd := NewEnvCommand(context.Background(), "translate")
	a.AddProcessingFlags(cmd)

	assert.Empty(t, a.SourceLang, "nothing named a source language")
	assert.Equal(t, DefaultSourceLang, a.SourceLocale(),
		"so the run reads its input in the built-in default, with no project to ask")

	require.NoError(t, cmd.Flags().Set(sourceLangFlag, "nb"))
	assert.Equal(t, "nb", a.SourceLocale(), "and honours the flag when one is typed")
}

// One App serves many projects in the desktop and the MCP server. A language
// adopted from one recipe must not be carried into the next run as though a
// user had typed it.
func TestResolveSourceLang_ScopedToOneRun(t *testing.T) {
	a := &App{}

	func() {
		defer a.scopeSourceLang()()
		assert.Equal(t, "en-GB", a.ResolveSourceLang("en-GB"))
	}()
	assert.Empty(t, a.SourceLang, "the first project's recipe does not outlive its run")

	func() {
		defer a.scopeSourceLang()()
		assert.Equal(t, "fr-CA", a.ResolveSourceLang("fr-CA"),
			"so the next project resolves its own")
	}()

	a.SourceLang = "ja"
	func() {
		defer a.scopeSourceLang()()
		assert.Equal(t, "ja", a.ResolveSourceLang("en-GB"))
	}()
	assert.Equal(t, "ja", a.SourceLang, "an explicitly named language survives the scope")
}

// The same three sources, same order, for the encoding a run reads in — the
// other processing flag whose registered default outranked its recipe key.
func TestEncodingPrecedence(t *testing.T) {
	cases := []struct {
		name   string
		flag   string
		recipe string
		want   string
	}{
		{name: "an explicit flag beats the recipe", flag: "Shift_JIS", recipe: "ISO-8859-1", want: "Shift_JIS"},
		{name: "the recipe beats the built-in default", recipe: "ISO-8859-1", want: "ISO-8859-1"},
		{name: "the built-in default stands when neither names one", want: DefaultEncoding},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &App{}
			cmd := NewEnvCommand(context.Background(), "run")
			a.AddEncodingFlag(cmd.Flags(), "e", "input file encoding")

			assert.Empty(t, a.Encoding, "registering --encoding must not write one into the field")
			assert.Empty(t, cmd.Flags().Lookup(encodingFlag).DefValue,
				"--encoding must register with an empty default")

			if tc.flag != "" {
				require.NoError(t, cmd.Flags().Set(encodingFlag, tc.flag))
			}

			assert.Equal(t, tc.want, a.ResolveEncoding(tc.recipe))
			assert.Equal(t, tc.want, a.InputEncoding())
		})
	}
}

// AddProcessingFlags is the registration every flow-running verb goes through,
// so it is the one that has to leave both fields empty.
func TestAddProcessingFlags_LeavesTheResolvableFieldsEmpty(t *testing.T) {
	a := &App{}
	cmd := NewEnvCommand(context.Background(), "up")
	a.AddProcessingFlags(cmd)

	assert.Empty(t, a.SourceLang)
	assert.Empty(t, a.Encoding)
	assert.Equal(t, DefaultSourceLang, a.SourceLocale())
	assert.Equal(t, DefaultEncoding, a.InputEncoding())
}

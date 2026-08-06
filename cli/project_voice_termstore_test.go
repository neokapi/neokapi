package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func registryToolID(name string) registry.ToolID { return registry.ToolID(name) }

// runPartThroughTool processes a single part through a tool and returns the
// (single) output part.
func runPartThroughTool(t *testing.T, tl tool.Tool, part *model.Part) *model.Part {
	t.Helper()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- part
	close(in)
	errc := make(chan error, 1)
	go func() {
		defer close(out)
		errc <- tl.Process(t.Context(), in, out)
	}()
	var result *model.Part
	for p := range out {
		result = p
	}
	require.NoError(t, <-errc)
	require.NotNil(t, result)
	return result
}

// writeProjectRecipe writes a minimal <name>.kapi recipe and returns the
// project root directory.
func writeProjectRecipe(t *testing.T, recipe string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".kapi"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi.yaml"), []byte(recipe), 0o644))
	return dir
}

// brandYAML is a minimal VoiceProfile with a forbidden term, written as a
// project convention/binding file.
const brandYAML = `name: Project Brand
vocabulary:
  forbidden_terms:
    - term: utilize
      replacement: use
      severity: minor
`

// TestResolveVoiceProfile_FromProjectBinding asserts that with no profile
// flag, brand resolution falls back to defaults.voice.profile_file in
// the .kapi recipe, resolved relative to the project root.
func TestResolveVoiceProfile_FromProjectBinding(t *testing.T) {
	root := writeProjectRecipe(t, `version: v1
name: proj
defaults:
  source_language: en
  target_languages: [fr]
  voice:
    profile_file: brand.yaml
`)
	require.NoError(t, os.WriteFile(filepath.Join(root, "brand.yaml"), []byte(brandYAML), 0o644))
	t.Chdir(root)

	a := &App{}
	cmd := newVoiceCheckCmd(a)

	profile, src, err := a.ResolveVoiceProfileCmd(cmd)
	require.NoError(t, err, "no flag + project binding must resolve, not error")
	require.NotNil(t, profile)
	assert.Equal(t, "Project Brand", profile.Name)
	assert.Equal(t, filepath.Join(root, "brand.yaml"), src)
}

// TestResolveVoiceProfile_FromConventionFile asserts that with no flag and no
// recipe binding, brand resolution falls back to a brand.yaml convention file
// at the project root.
func TestResolveVoiceProfile_FromConventionFile(t *testing.T) {
	root := writeProjectRecipe(t, `version: v1
name: proj
defaults:
  source_language: en
  target_languages: [fr]
`)
	require.NoError(t, os.WriteFile(filepath.Join(root, "brand.yaml"), []byte(brandYAML), 0o644))
	t.Chdir(root)

	a := &App{}
	cmd := newVoiceCheckCmd(a)

	profile, src, err := a.ResolveVoiceProfileCmd(cmd)
	require.NoError(t, err)
	require.NotNil(t, profile)
	assert.Equal(t, "Project Brand", profile.Name)
	assert.Equal(t, filepath.Join(root, "brand.yaml"), src)
}

// TestResolveVoiceProfile_NoProjectNoFlag asserts the original "specify a
// profile" error still fires when there is no flag, no project binding, and
// no convention file.
func TestResolveVoiceProfile_NoProjectNoFlag(t *testing.T) {
	// An empty temp dir with no .kapi recipe anywhere up the tree.
	t.Chdir(t.TempDir())

	a := &App{}
	cmd := newVoiceCheckCmd(a)

	_, _, err := a.ResolveVoiceProfileCmd(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "specify a profile")
}

// TestResolveVoiceProfile_ExplicitFlagWins asserts that an explicit
// --profile-file flag still works unchanged even inside a project.
func TestResolveVoiceProfile_ExplicitFlagWins(t *testing.T) {
	root := writeProjectRecipe(t, `version: v1
name: proj
defaults:
  voice:
    profile_file: brand.yaml
`)
	require.NoError(t, os.WriteFile(filepath.Join(root, "brand.yaml"), []byte(brandYAML), 0o644))

	explicit := filepath.Join(root, "explicit.yaml")
	require.NoError(t, os.WriteFile(explicit, []byte("name: Explicit\n"), 0o644))
	t.Chdir(root)

	a := &App{}
	cmd := newVoiceCheckCmd(a)
	require.NoError(t, cmd.Flags().Set("profile-file", explicit))

	profile, _, err := a.ResolveVoiceProfileCmd(cmd)
	require.NoError(t, err)
	require.NotNil(t, profile)
	assert.Equal(t, "Explicit", profile.Name, "explicit flag must override the project binding")
}

// seedProjectTerms puts one en→fr concept into the PROJECT's own store.
func seedProjectTerms(t *testing.T, root string) {
	t.Helper()
	seedTermsStore(t, root, terms.Concept{
		ID: "c1",
		Terms: []terms.Term{
			{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "Enregistrer", Locale: model.LocaleFrench, Status: model.TermPreferred},
		},
	})
}

// seedTermsStore writes concepts into the project store at root.
func seedTermsStore(t *testing.T, root string, concepts ...terms.Concept) {
	t.Helper()
	db, err := projectdb.Open(t.Context(), project.Layout{
		Root: root, StateDir: filepath.Join(root, project.StateDirName),
	})
	require.NoError(t, err)
	defer func() { require.NoError(t, db.Close()) }()
	for _, c := range concepts {
		require.NoError(t, db.Terms().AddConcept(t.Context(), c))
	}
}

// TestResolveProjectGlossary_FromProjectStore asserts that with no --termstore
// flag and no profile binding, the project's own store builds the vocabulary.
// That is the whole binding story now: a recipe carries no terms path, so being
// in a project IS the binding.
func TestResolveProjectGlossary_FromProjectStore(t *testing.T) {
	root := writeProjectRecipe(t, `version: v1
name: proj
defaults:
  source_language: en
  target_languages: [fr]
`)
	seedProjectTerms(t, root)
	t.Chdir(root)

	a := &App{SourceLang: "en"}
	// A command without a --termstore flag still resolves the project terms.
	cmd := newVoiceCheckCmd(a)

	glossary, err := a.ResolveProjectGlossary(cmd, "fr")
	require.NoError(t, err)
	require.Len(t, glossary, 1)
	assert.Equal(t, "Save", glossary[0].Source)
	assert.Equal(t, "Enregistrer", glossary[0].Target)
}

// TestResolveProjectGlossary_FromProfileTerms asserts a profile's standalone
// `terms:` (relative to the project root) is honoured over the project store —
// the surviving per-point vocabulary binding.
func TestResolveProjectGlossary_FromProfileTerms(t *testing.T) {
	root := writeProjectRecipe(t, `version: v1
name: proj
defaults:
  source_language: en
  target_languages: [fr]
coordinates:
  product:
    - id: press
profiles:
  - terms: brand-terms.db
`)
	// The project store says one thing; the profile's standalone store says
	// another, and the profile wins.
	seedProjectTerms(t, root)
	dbPath := filepath.Join(root, "brand-terms.db")
	tb, err := terms.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
		ID: "c1",
		Terms: []terms.Term{
			{Text: "Cancel", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "Annuler", Locale: model.LocaleFrench, Status: model.TermPreferred},
		},
	}))
	require.NoError(t, tb.Close())
	t.Chdir(root)

	a := &App{SourceLang: "en"}
	cmd := newVoiceCheckCmd(a)

	glossary, err := a.ResolveProjectGlossary(cmd, "fr")
	require.NoError(t, err)
	require.Len(t, glossary, 1)
	assert.Equal(t, "Cancel", glossary[0].Source)
	assert.Equal(t, "Annuler", glossary[0].Target)
}

// TestTermstoreFlagIsRegisteredAndRead is the wiring guard for --termstore.
//
// The flag is registered on several surfaces and read back by string name, so
// a rename that misses one site does not fail to compile. It reads empty, the
// resolver silently falls back to the project store, and the user's explicit
// choice is ignored with no diagnostic at all. Both halves are asserted here:
// the real command surface registers the flag, and the resolver honours it over
// the project binding.
func TestTermstoreFlagIsRegisteredAndRead(t *testing.T) {
	root := writeProjectRecipe(t, `version: v1
name: proj
defaults:
  source_language: en
  target_languages: [fr]
`)
	seedProjectTerms(t, root) // the project store: Save → Enregistrer

	// A different store, selected explicitly.
	named := filepath.Join(t.TempDir(), "named.db")
	tb, err := terms.NewSQLiteStore(named)
	require.NoError(t, err)
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
		ID: "c1",
		Terms: []terms.Term{
			{Text: "Cancel", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "Annuler", Locale: model.LocaleFrench, Status: model.TermPreferred},
		},
	}))
	require.NoError(t, tb.Close())
	t.Chdir(root)

	// A tool that requires a terms store gets the flag from its Requires
	// metadata — the real `kapi exec term-check` surface.
	termCheck := execChildren(t, newTestApp())["term-check"]
	require.NotNil(t, termCheck, "expected `exec term-check`")
	require.NotNil(t, termCheck.Flags().Lookup("termstore"),
		"`exec term-check` selects a terms store, so --termstore must be registered")
	require.Nil(t, termCheck.Flags().Lookup("terms"),
		"the flag is --termstore; the old spelling must not linger")
	require.NoError(t, termCheck.Flags().Set("termstore", named))

	a := &App{SourceLang: "en"}
	glossary, err := a.ResolveProjectGlossary(termCheck, "fr")
	require.NoError(t, err)
	require.Len(t, glossary, 1)
	assert.Equal(t, "Cancel", glossary[0].Source,
		"--termstore must select the named store, not fall back to the project's")
	assert.Equal(t, "Annuler", glossary[0].Target)
}

// TestResolveProjectGlossary_NoProject returns nil (no error) when there is no
// project in scope.
func TestResolveProjectGlossary_NoProject(t *testing.T) {
	t.Chdir(t.TempDir())
	a := &App{SourceLang: "en"}
	cmd := newVoiceCheckCmd(a)
	glossary, err := a.ResolveProjectGlossary(cmd, "fr")
	require.NoError(t, err)
	assert.Nil(t, glossary)
}

// TestTermCheck_EnforcesProjectGlossary proves the end-to-end chain: the
// project terms store glossary, injected as the term-check tool's config, makes
// the tool flag the violation. This mirrors what the term-check command's
// newTool closure does inside a project.
func TestTermCheck_EnforcesProjectGlossary(t *testing.T) {
	root := writeProjectRecipe(t, `version: v1
name: proj
defaults:
  source_language: en
  target_languages: [fr]
`)
	seedProjectTerms(t, root) // Save → Enregistrer
	t.Chdir(root)

	a := &App{SourceLang: "en"}
	a.InitRegistries()
	cmd := newVoiceCheckCmd(a)

	glossary, err := a.ResolveProjectGlossary(cmd, "fr")
	require.NoError(t, err)
	require.Len(t, glossary, 1)

	// Build term-check exactly as the toolcmds newTool closure would.
	config := map[string]any{"glossary": glossary}
	tl, err := a.ToolReg.NewToolWithConfig(registryToolID("term-check"), config, "fr")
	require.NoError(t, err)

	// A target that violates the glossary (Save → not Enregistrer).
	block := model.NewBlock("tu1", "Save the file")
	block.SetTargetText(model.LocaleFrench, "Sauvegarder le fichier")
	part := &model.Part{Type: model.PartBlock, Resource: block}

	out := runPartThroughTool(t, tl, part)
	resultBlock := out.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties["term-check-passed"],
		"project glossary should be enforced flag-free")
	assert.Contains(t, resultBlock.Properties["term-check-errors"], "Enregistrer")
}

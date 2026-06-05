package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/termbase"
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
	require.NoError(t, os.WriteFile(filepath.Join(dir, "proj.kapi"), []byte(recipe), 0o644))
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

// TestResolveBrandProfile_FromProjectBinding asserts that with no profile
// flag, brand resolution falls back to defaults.brand_voice.profile_file in
// the .kapi recipe, resolved relative to the project root.
func TestResolveBrandProfile_FromProjectBinding(t *testing.T) {
	root := writeProjectRecipe(t, `version: v1
name: proj
defaults:
  source_language: en
  target_languages: [fr]
  brand_voice:
    profile_file: brand.yaml
`)
	require.NoError(t, os.WriteFile(filepath.Join(root, "brand.yaml"), []byte(brandYAML), 0o644))
	t.Chdir(root)

	a := &App{}
	cmd := a.newBrandCheckCmd()

	profile, src, err := a.resolveBrandProfile(cmd)
	require.NoError(t, err, "no flag + project binding must resolve, not error")
	require.NotNil(t, profile)
	assert.Equal(t, "Project Brand", profile.Name)
	assert.Equal(t, filepath.Join(root, "brand.yaml"), src)
}

// TestResolveBrandProfile_FromConventionFile asserts that with no flag and no
// recipe binding, brand resolution falls back to a brand.yaml convention file
// at the project root.
func TestResolveBrandProfile_FromConventionFile(t *testing.T) {
	root := writeProjectRecipe(t, `version: v1
name: proj
defaults:
  source_language: en
  target_languages: [fr]
`)
	require.NoError(t, os.WriteFile(filepath.Join(root, "brand.yaml"), []byte(brandYAML), 0o644))
	t.Chdir(root)

	a := &App{}
	cmd := a.newBrandCheckCmd()

	profile, src, err := a.resolveBrandProfile(cmd)
	require.NoError(t, err)
	require.NotNil(t, profile)
	assert.Equal(t, "Project Brand", profile.Name)
	assert.Equal(t, filepath.Join(root, "brand.yaml"), src)
}

// TestResolveBrandProfile_NoProjectNoFlag asserts the original "specify a
// profile" error still fires when there is no flag, no project binding, and
// no convention file.
func TestResolveBrandProfile_NoProjectNoFlag(t *testing.T) {
	// An empty temp dir with no .kapi recipe anywhere up the tree.
	t.Chdir(t.TempDir())

	a := &App{}
	cmd := a.newBrandCheckCmd()

	_, _, err := a.resolveBrandProfile(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "specify a profile")
}

// TestResolveBrandProfile_ExplicitFlagWins asserts that an explicit
// --profile-file flag still works unchanged even inside a project.
func TestResolveBrandProfile_ExplicitFlagWins(t *testing.T) {
	root := writeProjectRecipe(t, `version: v1
name: proj
defaults:
  brand_voice:
    profile_file: brand.yaml
`)
	require.NoError(t, os.WriteFile(filepath.Join(root, "brand.yaml"), []byte(brandYAML), 0o644))

	explicit := filepath.Join(root, "explicit.yaml")
	require.NoError(t, os.WriteFile(explicit, []byte("name: Explicit\n"), 0o644))
	t.Chdir(root)

	a := &App{}
	cmd := a.newBrandCheckCmd()
	require.NoError(t, cmd.Flags().Set("profile-file", explicit))

	profile, _, err := a.resolveBrandProfile(cmd)
	require.NoError(t, err)
	require.NotNil(t, profile)
	assert.Equal(t, "Explicit", profile.Name, "explicit flag must override the project binding")
}

// seedProjectTermbase creates <root>/.kapi/termbase.db with one en→fr concept.
func seedProjectTermbase(t *testing.T, root string) {
	t.Helper()
	dbPath := filepath.Join(root, ".kapi", "termbase.db")
	tb, err := termbase.NewSQLiteTermBase(dbPath)
	require.NoError(t, err)
	defer tb.Close()
	require.NoError(t, tb.AddConcept(t.Context(), termbase.Concept{
		ID: "c1",
		Terms: []termbase.Term{
			{Text: "Save", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "Enregistrer", Locale: model.LocaleFrench, Status: model.TermPreferred},
		},
	}))
}

// TestResolveProjectGlossary_FromConventionTermbase asserts that with no
// --termbase flag and no defaults.termbase, the convention
// <root>/.kapi/termbase.db is used to build the project glossary.
func TestResolveProjectGlossary_FromConventionTermbase(t *testing.T) {
	root := writeProjectRecipe(t, `version: v1
name: proj
defaults:
  source_language: en
  target_languages: [fr]
`)
	seedProjectTermbase(t, root)
	t.Chdir(root)

	a := &App{SourceLang: "en"}
	// A command without a --termbase flag still resolves the project termbase.
	cmd := a.newBrandCheckCmd()

	glossary, err := a.resolveProjectGlossary(cmd, "fr")
	require.NoError(t, err)
	require.Len(t, glossary, 1)
	assert.Equal(t, "Save", glossary[0].Source)
	assert.Equal(t, "Enregistrer", glossary[0].Target)
}

// TestResolveProjectGlossary_FromBoundTermbase asserts that defaults.termbase
// (relative to the project root) is honored when set.
func TestResolveProjectGlossary_FromBoundTermbase(t *testing.T) {
	root := writeProjectRecipe(t, `version: v1
name: proj
defaults:
  source_language: en
  target_languages: [fr]
  termbase: glossary.db
`)
	// Bound termbase at the project root (not the .kapi convention path).
	dbPath := filepath.Join(root, "glossary.db")
	tb, err := termbase.NewSQLiteTermBase(dbPath)
	require.NoError(t, err)
	require.NoError(t, tb.AddConcept(t.Context(), termbase.Concept{
		ID: "c1",
		Terms: []termbase.Term{
			{Text: "Cancel", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "Annuler", Locale: model.LocaleFrench, Status: model.TermPreferred},
		},
	}))
	require.NoError(t, tb.Close())
	t.Chdir(root)

	a := &App{SourceLang: "en"}
	cmd := a.newBrandCheckCmd()

	glossary, err := a.resolveProjectGlossary(cmd, "fr")
	require.NoError(t, err)
	require.Len(t, glossary, 1)
	assert.Equal(t, "Cancel", glossary[0].Source)
	assert.Equal(t, "Annuler", glossary[0].Target)
}

// TestResolveProjectGlossary_NoProject returns nil (no error) when there is no
// project in scope.
func TestResolveProjectGlossary_NoProject(t *testing.T) {
	t.Chdir(t.TempDir())
	a := &App{SourceLang: "en"}
	cmd := a.newBrandCheckCmd()
	glossary, err := a.resolveProjectGlossary(cmd, "fr")
	require.NoError(t, err)
	assert.Nil(t, glossary)
}

// TestTermCheck_EnforcesProjectGlossary proves the end-to-end chain: the
// project termbase glossary, injected as the term-check tool's config, makes
// the tool flag the violation. This mirrors what the term-check command's
// newTool closure does inside a project.
func TestTermCheck_EnforcesProjectGlossary(t *testing.T) {
	root := writeProjectRecipe(t, `version: v1
name: proj
defaults:
  source_language: en
  target_languages: [fr]
`)
	seedProjectTermbase(t, root) // Save → Enregistrer
	t.Chdir(root)

	a := &App{SourceLang: "en"}
	a.InitRegistries()
	cmd := a.newBrandCheckCmd()

	glossary, err := a.resolveProjectGlossary(cmd, "fr")
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

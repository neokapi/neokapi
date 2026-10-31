package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/memory"
)

// Where a redacted .kpz refuses: a value it cannot withhold, a vault that
// does not restore it, a placeholder a translator changed, and a run that
// would send what the package's own rules did not withhold.

func TestMergeRedactedWorkspaceRefusesWithoutItsVault(t *testing.T) {
	a, work, dir := adHocRedactedPackage(t, false)
	returnTranslation(t, work)
	vault := work + ".vault"
	merged := filepath.Join(dir, "translated")

	t.Run("absent", func(t *testing.T) {
		require.NoError(t, os.Rename(vault, vault+".moved"))
		t.Cleanup(func() { _ = os.Rename(vault+".moved", vault) })
		err := a.MergeFromKpz(t.Context(), work, merged)
		require.Error(t, err)
		assert.Equal(t, "merge: work.kpz is redacted, and its vault is not at "+vault+
			". kapi merge wrote no file. Copy the vault kapi extract wrote to that path, or extract the package again", err.Error())
		assert.NoDirExists(t, merged, "the merge writes no file")
	})
	t.Run("a vault that records no ID", func(t *testing.T) {
		data, err := os.ReadFile(vault)
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.WriteFile(vault, data, 0o600) })
		require.NoError(t, os.WriteFile(vault, []byte(`{"version": 2, "values": []}`), 0o600))
		err = a.MergeFromKpz(t.Context(), work, merged)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is vault none. kapi merge wrote no file")
		assert.NoDirExists(t, merged, "the merge writes no file")
	})
	t.Run("a vault that does not open", func(t *testing.T) {
		data, err := os.ReadFile(vault)
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.WriteFile(vault, data, 0o600) })
		require.NoError(t, os.WriteFile(vault, []byte(`not a vault`), 0o600))
		err = a.MergeFromKpz(t.Context(), work, merged)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "merge: work.kpz is redacted, and its vault cannot be read: ")
		assert.True(t, strings.HasSuffix(err.Error(), ". Run kapi extract again to write a new package and vault"), err.Error())
		assert.NoDirExists(t, merged, "the merge writes no file")
	})
	t.Run("a vault a newer kapi wrote", func(t *testing.T) {
		data, err := os.ReadFile(vault)
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.WriteFile(vault, data, 0o600) })
		require.NoError(t, os.WriteFile(vault, []byte(`{"version": 3, "values": []}`), 0o600))
		err = a.MergeFromKpz(t.Context(), work, merged)
		require.ErrorIs(t, err, redaction.ErrNewerVault)
		assert.Contains(t, err.Error(), "upgrade kapi to read it")
		assert.NotContains(t, err.Error(), "kapi extract again")
		assert.NoDirExists(t, merged, "the merge writes no file")
	})
}

// The vault is the only copy of the values, so an extract to a package whose
// vault is still beside it writes nothing and keeps the vault.
func TestExtractRedactedWorkspaceKeepsAnEarlierVault(t *testing.T) {
	a, work, dir := adHocRedactedPackage(t, false)
	before, err := os.ReadFile(work + ".vault")
	require.NoError(t, err)
	err = a.ExtractRedactedToKpz(t.Context(), []string{filepath.Join(dir, "a.json")}, work, "fr", "", false, filepath.Join(dir, "redact.yaml"))
	require.Error(t, err)
	assert.Equal(t, "extract: "+work+".vault already holds the values an earlier extract withheld. Merge that package and remove the vault, or extract to another name", err.Error())
	after, err := os.ReadFile(work + ".vault")
	require.NoError(t, err)
	assert.Equal(t, before, after, "the earlier vault is kept as it is")
}

// adHocExtract writes src with content and extracts it with --redact-rules
// over the names, returning the error.
func adHocExtract(t *testing.T, content string, names ...string) (string, error) {
	t.Helper()
	t.Setenv("KAPI_KPZ_CACHE", t.TempDir())
	dir := t.TempDir()
	src := filepath.Join(dir, "app.json")
	require.NoError(t, os.WriteFile(src, []byte(content), 0o644))
	rules := &redaction.RulesFile{}
	for _, n := range names {
		rules.Rules = append(rules.Rules, redaction.Rule{Term: n, Category: redaction.CategoryPerson})
	}
	rulesPath := filepath.Join(dir, "redact.yaml")
	require.NoError(t, rules.Save(rulesPath))
	a := &App{SourceLang: "en", Quiet: true}
	a.InitRegistries()
	work := filepath.Join(dir, "work.kpz")
	return work, a.ExtractRedactedToKpz(t.Context(), []string{src}, work, "fr", "", true, rulesPath)
}

// A file that holds a withheld value outside the blocks kapi can withhold it
// from would put the value in the package, so extract writes none: a value
// in the markup beside the same value in a block, and a value that appears
// only in the markup, which the vault never records.
func TestExtractRedactedWorkspaceRefusesAValueOutsideTheBlocks(t *testing.T) {
	for name, content := range map[string]string{
		"beside a block that withholds it": `{"Alice Archer":"Hello Alice Archer"}`,
		"only in the markup":               `{"Alice Archer":"Hello there","b":"Hi Bruno Black"}`,
	} {
		t.Run(name, func(t *testing.T) {
			work, err := adHocExtract(t, content, "Alice Archer", "Bruno Black")
			require.Error(t, err)
			assert.Equal(t, "extract: app.json holds a withheld value outside any block (in markup, in a field kapi does not read, or across an inline code). kapi wrote no package. Extract XLIFF 2 or PO instead (--format xliff2 or --format po)", err.Error())
			assert.NoFileExists(t, work)
			assert.NoFileExists(t, work+".vault")
		})
	}
}

// A content-memory match or a terms concept that names a value the policy
// withholds from another file stays out of the package too.
func TestExtractKpzLeavesOutAMemoryMatchThatNamesAWithheldValue(t *testing.T) {
	a, recipe, dir := vaultProject(t)
	src := filepath.Join(dir, "src", "locales", "en", "a.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"greeting":"Welcome back to your account settings page, Alice Archer."}`+"\n"), 0o644))
	rules := &redaction.RulesFile{Rules: []redaction.Rule{
		{Term: "Alice Archer", Category: redaction.CategoryPerson},
		{Term: "Bruno Black", Category: redaction.CategoryPerson},
		{Term: "Carla Cruz", Category: redaction.CategoryPerson},
	}}
	require.NoError(t, rules.Save(filepath.Join(dir, "redact.yaml")))
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	db, err := a.ProjectDB(context.Background(), layout.Root)
	require.NoError(t, err)
	require.NoError(t, db.Memory().Add(context.Background(), memory.Entry{
		ID: "m1", HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {model.TextR("Welcome back to your account settings page, Carla Cruz.")},
			"fr": {model.TextR("Bon retour sur la page de votre compte, Carla Cruz.")},
		},
	}))
	out := t.TempDir()
	cmd := kpzExtractCommand(t, recipe, out)
	require.NoError(t, cmd.Flags().Set("no-memory", "false"))
	require.NoError(t, a.RunExtractKpz(cmd))
	packages := kpzPackages(t, out)
	require.NotEmpty(t, packages)
	for _, p := range packages {
		assertPackageWithholds(t, p, map[string]string{"a": "Alice Archer", "b": "Bruno Black", "c": "Carla Cruz"})
	}
}

// A policy that detects entities needs an entity-extract step a .kpz
// extraction does not run, and one with no rules file finds nothing, so
// neither writes a package that records it is redacted.
func TestExtractKpzRefusesAPolicyItCannotApply(t *testing.T) {
	for name, tc := range map[string]struct {
		spec *project.RedactionSpec
		want string
	}{
		"entities": {
			spec: &project.RedactionSpec{Enabled: true, Detectors: []string{"rules", "entities"}, Rules: "redact.yaml"},
			want: "extract: the redaction policy at defaults.redaction detects entities, which needs an entity-extract step that a .kpz extraction does not run",
		},
		"no rules": {
			spec: &project.RedactionSpec{Enabled: true},
			want: "no rules file is set",
		},
	} {
		t.Run(name, func(t *testing.T) {
			a, recipe, _ := vaultProject(t)
			proj, err := project.Load(recipe)
			require.NoError(t, err)
			proj.Defaults.Redaction = tc.spec
			require.NoError(t, project.Save(recipe, proj))
			out := t.TempDir()
			err = a.RunExtractKpz(kpzExtractCommand(t, recipe, out))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Empty(t, kpzPackages(t, out), "no package is written")
		})
	}
}

// The revisions a package records for merge are those of the source as the
// project holds it, so a merge can tell a unit whose source moved. They are
// hashes of the original runs.
func TestRedactedKpzRevisionsAreThoseOfTheProjectSource(t *testing.T) {
	a, recipe, dir := vaultProject(t)
	out := t.TempDir()
	require.NoError(t, a.RunExtractKpz(kpzExtractCommand(t, recipe, out)))
	for _, p := range kpzPackages(t, out) {
		pkg, err := LoadWorkspace(p)
		require.NoError(t, err)
		doc := pkg.Sources[0].SourcePath
		blocks, _, err := project.ReadSourceBlocks(context.Background(), a.FormatReg, "json", filepath.Join(dir, filepath.FromSlash(doc)), "en", "", nil)
		require.NoError(t, err)
		assert.Equal(t, extractedRevision(blocks[0], "en", nil).Basis, pkg.InterchangeTask.Revisions["tu1"].Basis, doc)
	}
}

// A placeholder a translator changed survives the restore, so the merge
// stops before it writes the file, on both routes.
func TestMergeRedactedKpzRefusesAChangedPlaceholder(t *testing.T) {
	changed := []model.Run{model.TextR("Bonjour [REDACTED:Personne]")}
	t.Run("project", func(t *testing.T) {
		a, recipe, dir := vaultProject(t)
		out := t.TempDir()
		require.NoError(t, a.RunExtractKpz(kpzExtractCommand(t, recipe, out)))
		packages := kpzPackages(t, out)
		require.NotEmpty(t, packages)
		returnTranslationRuns(t, packages[0], "tu1", changed)
		err := a.MergeOneKpz(mergeKpzCommand(t, recipe), packages[0])
		require.Error(t, err)
		assert.Contains(t, err.Error(), ", block tu1: the translation holds a placeholder no value in the vault matches (\"[REDACTED:\"…), so a translator changed it")
		assertNoFrenchFile(t, dir)
	})
	t.Run("source files", func(t *testing.T) {
		a, work, dir := adHocRedactedPackage(t, false)
		returnTranslationRuns(t, work, "tu1", changed)
		merged := filepath.Join(dir, "translated")
		err := a.MergeFromKpz(t.Context(), work, merged)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "block tu1: the translation holds a placeholder no value in the vault matches")
		assert.NoDirExists(t, merged, "the merge writes no file")
	})
}

// arbProject is a project with one ARB file whose plural message names a
// person in every form, under a policy that withholds the name.
func arbProject(t *testing.T) (*App, string, string) {
	t.Helper()
	dir := t.TempDir()
	isolateKapiEnv(t)
	t.Setenv("KAPI_KPZ_CACHE", t.TempDir())
	src := filepath.Join(dir, "l10n", "app_en.arb")
	require.NoError(t, os.MkdirAll(filepath.Dir(src), 0o755))
	require.NoError(t, os.WriteFile(src, []byte(`{
  "@@locale": "en",
  "inbox": "{count, plural, one{Alice Archer sent one message} other{Alice Archer sent {count} messages}}"
}
`), 0o644))
	rules := &redaction.RulesFile{Rules: []redaction.Rule{{Term: "Alice Archer", Category: redaction.CategoryPerson}}}
	require.NoError(t, rules.Save(filepath.Join(dir, "redact.yaml")))
	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "ArbPlural",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
			Redaction:       &project.RedactionSpec{Enabled: true, Rules: "redact.yaml"},
		},
		Collections: []project.Collection{{
			Path:   "l10n/app_en.arb",
			Format: &project.FormatSpec{Name: "arb"},
			Target: "l10n/app_{lang}.arb",
		}},
	}
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	require.NoError(t, project.EnsureLayout(layout))
	a := &App{SourceLang: "en", Quiet: true}
	a.InitRegistries()
	return a, recipe, dir
}

// A plural translation carries the placeholder in each of its forms, and the
// merge restores the name in every one.
func TestMergeRedactedKpzRestoresEveryPluralForm(t *testing.T) {
	a, recipe, dir := arbProject(t)
	out := t.TempDir()
	require.NoError(t, a.RunExtractKpz(kpzExtractCommand(t, recipe, out)))
	packages := kpzPackages(t, out)
	require.Len(t, packages, 1)
	assertPackageWithholds(t, packages[0], map[string]string{"a": "Alice Archer"})

	// The translator keeps the placeholder and the {count} code in each form.
	blocks, _, err := project.ReadSourceBlocks(context.Background(), a.FormatReg, "arb", filepath.Join(dir, "l10n", "app_en.arb"), "en", "", nil)
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	src := blocks[0].SourceRuns()
	require.Len(t, src, 1)
	require.NotNil(t, src[0].Plural)
	forms := map[model.PluralForm][]model.Run{}
	for form, runs := range src[0].Plural.Forms {
		var tr []model.Run
		for _, r := range runs {
			if r.Text != nil {
				r = model.TextR(strings.ReplaceAll(strings.ReplaceAll(r.Text.Text, "Alice Archer", "[REDACTED:Person]"), "sent", "a envoyé"))
			}
			tr = append(tr, r)
		}
		forms[form] = tr
	}
	returnTranslationRuns(t, packages[0], "tu1", []model.Run{{Plural: &model.PluralRun{Pivot: src[0].Plural.Pivot, Forms: forms}}})
	require.NoError(t, a.MergeOneKpz(mergeKpzCommand(t, recipe), packages[0]))
	data, err := os.ReadFile(filepath.Join(dir, "l10n", "app_fr.arb"))
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(string(data), "Alice Archer"), "the name is back in both forms: %s", data)
	assert.NotContains(t, string(data), "REDACTED")
}

// A pack keeps the profile: an interchange package a receiver works on and
// packs still records its kind, its task and each file's project path, so it
// merges back into the project as an interchange package.
func TestPackKeepsTheInterchangeProfile(t *testing.T) {
	a, recipe, dir := vaultProject(t)
	t.Setenv("KAPI_KPZ_CACHE", t.TempDir())
	out := t.TempDir()
	require.NoError(t, a.RunExtractKpz(kpzExtractCommand(t, recipe, out)))
	packages := kpzPackages(t, out)
	require.NotEmpty(t, packages)
	p := packages[0]
	before, err := LoadWorkspace(p)
	require.NoError(t, err)
	returnTranslation(t, p)

	// The receiver, with a cache of its own.
	t.Setenv("KAPI_KPZ_CACHE", t.TempDir())
	require.NoError(t, runToolOnKpz(a, p, "case-transform", map[string]any{"mode": "upper"}))
	require.NoError(t, a.PackKpz(t.Context(), p))
	after, err := LoadWorkspace(p)
	require.NoError(t, err)
	assert.Equal(t, before.Kind, after.Kind)
	require.NotNil(t, after.InterchangeTask)
	assert.Equal(t, before.InterchangeTask.SourceFiles, after.InterchangeTask.SourceFiles)
	require.Len(t, after.Sources, 1)
	assert.Equal(t, before.Sources[0].SourcePath, after.Sources[0].SourcePath)
	require.NotNil(t, after.Redaction)
	assert.Equal(t, before.Redaction.VaultID, after.Redaction.VaultID)
	assertPackageWithholds(t, p, vaultNames)

	require.NoError(t, a.MergeOneKpz(mergeKpzCommand(t, recipe), p))
	doc := before.Sources[0].SourcePath
	data, err := os.ReadFile(filepath.Join(dir, "src", "locales", "fr", filepath.Base(doc)))
	require.NoError(t, err)
	assert.Contains(t, string(data), vaultNames[doc], "the merge restores the name")
}

// collectionPolicyProject is the vault fixture with its redaction policy
// declared on the collection, under which a run that sends blocks out over a
// package is refused.
func collectionPolicyProject(t *testing.T) (*App, string, string) {
	t.Helper()
	a, recipe, dir := vaultProject(t)
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Collections[0].Redaction = proj.Defaults.Redaction
	proj.Defaults.Redaction = nil
	require.NoError(t, project.Save(recipe, proj))
	t.Setenv("KAPI_KPZ_CACHE", t.TempDir())
	return a, recipe, dir
}

// kapi exec over a redacted package in a project with a collection's policy:
// a tool that sends blocks out runs over a package kapi extract --format kpz
// wrote in this project, whose vault is among the project's batch vaults. It
// is refused over any other package: one that withholds nothing, one whose
// batch vault is gone, and one extracted from source files under rules the
// project never set, whose vault beside it proves nothing.
func TestExecOverARedactedPackage(t *testing.T) {
	a, recipe, dir := collectionPolicyProject(t)
	srcs := []string{
		filepath.Join(dir, "src", "locales", "en", "a.json"),
		filepath.Join(dir, "src", "locales", "en", "b.json"),
	}
	plain := filepath.Join(dir, "plain.kpz")
	require.NoError(t, a.ExtractToKpz(t.Context(), srcs, plain, "fr", "", true))
	out := filepath.Join(dir, "out")
	require.NoError(t, a.RunExtractKpz(kpzExtractCommand(t, recipe, out)))
	packages := kpzPackages(t, out)
	require.NotEmpty(t, packages)
	work := packages[0]
	pkg, err := LoadWorkspace(work)
	require.NoError(t, err)
	require.NotNil(t, pkg.Redaction)

	t.Run("a package that withholds nothing is refused", func(t *testing.T) {
		config, _ := echoProgram(t)
		err := a.RunToolOnFiles(t.Context(), externalCommandRun(a, recipe, plain, config))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "would run over every file of "+plain+" under one policy")
	})
	t.Run("a package this project extracted runs", func(t *testing.T) {
		config, saw := echoProgram(t)
		require.NoError(t, a.RunToolOnFiles(t.Context(), externalCommandRun(a, recipe, work, config)))
		if read, err := os.ReadFile(saw); err == nil {
			for _, name := range vaultNames {
				assert.NotContains(t, string(read), name, "the program never reads a withheld name")
			}
		}
	})
	t.Run("a package whose batch vault is gone is refused", func(t *testing.T) {
		vault := project.LayoutAt(dir).RedactionSidecarPath(pkg.Redaction.VaultID)
		require.NoError(t, os.Rename(vault, vault+".moved"))
		t.Cleanup(func() { _ = os.Rename(vault+".moved", vault) })
		config, saw := echoProgram(t)
		err := a.RunToolOnFiles(t.Context(), externalCommandRun(a, recipe, work, config))
		require.Error(t, err)
		assert.Equal(t, "kapi refused to run external-command: "+work+" is a redacted package, and its vault is not in this project. The project declares a redaction policy at collections[0].redaction. Run external-command on the project's files", err.Error())
		assert.NoFileExists(t, saw, "the program never runs")

		// A tool that sends nothing out runs.
		require.NoError(t, a.RunToolOnFiles(t.Context(), ToolRunConfig{
			ToolName: "case-transform", Project: recipe, Files: []string{work},
			ToolConfig: map[string]any{"mode": "upper"},
			NewTool: func() (tool.Tool, error) {
				return a.ToolReg.NewToolWithConfig(registry.ToolID("case-transform"), map[string]any{"mode": "upper"}, "")
			},
		}))
	})
	t.Run("a package extracted from source files under other rules is refused", func(t *testing.T) {
		weak := &redaction.RulesFile{Rules: []redaction.Rule{{Term: "Hello", Category: redaction.CategoryPerson}}}
		weakPath := filepath.Join(t.TempDir(), "weak.yaml")
		require.NoError(t, weak.Save(weakPath))
		adHoc := filepath.Join(dir, "work.kpz")
		require.NoError(t, a.ExtractRedactedToKpz(t.Context(), srcs[:1], adHoc, "fr", "", true, weakPath))
		require.FileExists(t, adHoc+".vault")
		config, saw := echoProgram(t)
		err := a.RunToolOnFiles(t.Context(), externalCommandRun(a, recipe, adHoc, config))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is a redacted package, and its vault is not in this project")
		assert.NoFileExists(t, saw, "the program never reads Alice Archer")
	})
}

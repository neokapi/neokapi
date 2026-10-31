package host

import (
	"archive/zip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/kpz"
)

// A .kpz written under a redaction policy holds a token where the policy
// withholds a value. The vault stays with whoever extracted it, and kapi
// merge restores each file's own values from it. The fixture is two files
// that each withhold a different name in a block with the same ID.

// assertPackageWithholds checks that no member of the package at path, its
// manifest included, holds any of the names.
func assertPackageWithholds(t *testing.T, path string, names map[string]string) {
	t.Helper()
	zr, err := zip.OpenReader(path)
	require.NoError(t, err)
	defer zr.Close()
	require.NotEmpty(t, zr.File)
	for _, f := range zr.File {
		rc, err := f.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(rc)
		require.NoError(t, rc.Close())
		require.NoError(t, err)
		for _, name := range names {
			assert.NotContains(t, string(data), name, "%s in %s withholds the name", f.Name, filepath.Base(path))
		}
	}
}

// returnTranslation writes into the package at path a French translation of
// block tu1 of every file it carries, the token kept and "Hello" put into
// French, as a translator returns it.
func returnTranslation(t *testing.T, path string) {
	t.Helper()
	pkg, err := LoadWorkspace(path)
	require.NoError(t, err)
	kind := blockstore.TargetOverlayKind("fr")
	kept := pkg.Overlays[:0]
	for _, ov := range pkg.Overlays {
		if blockstore.CanonicalOverlayKind(ov.Kind) != kind {
			kept = append(kept, ov)
		}
	}
	pkg.Overlays = kept
	for _, si := range pkg.Sources {
		payload, err := json.Marshal(map[string]string{"text": "Bonjour [REDACTED:Person]", "status": string(model.TargetStatusDraft)})
		require.NoError(t, err)
		pkg.Overlays = append(pkg.Overlays, kpz.OverlayDoc{
			Kind: kind, BlockHash: "tu1", Payload: payload, Source: kpz.SourceDir + filepath.Base(si.SourcePath),
		})
	}
	require.NoError(t, saveWorkspace(pkg, path))
}

// mergeKpzCommand is kapi merge <package> in the project at recipe.
func mergeKpzCommand(t *testing.T, recipe string) *EnvCommand {
	t.Helper()
	cmd, _, _ := mergeCommand(t, recipe)
	cmd.Flags().Bool("no-restore", false, "")
	require.NoError(t, cmd.Flags().Set("no-memory-update", "true"))
	return cmd
}

func TestMergeRedactedKpzRestoresEachFilesName(t *testing.T) {
	a, recipe, dir := vaultProject(t)
	out := t.TempDir()
	require.NoError(t, a.RunExtractKpz(kpzExtractCommand(t, recipe, out)))
	packages := kpzPackages(t, out)
	require.Len(t, packages, len(vaultNames))

	for _, p := range packages {
		returnTranslation(t, p)
		assertPackageWithholds(t, p, vaultNames)
		require.NoError(t, a.MergeOneKpz(mergeKpzCommand(t, recipe), p))
	}
	for doc, name := range vaultNames {
		data, err := os.ReadFile(filepath.Join(dir, "src", "locales", "fr", filepath.Base(doc)))
		require.NoError(t, err, doc)
		assert.Equal(t, `{"greeting":"Bonjour `+name+`"}`, strings.TrimSpace(string(data)), doc)
	}
}

func TestMergeRedactedKpzRefusesWithoutItsVault(t *testing.T) {
	a, recipe, dir := vaultProject(t)
	out := t.TempDir()
	require.NoError(t, a.RunExtractKpz(kpzExtractCommand(t, recipe, out)))
	packages := kpzPackages(t, out)
	require.NotEmpty(t, packages)
	pkg, err := LoadWorkspace(packages[0])
	require.NoError(t, err)
	vault := project.LayoutAt(dir).RedactionSidecarPath(pkg.Redaction.VaultID)
	returnTranslation(t, packages[0])

	t.Run("absent", func(t *testing.T) {
		moved := vault + ".moved"
		require.NoError(t, os.Rename(vault, moved))
		t.Cleanup(func() { _ = os.Rename(moved, vault) })
		err := a.MergeOneKpz(mergeKpzCommand(t, recipe), packages[0])
		require.Error(t, err)
		assert.Equal(t, "merge: "+filepath.Base(packages[0])+" is a redacted package: it holds a placeholder wherever its redaction policy withheld a value, and the vault that restores them is not at "+vault+
			", so kapi merge writes nothing. Merge it in the project that extracted it, where kapi extract kept the vault under .kapi/vault/batches/, or extract the package again to write a new package and vault.", err.Error())
		assertNoFrenchFile(t, dir)
	})
	t.Run("another vault", func(t *testing.T) {
		data, err := os.ReadFile(vault)
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.WriteFile(vault, data, 0o600) })
		other, err := redaction.OpenFileVault(vault)
		require.NoError(t, err)
		other.SetID("00000000000000000000000000000000")
		require.NoError(t, other.Flush())
		err = a.MergeOneKpz(mergeKpzCommand(t, recipe), packages[0])
		require.Error(t, err)
		assert.Contains(t, err.Error(), "names vault "+pkg.Redaction.VaultID+", and the vault at "+vault+" is vault 00000000000000000000000000000000, so kapi merge writes nothing")
		assertNoFrenchFile(t, dir)
	})
}

func assertNoFrenchFile(t *testing.T, dir string) {
	t.Helper()
	for doc := range vaultNames {
		assert.NoFileExists(t, filepath.Join(dir, "src", "locales", "fr", filepath.Base(doc)), "the merge writes no target file")
	}
}

// adHocNames is the name each file of the ad-hoc fixture withholds, by the
// name the package gives the file.
var adHocNames = map[string]string{
	"a.json": "Alice Archer",
	"b.json": "Bruno Black",
}

// adHocRedactedPackage writes the ad-hoc fixture and extracts it with
// kapi extract a.json b.json -o work.kpz --redact-rules redact.yaml
// --target-lang fr, the raw source carried with withSource.
func adHocRedactedPackage(t *testing.T, withSource bool) (*App, string, string) {
	t.Helper()
	t.Setenv("KAPI_KPZ_CACHE", t.TempDir())
	dir := t.TempDir()
	var srcs []string
	for base, name := range adHocNames {
		src := filepath.Join(dir, base)
		require.NoError(t, os.WriteFile(src, []byte(`{"greeting":"Hello `+name+`"}`+"\n"), 0o644))
		srcs = append(srcs, src)
	}
	rules := &redaction.RulesFile{Rules: []redaction.Rule{
		{Term: "Alice Archer", Category: redaction.CategoryPerson},
		{Term: "Bruno Black", Category: redaction.CategoryPerson},
	}}
	rulesPath := filepath.Join(dir, "redact.yaml")
	require.NoError(t, rules.Save(rulesPath))

	a := &App{SourceLang: "en", Quiet: true}
	a.InitRegistries()
	work := filepath.Join(dir, "work.kpz")
	require.NoError(t, a.ExtractRedactedToKpz(t.Context(), srcs, work, "fr", "", withSource, rulesPath))
	return a, work, dir
}

func TestExtractRedactedWorkspaceRoundTrip(t *testing.T) {
	for _, withSource := range []bool{false, true} {
		t.Run(map[bool]string{false: "skeleton", true: "with source"}[withSource], func(t *testing.T) {
			a, work, dir := adHocRedactedPackage(t, withSource)
			assertPackageWithholds(t, work, adHocNames)

			pkg, err := LoadWorkspace(work)
			require.NoError(t, err)
			require.NotNil(t, pkg.Redaction, "the manifest records that the package is redacted")
			if withSource {
				require.Len(t, pkg.Source, len(adHocNames))
				for _, s := range pkg.Source {
					data, err := kpz.ReadAll(s.Content)
					require.NoError(t, err)
					assert.Equal(t, `{"greeting":"Hello [REDACTED:Person]"}`, strings.TrimSpace(string(data)), s.Path)
				}
			}
			v, err := redaction.OpenFileVault(work + ".vault")
			require.NoError(t, err, "the vault sits beside the package")
			assert.Equal(t, pkg.Redaction.VaultID, v.ID())
			for doc, name := range adHocNames {
				got, ok := v.Get(doc, "tu1", "rdx1")
				require.True(t, ok, doc)
				assert.Equal(t, name, got.Original, doc)
			}

			returnTranslation(t, work)
			assertPackageWithholds(t, work, adHocNames)
			merged := filepath.Join(dir, "translated")
			require.NoError(t, a.MergeFromKpz(t.Context(), work, merged))
			for base, name := range adHocNames {
				data, err := os.ReadFile(filepath.Join(merged, base))
				require.NoError(t, err, base)
				assert.Equal(t, `{"greeting":"Bonjour `+name+`"}`, strings.TrimSpace(string(data)), base)
			}
		})
	}
}

// A pack keeps the record: the package a transform packs back is still
// marked as redacted, so its merge still needs the vault.
func TestPackKeepsTheRedactionRecord(t *testing.T) {
	a, work, _ := adHocRedactedPackage(t, true)
	before, err := LoadWorkspace(work)
	require.NoError(t, err)
	returnTranslation(t, work)
	require.NoError(t, runToolOnKpz(a, work, "case-transform", map[string]any{"mode": "upper"}))
	require.NoError(t, a.PackKpz(t.Context(), work))
	after, err := LoadWorkspace(work)
	require.NoError(t, err)
	require.NotNil(t, after.Redaction)
	assert.Equal(t, before.Redaction.VaultID, after.Redaction.VaultID)
}

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
		assert.Equal(t, "merge: work.kpz is a redacted package: it holds a placeholder wherever its redaction policy withheld a value, and the vault that restores them is not at "+vault+
			", so kapi merge writes nothing. Put the vault kapi extract wrote with the package beside it under that name, or extract the package again to write a new package and vault.", err.Error())
		assert.NoDirExists(t, merged, "the merge writes no file")
	})
	t.Run("a vault an earlier kapi wrote", func(t *testing.T) {
		data, err := os.ReadFile(vault)
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.WriteFile(vault, data, 0o600) })
		require.NoError(t, os.WriteFile(vault, []byte(`{"version": 2, "values": []}`), 0o600))
		err = a.MergeFromKpz(t.Context(), work, merged)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is vault none, so kapi merge writes nothing")
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
	assert.Contains(t, err.Error(), "a vault already sits at "+work+".vault")
	after, err := os.ReadFile(work + ".vault")
	require.NoError(t, err)
	assert.Equal(t, before, after, "the earlier vault is kept as it is")
}

// A file that holds a withheld value outside the blocks kapi can withhold it
// from would put the value in the package, so extract writes none.
func TestExtractRedactedWorkspaceRefusesAValueOutsideTheBlocks(t *testing.T) {
	t.Setenv("KAPI_KPZ_CACHE", t.TempDir())
	dir := t.TempDir()
	src := filepath.Join(dir, "app.json")
	// The key is markup, which the package carries whole.
	require.NoError(t, os.WriteFile(src, []byte(`{"Alice Archer":"Hello Alice Archer"}`), 0o644))
	rules := &redaction.RulesFile{Rules: []redaction.Rule{{Term: "Alice Archer", Category: redaction.CategoryPerson}}}
	rulesPath := filepath.Join(dir, "redact.yaml")
	require.NoError(t, rules.Save(rulesPath))
	a := &App{SourceLang: "en", Quiet: true}
	a.InitRegistries()
	work := filepath.Join(dir, "work.kpz")
	err := a.ExtractRedactedToKpz(t.Context(), []string{src}, work, "fr", "", false, rulesPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "app.json holds a value the redaction policy withholds outside any block")
	assert.NoFileExists(t, work)
	assert.NoFileExists(t, work+".vault")
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

// kapi exec over a redacted package: a tool that sends blocks out runs when
// the vault that restores the package is on this machine, and reads tokens;
// it is refused when the vault is not, as it is over a package that withholds
// nothing. A tool that sends nothing out runs either way.
func TestExecOverARedactedPackage(t *testing.T) {
	a, recipe, dir := collectionPolicyProject(t)
	srcs := []string{
		filepath.Join(dir, "src", "locales", "en", "a.json"),
		filepath.Join(dir, "src", "locales", "en", "b.json"),
	}
	plain := filepath.Join(dir, "plain.kpz")
	require.NoError(t, a.ExtractToKpz(t.Context(), srcs, plain, "fr", "", true))
	work := filepath.Join(dir, "work.kpz")
	require.NoError(t, a.ExtractRedactedToKpz(t.Context(), srcs, work, "fr", "", true, filepath.Join(dir, "redact.yaml")))

	t.Run("a package that withholds nothing is refused", func(t *testing.T) {
		config, _ := echoProgram(t)
		err := a.RunToolOnFiles(t.Context(), externalCommandRun(a, recipe, plain, config))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "would run over every file of "+plain+" under one policy")
	})
	t.Run("a redacted package with its vault runs on tokens", func(t *testing.T) {
		config, saw := echoProgram(t)
		require.NoError(t, a.RunToolOnFiles(t.Context(), externalCommandRun(a, recipe, work, config)))
		read, err := os.ReadFile(saw)
		require.NoError(t, err)
		assert.Contains(t, string(read), "[REDACTED:Person]", "the program reads the tokens")
		for _, name := range vaultNames {
			assert.NotContains(t, string(read), name, "the program never reads a withheld name")
		}
	})
	t.Run("a redacted package without its vault is refused", func(t *testing.T) {
		vault := work + ".vault"
		require.NoError(t, os.Rename(vault, vault+".moved"))
		t.Cleanup(func() { _ = os.Rename(vault+".moved", vault) })
		config, saw := echoProgram(t)
		err := a.RunToolOnFiles(t.Context(), externalCommandRun(a, recipe, work, config))
		require.Error(t, err)
		assert.Contains(t, err.Error(), work+" is a redacted package whose vault is not on this machine")
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
}

package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputPathFor(t *testing.T) {
	base := "/proj"
	tests := []struct {
		name      string
		sourceRel string
		target    string
		lang      string
		want      string
	}{
		{
			name:      "lang and wildcard",
			sourceRel: "input/app.json",
			target:    "output/{lang}/*",
			lang:      "fr-FR",
			want:      "/proj/output/fr-FR/app.json",
		},
		{
			name:      "lang only, no wildcard",
			sourceRel: "input/app.json",
			target:    "output/{lang}.json",
			lang:      "de-DE",
			want:      "/proj/output/de-DE.json",
		},
		{
			name:      "wildcard only",
			sourceRel: "src/a.txt",
			target:    "dist/*",
			lang:      "ja-JP",
			want:      "/proj/dist/a.txt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := outputPathFor(base, tt.sourceRel, tt.target, tt.lang)
			assert.Equal(t, filepath.FromSlash(tt.want), got)
		})
	}
}

func TestListOutputs(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "input"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "input", "en.json"), []byte(`{"a":"b"}`), 0o644))
	// One target already generated, one not yet.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "output", "fr-FR"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "output", "fr-FR", "en.json"), []byte(`{"a":"x"}`), 0o644))

	kapiPath := filepath.Join(dir, "test.kapi")
	proj := &project.KapiProject{
		Version: "v1",
		Name:    "Test",
		Defaults: project.Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"fr-FR", "de-DE"},
		},
		Content: []project.ContentCollection{
			{Path: "input/*.json", Target: "output/{lang}/*", Format: &project.FormatSpec{Name: "json"}},
		},
	}
	require.NoError(t, project.Save(kapiPath, proj))

	app := NewApp()
	tab := openTestProjectFile(t, app, kapiPath)

	outs, err := app.ListOutputs(tab.ID)
	require.NoError(t, err)

	// Keyed by the source file's relative path.
	list, ok := outs["input/en.json"]
	require.True(t, ok, "expected outputs keyed by source relative path, got keys %v", keysOf(outs))
	require.Len(t, list, 2)

	byLang := map[string]OutputFileInfo{}
	for _, o := range list {
		byLang[o.Lang] = o
	}

	fr := byLang["fr-FR"]
	assert.True(t, fr.Exists, "fr-FR output should exist on disk")
	assert.Equal(t, "output/fr-FR/en.json", fr.Relative)
	assert.Positive(t, fr.Size)
	assert.NotEmpty(t, fr.ModTime)

	de := byLang["de-DE"]
	assert.False(t, de.Exists, "de-DE output should not exist yet")
	assert.Equal(t, "output/de-DE/en.json", de.Relative)
}

func TestListOutputsDiscoversUndeclaredLang(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "input"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "input", "en.json"), []byte(`{"a":"b"}`), 0o644))
	// A pseudo-translate run produced output/qps/en.json even though qps is not
	// a declared target language.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "output", "qps"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "output", "qps", "en.json"), []byte(`{"a":"x"}`), 0o644))

	kapiPath := filepath.Join(dir, "test.kapi")
	proj := &project.KapiProject{
		Version:  "v1",
		Name:     "Test",
		Defaults: project.Defaults{SourceLanguage: "en-US", TargetLanguages: []model.LocaleID{"fr-FR"}},
		Content: []project.ContentCollection{
			{Path: "input/*.json", Target: "output/{lang}/*", Format: &project.FormatSpec{Name: "json"}},
		},
	}
	require.NoError(t, project.Save(kapiPath, proj))

	app := NewApp()
	tab := openTestProjectFile(t, app, kapiPath)

	outs, err := app.ListOutputs(tab.ID)
	require.NoError(t, err)

	byLang := map[string]OutputFileInfo{}
	for _, o := range outs["input/en.json"] {
		byLang[o.Lang] = o
	}
	// Declared fr-FR is shown as pending; discovered qps is shown as existing.
	require.Contains(t, byLang, "fr-FR")
	assert.False(t, byLang["fr-FR"].Exists)
	require.Contains(t, byLang, "qps", "undeclared qps output should be discovered")
	assert.True(t, byLang["qps"].Exists)
	assert.Equal(t, "output/qps/en.json", byLang["qps"].Relative)
}

func TestListOutputsNoTarget(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "input"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "input", "en.json"), []byte(`{}`), 0o644))

	kapiPath := filepath.Join(dir, "test.kapi")
	proj := &project.KapiProject{
		Version: "v1",
		Name:    "Test",
		// No target template → no outputs to resolve.
		Content: []project.ContentCollection{
			{Path: "input/*.json", Format: &project.FormatSpec{Name: "json"}},
		},
	}
	require.NoError(t, project.Save(kapiPath, proj))

	app := NewApp()
	tab := openTestProjectFile(t, app, kapiPath)

	outs, err := app.ListOutputs(tab.ID)
	require.NoError(t, err)
	assert.Empty(t, outs)
}

func TestInspectOutputRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	kapiPath := filepath.Join(dir, "test.kapi")
	proj := &project.KapiProject{Version: "v1", Name: "Test"}
	require.NoError(t, project.Save(kapiPath, proj))

	app := NewApp()
	tab := openTestProjectFile(t, app, kapiPath)

	_, err := app.InspectOutput(tab.ID, "../escape.txt")
	require.Error(t, err)
}

func keysOf(m map[string][]OutputFileInfo) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

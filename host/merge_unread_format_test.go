package host

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/host/output"
)

// `kapi merge` writes the localized files for every declared source from the
// project store. A collection in a format no installed reader opens has nothing
// in the store and cannot be rewritten, so the merge leaves it out, names it,
// and writes the rest.

// declareLayout adds a translated okf_idml collection to the project, and when
// keepApp is false removes the readable JSON collection.
func declareLayout(t *testing.T, recipe, dir string, keepApp bool) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "pkg"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pkg", "doc.idml"), []byte("<doc>Hello</doc>\n"), 0o644))
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	if !keepApp {
		proj.Collections = nil
	}
	proj.Collections = append(proj.Collections, project.Collection{
		Name: "layout",
		Content: []project.ContentItem{{
			Path: "pkg/doc.idml", Target: "pkg/doc.{lang}.idml",
			Format: &project.FormatSpec{Name: "okf_idml"},
		}},
	})
	require.NoError(t, project.Save(recipe, proj))
}

func mergeCommand(t *testing.T, recipe string) (*EnvCommand, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	cmd := NewEnvCommand(t.Context(), "merge")
	AddProjectFlag(cmd)
	cmd.Flags().String("output-format", "json", "")
	cmd.Flags().Bool("no-memory-update", false, "")
	require.NoError(t, cmd.Flags().Set("project", recipe))
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	return cmd, &stdout, &stderr
}

func TestMergeFromProjectStoreSkipsContentWithNoReader(t *testing.T) {
	a, recipe, dir := newMultiLocaleProject(t)
	declareLayout(t, recipe, dir, true)
	translateIntoStore(t, a, recipe, dir, model.LocaleID("nb"))

	cmd, stdout, stderr := mergeCommand(t, recipe)
	require.NoError(t, a.MergeFromProjectStore(cmd), "a missing plugin leaves the rest mergeable: %s", stderr.String())

	var out output.MergeStoreOutput
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &out), stdout.String())
	assert.Positive(t, out.Written, "the readable collection was written")
	_, err := os.Stat(filepath.Join(dir, "src", "nb.json"))
	require.NoError(t, err, "the readable translation is on disk")
	requireNoReader(t, out.Warnings, "pkg/doc.idml", "okf_idml")
	assert.Contains(t, stderr.String(), `no reader for format "okf_idml"`)
}

func TestMergeFromProjectStoreOverOnlyUnreadableContentFails(t *testing.T) {
	a, recipe, dir := newMultiLocaleProject(t)
	declareLayout(t, recipe, dir, false)

	cmd, _, _ := mergeCommand(t, recipe)
	err := a.MergeFromProjectStore(cmd)
	require.Error(t, err, "a merge over nothing it can read never reports success")
	assert.Contains(t, err.Error(), "nothing was materialized")
	assert.Contains(t, err.Error(), "kapi plugins install okf_idml")
}

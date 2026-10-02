package host

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaterializeUsesInjectedBlockStore(t *testing.T) {
	a, recipe, dir := newProcessOnlyProject(t)
	a.BlocksBackend = blockstore.NewPersistentMemoryStore()
	pseudo, err := tools.NewPseudoTranslateFromConfig(map[string]any{"target_locale": "fr"}, "fr")
	require.NoError(t, err)
	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    a.FormatReg,
		SourceLocale: "en",
		Store:        a.BlocksBackend,
		ProjectRoot:  dir,
	})
	require.NoError(t, runner.RunFileProcessOnly(
		context.Background(), "pseudo-translate", []tool.Tool{pseudo}, filepath.Join(dir, "src", "en.json"), "fr",
	))
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	written, err := a.materializeFromProjectStore(
		context.Background(), io.Discard, proj, recipe, []model.LocaleID{"fr"}, true,
	)
	require.NoError(t, err)
	assert.Equal(t, 1, written)
	result, err := os.ReadFile(filepath.Join(dir, "src", "fr.json"))
	require.NoError(t, err)
	assert.NotContains(t, string(result), "Hello world")
	assert.Contains(t, string(result), "greeting")
}

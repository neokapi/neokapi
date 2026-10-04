package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// A check that keeps no execution record gives no analyzer its canaries: the
// outcome would be read by nobody. A check that keeps one proves each analyzer
// it runs.
func TestCollectFileDiagnostics_ProbesOnlyForARecord(t *testing.T) {
	isolateCheckExecution(t)
	seen := 0
	real := hygieneTool
	hygieneTool = func() BlockProcessor {
		lint := check.NewContentLintTool()
		inner := lint.Annotate
		lint.Annotate = func(v tool.BlockView) error {
			seen++
			return inner(v)
		}
		return lint
	}
	t.Cleanup(func() { hygieneTool = real })

	blocks := func() []*model.Block {
		return []*model.Block{model.NewBlock("p", "We we ship.")}
	}
	a := &App{SourceLang: "en"}
	diags, err := a.collectFileDiagnostics(t.Context(), blocks(), "draft.md", checkRunOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, diags, "the content is still checked")
	assert.Equal(t, 1, seen, "only the content block was checked")

	seen = 0
	execution := newCheckExecution()
	_, err = a.collectFileDiagnostics(t.Context(), blocks(), "draft.md", checkRunOptions{execution: execution})
	require.NoError(t, err)
	assert.Greater(t, seen, 1, "the canaries went through the same checker")
	require.NotEmpty(t, execution.Analyzers)
	assert.Equal(t, "hygiene", execution.Analyzers[0].ID)
	require.NotNil(t, execution.Analyzers[0].Canary)
	assert.Equal(t, check.CanaryCaught, execution.Analyzers[0].Canary.Status)
}

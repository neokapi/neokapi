package mcp

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestContentStore(t *testing.T) store.ContentStore {
	t.Helper()
	cs, err := bstore.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { cs.Close() })
	return cs
}

func newTestMCPServerWithContent(t *testing.T) *MCPServer {
	t.Helper()
	bs := &memBrandStore{}
	cs := newTestContentStore(t)
	ms, err := NewMCPServerWithStore(bs, cs, Config{})
	require.NoError(t, err)
	return ms
}

func TestHandleCreateProject(t *testing.T) {
	ms := newTestMCPServerWithContent(t)
	ctx := t.Context()

	_, out, err := ms.handleCreateProject(ctx, nil, createProjectInput{
		Name:            "My Project",
		SourceLanguage:  "en-US",
		TargetLanguages: []string{"fr-FR", "de-DE"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, out.ID)
	assert.Equal(t, "My Project", out.Name)
}

func TestHandleCreateProject_MissingName(t *testing.T) {
	ms := newTestMCPServerWithContent(t)
	ctx := t.Context()

	_, _, err := ms.handleCreateProject(ctx, nil, createProjectInput{
		SourceLanguage: "en-US",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")
}

func TestHandleUpdateProject(t *testing.T) {
	ms := newTestMCPServerWithContent(t)
	ctx := t.Context()

	// Create first.
	p := &store.Project{
		Name:                  "Original",
		DefaultSourceLanguage: model.LocaleEnglish,
	}
	require.NoError(t, ms.contentStore.CreateProject(ctx, p))

	// Update.
	_, out, err := ms.handleUpdateProject(ctx, nil, updateProjectInput{
		ProjectID:       p.ID,
		Name:            "Updated",
		TargetLanguages: []string{"ja-JP"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Updated", out.Name)
	assert.Equal(t, []string{"ja-JP"}, out.TargetLanguages)
}

func TestHandleUpdateBlock(t *testing.T) {
	ms := newTestMCPServerWithContent(t)
	ctx := t.Context()

	// Create a project and block.
	p := &store.Project{
		Name:                  "Test",
		DefaultSourceLanguage: model.LocaleEnglish,
	}
	require.NoError(t, ms.contentStore.CreateProject(ctx, p))
	b := model.NewBlock("b1", "Hello")
	require.NoError(t, ms.contentStore.StoreBlocks(ctx, p.ID, "main", []*model.Block{b}))

	// Update block's target.
	_, out, err := ms.handleUpdateBlock(ctx, nil, updateBlockInput{
		ProjectID:    p.ID,
		BlockID:      "b1",
		TargetLocale: "fr-FR",
		TargetText:   "Bonjour",
	})
	require.NoError(t, err)
	assert.True(t, out.Updated)
	assert.Equal(t, "fr-FR", out.TargetLocale)

	// Verify persistence.
	sb, err := ms.contentStore.GetBlock(ctx, p.ID, "main", "b1")
	require.NoError(t, err)
	assert.Equal(t, "Bonjour", sb.Block.TargetText(model.LocaleID("fr-FR")))
}

func TestHandleSandboxExecuteScript(t *testing.T) {
	bs := &memBrandStore{}
	_ = bs.CreateProfile(t.Context(), &corebrand.VoiceProfile{ID: "p1"})

	sandbox := &mockSandbox{}
	ms, err := NewMCPServerWithStore(bs, nil, Config{}, WithSandbox(sandbox))
	require.NoError(t, err)

	_, out, err := ms.handleExecuteScript(t.Context(), nil, executeScriptInput{
		Language: "python",
		Code:     "print('hello')",
	})
	require.NoError(t, err)
	assert.Equal(t, "mock stdout: print('hello')", out.Stdout)
	assert.Equal(t, 0, out.ExitCode)
}

func TestHandleSandboxExecuteScript_InvalidLanguage(t *testing.T) {
	bs := &memBrandStore{}
	sandbox := &mockSandbox{}
	ms, err := NewMCPServerWithStore(bs, nil, Config{}, WithSandbox(sandbox))
	require.NoError(t, err)

	_, _, err = ms.handleExecuteScript(t.Context(), nil, executeScriptInput{
		Language: "ruby",
		Code:     "puts 'hi'",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported language")
}

func TestHandleSandboxNotConfigured(t *testing.T) {
	bs := &memBrandStore{}
	ms, err := NewMCPServer(bs, Config{})
	require.NoError(t, err)

	_, _, err = ms.handleExecuteScript(t.Context(), nil, executeScriptInput{
		Language: "python",
		Code:     "print('hi')",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sandbox executor not configured")
}

// mockSandbox implements SandboxExecutor for testing.
type mockSandbox struct{}

func (m *mockSandbox) Execute(ctx context.Context, req SandboxRequest) (*SandboxResult, error) {
	return &SandboxResult{
		Stdout:   "mock stdout: " + req.Code,
		Stderr:   "",
		ExitCode: 0,
	}, nil
}

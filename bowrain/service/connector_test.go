package service

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testConnector is a minimal connector for testing the service layer.
type testConnector struct {
	id    string
	items []*connector.ContentItem
}

func (c *testConnector) ID() string                   { return c.id }
func (c *testConnector) Name() string                 { return "test" }
func (c *testConnector) Category() connector.Category { return connector.CategoryFile }
func (c *testConnector) Fetch(_ context.Context, _ connector.FetchOptions) ([]*connector.ContentItem, error) {
	return c.items, nil
}
func (c *testConnector) Publish(_ context.Context, _ []*connector.ContentItem, _ connector.PublishOptions) error {
	return nil
}
func (c *testConnector) List(_ context.Context) ([]*connector.ContentItem, error) {
	return c.items, nil
}
func (c *testConnector) Status(_ context.Context) (*connector.SyncStatus, error) {
	return &connector.SyncStatus{ConnectorID: c.id, ItemCount: len(c.items)}, nil
}
func (c *testConnector) Configure(_ map[string]string) error { return nil }
func (c *testConnector) Close() error                        { return nil }

func TestConnectorServiceAddRemove(t *testing.T) {
	s := newTestStore(t)
	reg := connector.NewRegistry()
	reg.Register("test", connector.CategoryFile, func(config map[string]string) (connector.IntegrationConnector, error) {
		return &testConnector{id: "test-1"}, nil
	})

	svc := NewConnectorService(s, reg)

	c, err := svc.AddConnector("ws-a", "test", nil)
	require.NoError(t, err)
	assert.Equal(t, "test-1", c.ID())

	active := svc.ListActive("ws-a")
	assert.Len(t, active, 1)

	got, err := svc.GetConnector("ws-a", "test-1")
	require.NoError(t, err)
	assert.Equal(t, "test-1", got.ID())

	require.NoError(t, svc.RemoveConnector("ws-a", "test-1"))
	assert.Empty(t, svc.ListActive("ws-a"))
}

// TestConnectorServiceWorkspaceScoping verifies a connector added in one
// workspace cannot be addressed from another, even with a guessed connector ID.
func TestConnectorServiceWorkspaceScoping(t *testing.T) {
	s := newTestStore(t)
	reg := connector.NewRegistry()
	reg.Register("test", connector.CategoryFile, func(config map[string]string) (connector.IntegrationConnector, error) {
		return &testConnector{id: "wp-shared-id"}, nil
	})
	svc := NewConnectorService(s, reg)

	// Workspace A adds a connector.
	c, err := svc.AddConnector("ws-a", "test", nil)
	require.NoError(t, err)
	require.Equal(t, "wp-shared-id", c.ID())

	// Workspace B cannot see, get, fetch, publish, status, or remove it — even
	// though connector IDs are deterministic and thus guessable.
	assert.Empty(t, svc.ListActive("ws-b"))

	_, err = svc.GetConnector("ws-b", "wp-shared-id")
	require.ErrorIs(t, err, ErrConnectorNotFound)

	_, err = svc.Fetch(context.Background(), "ws-b", "wp-shared-id", "proj-1", connector.FetchOptions{})
	require.ErrorIs(t, err, ErrConnectorNotFound)

	err = svc.Publish(context.Background(), "ws-b", "wp-shared-id", "proj-1", connector.PublishOptions{})
	require.ErrorIs(t, err, ErrConnectorNotFound)

	_, err = svc.ConnectorStatus(context.Background(), "ws-b", "wp-shared-id")
	require.ErrorIs(t, err, ErrConnectorNotFound)

	require.ErrorIs(t, svc.RemoveConnector("ws-b", "wp-shared-id"), ErrConnectorNotFound)

	// Workspace A still owns it.
	assert.Len(t, svc.ListActive("ws-a"), 1)
	got, err := svc.GetConnector("ws-a", "wp-shared-id")
	require.NoError(t, err)
	assert.Equal(t, "wp-shared-id", got.ID())
}

func TestConnectorServiceFetch(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	// Create a project.
	p := &store.Project{Name: "Test", DefaultSourceLanguage: model.LocaleEnglish}
	require.NoError(t, s.CreateProject(ctx, p))

	reg := connector.NewRegistry()
	reg.Register("test", connector.CategoryFile, func(config map[string]string) (connector.IntegrationConnector, error) {
		return &testConnector{
			id: "test-1",
			items: []*connector.ContentItem{{
				ID:     "item1",
				Blocks: []*model.Block{model.NewBlock("b1", "Hello from connector")},
			}},
		}, nil
	})

	svc := NewConnectorService(s, reg)
	_, err := svc.AddConnector("ws-a", "test", nil)
	require.NoError(t, err)

	items, err := svc.Fetch(ctx, "ws-a", "test-1", p.ID, connector.FetchOptions{})
	require.NoError(t, err)
	assert.Len(t, items, 1)

	// Verify blocks were stored.
	blocks, err := s.GetBlocks(ctx, store.BlockQuery{ProjectID: p.ID})
	require.NoError(t, err)
	assert.Len(t, blocks, 1)
	assert.Equal(t, "Hello from connector", blocks[0].SourceText())
}

func TestConnectorServiceStatus(t *testing.T) {
	s := newTestStore(t)
	reg := connector.NewRegistry()
	reg.Register("test", connector.CategoryFile, func(config map[string]string) (connector.IntegrationConnector, error) {
		return &testConnector{
			id:    "test-1",
			items: []*connector.ContentItem{{ID: "a"}, {ID: "b"}},
		}, nil
	})

	svc := NewConnectorService(s, reg)
	_, _ = svc.AddConnector("ws-a", "test", nil)

	status, err := svc.ConnectorStatus(t.Context(), "ws-a", "test-1")
	require.NoError(t, err)
	assert.Equal(t, 2, status.ItemCount)
}

package service

import (
	"context"
	"testing"

	"github.com/gokapi/gokapi/core/connector"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/store"
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
func (c *testConnector) Pull(_ context.Context, _ connector.PullOptions) ([]*connector.ContentItem, error) {
	return c.items, nil
}
func (c *testConnector) Push(_ context.Context, _ []*connector.ContentItem, _ connector.PushOptions) error {
	return nil
}
func (c *testConnector) List(_ context.Context) ([]*connector.ContentItem, error) {
	return c.items, nil
}
func (c *testConnector) Sync(_ context.Context) (*connector.SyncStatus, error) {
	return &connector.SyncStatus{ConnectorID: c.id, ItemCount: len(c.items)}, nil
}
func (c *testConnector) Configure(_ map[string]string) error { return nil }
func (c *testConnector) Close() error                        { return nil }

func TestConnectorServiceAddRemove(t *testing.T) {
	s := newTestStore(t)
	reg := connector.NewRegistry()
	reg.Register("test", connector.CategoryFile, func(config map[string]string) (connector.Connector, error) {
		return &testConnector{id: "test-1"}, nil
	})

	svc := NewConnectorService(s, reg)

	c, err := svc.AddConnector("test", nil)
	require.NoError(t, err)
	assert.Equal(t, "test-1", c.ID())

	active := svc.ListActive()
	assert.Len(t, active, 1)

	got, err := svc.GetConnector("test-1")
	require.NoError(t, err)
	assert.Equal(t, "test-1", got.ID())

	require.NoError(t, svc.RemoveConnector("test-1"))
	assert.Len(t, svc.ListActive(), 0)
}

func TestConnectorServicePull(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Create a project.
	p := &store.Project{Name: "Test", SourceLocale: model.LocaleEnglish}
	require.NoError(t, s.CreateProject(ctx, p))

	reg := connector.NewRegistry()
	reg.Register("test", connector.CategoryFile, func(config map[string]string) (connector.Connector, error) {
		return &testConnector{
			id: "test-1",
			items: []*connector.ContentItem{{
				ID:     "item1",
				Blocks: []*model.Block{model.NewBlock("b1", "Hello from connector")},
			}},
		}, nil
	})

	svc := NewConnectorService(s, reg)
	_, err := svc.AddConnector("test", nil)
	require.NoError(t, err)

	items, err := svc.Pull(ctx, "test-1", p.ID, connector.PullOptions{})
	require.NoError(t, err)
	assert.Len(t, items, 1)

	// Verify blocks were stored.
	blocks, err := s.GetBlocks(ctx, store.BlockQuery{ProjectID: p.ID})
	require.NoError(t, err)
	assert.Len(t, blocks, 1)
	assert.Equal(t, "Hello from connector", blocks[0].SourceText())
}

func TestConnectorServiceSyncStatus(t *testing.T) {
	s := newTestStore(t)
	reg := connector.NewRegistry()
	reg.Register("test", connector.CategoryFile, func(config map[string]string) (connector.Connector, error) {
		return &testConnector{
			id:    "test-1",
			items: []*connector.ContentItem{{ID: "a"}, {ID: "b"}},
		}, nil
	})

	svc := NewConnectorService(s, reg)
	_, _ = svc.AddConnector("test", nil)

	status, err := svc.SyncStatus(context.Background(), "test-1")
	require.NoError(t, err)
	assert.Equal(t, 2, status.ItemCount)
}

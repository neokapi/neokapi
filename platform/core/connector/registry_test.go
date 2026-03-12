package connector

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockConnector is a minimal connector for testing the registry.
type mockConnector struct {
	id       string
	name     string
	category Category
}

func (m *mockConnector) ID() string         { return m.id }
func (m *mockConnector) Name() string       { return m.name }
func (m *mockConnector) Category() Category { return m.category }
func (m *mockConnector) Fetch(_ context.Context, _ FetchOptions) ([]*ContentItem, error) {
	return []*ContentItem{{ID: "item1", Name: "test"}}, nil
}
func (m *mockConnector) Publish(_ context.Context, _ []*ContentItem, _ PublishOptions) error {
	return nil
}
func (m *mockConnector) List(_ context.Context) ([]*ContentItem, error) { return nil, nil }
func (m *mockConnector) Status(_ context.Context) (*SyncStatus, error) {
	return &SyncStatus{ConnectorID: m.id}, nil
}
func (m *mockConnector) Configure(_ map[string]string) error { return nil }
func (m *mockConnector) Close() error                        { return nil }

// Ensure mockConnector satisfies the IntegrationConnector interface at compile time.
var _ IntegrationConnector = (*mockConnector)(nil)

func TestRegistry(t *testing.T) {
	r := NewRegistry()

	factory := func(config map[string]string) (IntegrationConnector, error) {
		return &mockConnector{
			id:       config["id"],
			name:     "mock",
			category: CategoryFile,
		}, nil
	}

	t.Run("register and create", func(t *testing.T) {
		r.Register("mock", CategoryFile, factory)
		assert.True(t, r.Has("mock"))

		c, err := r.NewConnector("mock", map[string]string{"id": "c1"})
		require.NoError(t, err)
		assert.Equal(t, "c1", c.ID())
	})

	t.Run("unknown connector", func(t *testing.T) {
		_, err := r.NewConnector("nonexistent", nil)
		assert.Error(t, err)
	})

	t.Run("list", func(t *testing.T) {
		infos := r.List()
		assert.Len(t, infos, 1)
		assert.Equal(t, "mock", infos[0].Name)
		assert.Equal(t, CategoryFile, infos[0].Category)
	})

	t.Run("has", func(t *testing.T) {
		assert.True(t, r.Has("mock"))
		assert.False(t, r.Has("ghost"))
	})
}

// Suppress unused import warning for model.
var _ = model.LocaleEnglish

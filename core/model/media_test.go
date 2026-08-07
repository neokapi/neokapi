package model_test

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMediaInlineContent(t *testing.T) {
	m := &model.Media{ID: "m1", Data: []byte("inline bytes")}

	assert.True(t, m.HasContent())
	got, err := m.Bytes()
	require.NoError(t, err)
	assert.Equal(t, []byte("inline bytes"), got)

	// Materialize is a no-op for media that is already inline.
	require.NoError(t, m.Materialize())
	assert.Equal(t, []byte("inline bytes"), m.Data)
}

func TestMediaDeferredContent(t *testing.T) {
	opens := 0
	m := &model.Media{
		ID:   "m1",
		Size: 5,
		Open: func() (io.ReadCloser, error) {
			opens++
			return io.NopCloser(strings.NewReader("hello")), nil
		},
	}

	assert.True(t, m.HasContent())
	assert.Empty(t, m.Data, "a deferred slice holds no bytes until asked")

	got, err := m.Bytes()
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), got)
	assert.Equal(t, 1, opens)

	// Bytes does not cache: the caller decides how long the bytes live.
	assert.Empty(t, m.Data)
	_, err = m.Bytes()
	require.NoError(t, err)
	assert.Equal(t, 2, opens)

	// Materialize does cache, and retires the accessor.
	require.NoError(t, m.Materialize())
	assert.Equal(t, []byte("hello"), m.Data)
	assert.Nil(t, m.Open)
	_, err = m.Bytes()
	require.NoError(t, err)
	assert.Equal(t, 3, opens, "materialized media must not re-open its source")
}

func TestMediaContentStreams(t *testing.T) {
	t.Run("inline", func(t *testing.T) {
		rc, err := (&model.Media{Data: []byte("abc")}).Content()
		require.NoError(t, err)
		defer rc.Close()
		b, err := io.ReadAll(rc)
		require.NoError(t, err)
		assert.Equal(t, "abc", string(b))
	})

	t.Run("deferred", func(t *testing.T) {
		m := &model.Media{Open: func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("abc")), nil
		}}
		rc, err := m.Content()
		require.NoError(t, err)
		defer rc.Close()
		b, err := io.ReadAll(rc)
		require.NoError(t, err)
		assert.Equal(t, "abc", string(b))
	})
}

func TestMediaWithoutContent(t *testing.T) {
	// A blob-store or URI reference carries no bytes this process can read; the
	// error keeps that from being mistaken for an empty asset.
	m := &model.Media{ID: "m1", BlobKey: "deadbeef", URI: "https://cdn.example/x.png"}
	assert.False(t, m.HasContent())
	_, err := m.Bytes()
	require.Error(t, err)

	_, err = (*model.Media)(nil).Content()
	require.Error(t, err)
	assert.False(t, (*model.Media)(nil).HasContent())
	require.NoError(t, (*model.Media)(nil).Materialize())
}

func TestMediaDeferredOpenError(t *testing.T) {
	want := errors.New("entry gone")
	m := &model.Media{Open: func() (io.ReadCloser, error) { return nil, want }}

	_, err := m.Bytes()
	require.ErrorIs(t, err, want)
	require.ErrorIs(t, m.Materialize(), want)
	assert.Empty(t, m.Data, "a failed materialization must not leave partial bytes")
}

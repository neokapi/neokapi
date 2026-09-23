package workspace_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/workspace"
)

func TestNewOpIDSortsByTimeAndAfterTheNewest(t *testing.T) {
	now := time.Date(2026, time.September, 24, 12, 0, 0, 0, time.UTC)
	first := workspace.NewOpID(now, "")
	require.True(t, workspace.ValidOpID(first))
	assert.Len(t, first, workspace.OpIDLength)

	later := workspace.NewOpID(now.Add(time.Second), first)
	assert.Less(t, first, later, "a later moment sorts later")

	same := workspace.NewOpID(now, first)
	assert.Less(t, first, same, "an id minted in the same millisecond sorts after the newest")
	assert.NotEqual(t, workspace.ShortOpID(first), workspace.ShortOpID(same),
		"and differs within the short form")

	behind := workspace.NewOpID(now.Add(-time.Hour), later)
	assert.Less(t, later, behind, "a clock running behind the newest id still sorts after it")

	at, ok := workspace.OpIDTime(first)
	require.True(t, ok)
	assert.Equal(t, now, at, "the time part reads back as the moment")

	before := workspace.NewOpID(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), "")
	assert.True(t, workspace.ValidOpID(before), "a moment before the epoch still mints an id")
}

func TestResolveOpID(t *testing.T) {
	ids := []string{
		"0d3kf9ax" + "q7aaaaaaaaaaaaaa",
		"0d3kf9ax" + "q8aaaaaaaaaaaaaa",
		"0d3kf9bb" + "zzaaaaaaaaaaaaaa",
	}
	got, err := workspace.ResolveOpID(ids[2], ids)
	require.NoError(t, err)
	assert.Equal(t, ids[2], got, "a full id resolves to itself")

	got, err = workspace.ResolveOpID("#0D3KF9AXQ7", ids)
	require.NoError(t, err)
	assert.Equal(t, ids[0], got, "a prefix copied from a log line resolves, whatever its case")

	_, err = workspace.ResolveOpID("0d3kf9ax", ids)
	var ambiguous *workspace.AmbiguousOpIDError
	require.ErrorAs(t, err, &ambiguous)
	assert.Equal(t, []string{ids[0], ids[1]}, ambiguous.Candidates, "an ambiguous prefix lists every candidate")
	assert.Contains(t, err.Error(), ids[0])

	_, err = workspace.ResolveOpID("zz", ids)
	assert.True(t, errors.Is(err, workspace.ErrNoOperation))
	_, err = workspace.ResolveOpID("  ", ids)
	assert.True(t, errors.Is(err, workspace.ErrNoOperation))
}

func TestShortOpID(t *testing.T) {
	id := workspace.NewOpID(time.Now(), "")
	assert.Equal(t, id[:workspace.ShortOpIDLength], workspace.ShortOpID(id))
	assert.Equal(t, "abc", workspace.ShortOpID("abc"), "what is not an id is left as it is")
}

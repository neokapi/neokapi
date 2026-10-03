package change_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
)

// Every edition a read lists carries a status. An edition with none recorded
// is new, or untranslated when it holds the authoritative edition's text, as a
// target file the source filled does, so a reader never takes such text for
// a translation.
func TestRead_EveryEditionCarriesAStatus(t *testing.T) {
	h := newMemHome(map[string][]memBlock{"a": {
		textBlock("copied", "Harbor Help connects you", "nb", "Harbor Help connects you"),
		textBlock("translated", "Read the guide", "nb", "Les veiledningen"),
	}})
	svc := newMemService(h)
	page, err := svc.Read(context.Background(), change.ReadRequest{Doc: "a"})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 2)
	byKey := map[string]change.BlockRead{}
	for _, b := range page.Blocks {
		byKey[b.Ref.Block] = b
	}
	require.Contains(t, byKey["copied"].Editions, "nb")
	assert.Equal(t, change.StatusUntranslated, byKey["copied"].Editions["nb"].Status)
	require.Contains(t, byKey["translated"].Editions, "nb")
	assert.Equal(t, change.StatusNew, byKey["translated"].Editions["nb"].Status)
}

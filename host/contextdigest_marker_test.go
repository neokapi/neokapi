package host

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/workspace"
)

// A surface can mark a digest seen twice in one moment; both writes land and
// the latest instant wins, with no temporary file lost under the other write.
func TestMarkContextDigestSeenConcurrently(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	key := workspace.ProjectKey("prj_concurrent")
	base := time.Date(2026, 10, 16, 1, 0, 0, 0, time.UTC)

	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := range 16 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := MarkContextDigestSeen(key, base.Add(time.Duration(i)*time.Second))
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, base.Add(15*time.Second), ContextDigestMarker(key))
}

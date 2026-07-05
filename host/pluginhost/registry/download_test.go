package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadWithProgress(t *testing.T) {
	payload := make([]byte, 1<<16) // 64 KiB so io.ReadAll makes several reads
	for i := range payload {
		payload[i] = byte(i)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Real release downloads send Content-Length (a static file); set it so
		// the progress reader surfaces a real total.
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	var lastDownloaded, total int64
	calls := 0
	data, err := DownloadWithProgress(context.Background(), srv.URL, func(downloaded, tot int64) {
		calls++
		lastDownloaded = downloaded
		total = tot
	})
	require.NoError(t, err)
	assert.Equal(t, payload, data)
	assert.Positive(t, calls, "progress callback should fire as bytes arrive")
	assert.Equal(t, int64(len(payload)), lastDownloaded, "final progress equals payload size")
	assert.Equal(t, int64(len(payload)), total, "Content-Length surfaced as total")
}

package contextscan

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/safehttp"
)

// The scheme and address policy itself is tested in bowrain/safehttp, which
// owns it. What is left here is what contextscan adds on top: the https-only
// scheme choice, the body cap, the HTML content-type requirement, and the text
// extraction — exercised through the real client so the shared policy is in
// the path rather than stubbed out.

// newTestServerFetcher wires a fetcher to a TLS httptest server:
// "example.com" resolves to a public (documentation-range) address via the
// lookup seam, every vetted dial lands on the test server's listener, and
// the test server's root CA is trusted. The httptest TLS certificate is
// valid for "example.com", so full certificate verification stays on.
func newTestServerFetcher(t *testing.T, ts *httptest.Server) *fetcher {
	t.Helper()
	tsAddr := ts.Listener.Addr().String()
	transport, ok := ts.Client().Transport.(*http.Transport)
	require.True(t, ok, "httptest client transport should be *http.Transport")

	return newFetcher(
		safehttp.WithLookup(func(_ context.Context, host string) ([]netip.Addr, error) {
			if host == "example.com" {
				return []netip.Addr{netip.MustParseAddr("203.0.113.7")}, nil
			}
			return nil, fmt.Errorf("unexpected DNS lookup for %q", host)
		}),
		safehttp.WithDialer(func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, tsAddr)
		}),
		safehttp.WithTLSConfig(transport.TLSClientConfig),
	)
}

func newVoiceTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!doctype html><html><head><title>Acme Rockets</title>`+
			`<script>var secret = "SCRIPT_SECRET";</script>`+
			`<style>.hidden{color:red}</style></head>`+
			`<body><h1>Acme builds rockets</h1>`+
			`<p>We deliver payloads to orbit, on time.</p>`+
			`<noscript>NOSCRIPT_FALLBACK</noscript></body></html>`)
	})
	mux.HandleFunc("/redirect-private", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://10.0.0.5/secret", http.StatusFound)
	})
	mux.HandleFunc("/redirect-loop", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://example.com/redirect-loop", http.StatusFound)
	})
	mux.HandleFunc("/big", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<html><body><p>")
		filler := strings.Repeat("padding ", 1024) // 8 KiB per chunk
		for written := 0; written <= fetchMaxBytes; written += len(filler) {
			fmt.Fprint(w, filler)
		}
		fmt.Fprint(w, "</p></body></html>")
	})
	mux.HandleFunc("/json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"brand":"acme"}`)
	})
	mux.HandleFunc("/missing", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})
	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestFetchURLHappyPath(t *testing.T) {
	ts := newVoiceTestServer(t)
	f := newTestServerFetcher(t, ts)

	text, err := f.fetch(context.Background(), "https://example.com/")
	require.NoError(t, err)

	assert.Contains(t, text, "Acme builds rockets")
	assert.Contains(t, text, "We deliver payloads to orbit, on time.")
	assert.NotContains(t, text, "SCRIPT_SECRET", "script content must be stripped")
	assert.NotContains(t, text, "color:red", "style content must be stripped")
	assert.NotContains(t, text, "NOSCRIPT_FALLBACK", "noscript fallback must stay buried")
}

func TestFetchURLRejectsRedirectToPrivateIP(t *testing.T) {
	ts := newVoiceTestServer(t)
	f := newTestServerFetcher(t, ts)

	_, err := f.fetch(context.Background(), "https://example.com/redirect-private")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "private")
}

func TestFetchURLCapsRedirectChain(t *testing.T) {
	ts := newVoiceTestServer(t)
	f := newTestServerFetcher(t, ts)

	_, err := f.fetch(context.Background(), "https://example.com/redirect-loop")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redirects")
}

func TestFetchURLRejectsOversizedBody(t *testing.T) {
	ts := newVoiceTestServer(t)
	f := newTestServerFetcher(t, ts)

	_, err := f.fetch(context.Background(), "https://example.com/big")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")
}

func TestFetchURLRejectsNonHTMLContentType(t *testing.T) {
	ts := newVoiceTestServer(t)
	f := newTestServerFetcher(t, ts)

	_, err := f.fetch(context.Background(), "https://example.com/json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not HTML")
}

func TestFetchURLRejectsNonOKStatus(t *testing.T) {
	ts := newVoiceTestServer(t)
	f := newTestServerFetcher(t, ts)

	_, err := f.fetch(context.Background(), "https://example.com/missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status")
}

// TestFetchURLExported exercises the exported entry point: brand scanning is
// https only, and the shared address policy is wired into it — a plain-http or
// private-space source is refused before any DNS lookup or connection attempt.
func TestFetchURLExported(t *testing.T) {
	for _, tt := range []struct {
		rawURL  string
		wantErr string
	}{
		{"http://169.254.169.254/", "https only"},
		{"http://10.0.0.5", "https only"},
		{"http://127.0.0.1", "https only"},
		{"http://localhost", "https only"},
		{"https://[fd00::1]/", "private"},
		{"https://127.0.0.1/", "loopback"},
		{"https://169.254.169.254/", "link-local"},
	} {
		t.Run(tt.rawURL, func(t *testing.T) {
			text, err := FetchURL(context.Background(), tt.rawURL)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, text)
		})
	}
}

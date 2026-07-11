package brandscan

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
)

// rejectingFetcher returns a fetcher whose lookup only knows "localhost"
// (→ 127.0.0.1) and whose dialer fails the test if it is ever reached —
// every table case below must be rejected before any connection attempt.
func rejectingFetcher(t *testing.T) *fetcher {
	t.Helper()
	f := newFetcher()
	f.lookup = func(_ context.Context, host string) ([]netip.Addr, error) {
		if host == "localhost" {
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		}
		return nil, fmt.Errorf("unexpected DNS lookup for %q", host)
	}
	f.dial = func(_ context.Context, _, addr string) (net.Conn, error) {
		t.Fatalf("dial reached for %q; the URL should have been rejected earlier", addr)
		return nil, nil
	}
	return f
}

func TestFetchURLRejectsUnsafeURLs(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		{"http metadata IP", "http://169.254.169.254/", "https only"},
		{"http private IP", "http://10.0.0.5", "https only"},
		{"http loopback IP", "http://127.0.0.1", "https only"},
		{"http localhost", "http://localhost", "https only"},
		{"non-https scheme ftp", "ftp://example.com/brand", "https only"},
		{"https metadata IP", "https://169.254.169.254/", "link-local"},
		{"https private IP", "https://10.0.0.5/", "private"},
		{"https loopback IP", "https://127.0.0.1/", "loopback"},
		{"https IPv6 loopback", "https://[::1]/", "loopback"},
		{"https localhost via DNS", "https://localhost/", "loopback"},
		{"https CGNAT IP", "https://100.64.1.2/", "carrier-grade NAT"},
		{"https unique-local IPv6", "https://[fd00::1]/", "private"},
		{"https link-local IPv6", "https://[fe80::1]/", "link-local"},
		{"https IPv4-mapped private", "https://[::ffff:10.0.0.5]/", "private"},
		{"https unspecified", "https://0.0.0.0/", "unspecified"},
		{"no host", "https:///path", "no host"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := rejectingFetcher(t)
			text, err := f.fetch(context.Background(), tt.url)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Empty(t, text)
		})
	}
}

// newTestServerFetcher wires a fetcher to a TLS httptest server:
// "example.com" resolves to a public (documentation-range) address via the
// lookup seam, every vetted dial lands on the test server's listener, and
// the test server's root CA is trusted. The httptest TLS certificate is
// valid for "example.com", so full certificate verification stays on.
func newTestServerFetcher(t *testing.T, ts *httptest.Server) *fetcher {
	t.Helper()
	tsAddr := ts.Listener.Addr().String()
	f := newFetcher()
	f.lookup = func(_ context.Context, host string) ([]netip.Addr, error) {
		if host == "example.com" {
			return []netip.Addr{netip.MustParseAddr("203.0.113.7")}, nil
		}
		return nil, fmt.Errorf("unexpected DNS lookup for %q", host)
	}
	f.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, tsAddr)
	}
	transport, ok := ts.Client().Transport.(*http.Transport)
	require.True(t, ok, "httptest client transport should be *http.Transport")
	f.tlsConfig = transport.TLSClientConfig
	return f
}

func newBrandTestServer(t *testing.T) *httptest.Server {
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
	ts := newBrandTestServer(t)
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
	ts := newBrandTestServer(t)
	f := newTestServerFetcher(t, ts)

	_, err := f.fetch(context.Background(), "https://example.com/redirect-private")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "private")
}

func TestFetchURLCapsRedirectChain(t *testing.T) {
	ts := newBrandTestServer(t)
	f := newTestServerFetcher(t, ts)

	_, err := f.fetch(context.Background(), "https://example.com/redirect-loop")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redirects")
}

func TestFetchURLRejectsOversizedBody(t *testing.T) {
	ts := newBrandTestServer(t)
	f := newTestServerFetcher(t, ts)

	_, err := f.fetch(context.Background(), "https://example.com/big")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")
}

func TestFetchURLRejectsNonHTMLContentType(t *testing.T) {
	ts := newBrandTestServer(t)
	f := newTestServerFetcher(t, ts)

	_, err := f.fetch(context.Background(), "https://example.com/json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not HTML")
}

func TestFetchURLRejectsNonOKStatus(t *testing.T) {
	ts := newBrandTestServer(t)
	f := newTestServerFetcher(t, ts)

	_, err := f.fetch(context.Background(), "https://example.com/missing")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status")
}

// TestFetchURLExported exercises the exported entry point on rejections that
// fail before any DNS lookup or connection attempt.
func TestFetchURLExported(t *testing.T) {
	for _, rawURL := range []string{
		"http://169.254.169.254/",
		"http://10.0.0.5",
		"http://127.0.0.1",
		"http://localhost",
		"https://[fd00::1]/",
	} {
		t.Run(rawURL, func(t *testing.T) {
			text, err := FetchURL(context.Background(), rawURL)
			require.Error(t, err)
			assert.Empty(t, text)
		})
	}
}

// TestVetPublicHost: the exported host policy rejects forbidden IP literals
// without any DNS lookup and accepts public ones.
func TestVetPublicHost(t *testing.T) {
	for _, host := range []string{
		"127.0.0.1",
		"::1",
		"10.0.0.5",
		"192.168.1.1",
		"169.254.169.254",
		"100.64.0.1",
		"0.0.0.0",
	} {
		t.Run(host, func(t *testing.T) {
			require.Error(t, VetPublicHost(context.Background(), host))
		})
	}
	require.NoError(t, VetPublicHost(context.Background(), "203.0.113.7"))
}

func TestForbiddenAddr(t *testing.T) {
	tests := []struct {
		addr string
		want string
	}{
		{"127.0.0.1", "loopback"},
		{"::1", "loopback"},
		{"10.0.0.5", "private"},
		{"172.16.9.1", "private"},
		{"192.168.1.1", "private"},
		{"fd12:3456::1", "private"},
		{"169.254.169.254", "link-local"},
		{"fe80::1", "link-local"},
		{"100.64.0.1", "carrier-grade NAT"},
		{"100.127.255.255", "carrier-grade NAT"},
		{"0.0.0.0", "unspecified"},
		{"224.0.0.1", "link-local"},
		{"239.1.2.3", "multicast"},
		{"::ffff:192.168.0.1", "private"},
		{"8.8.8.8", ""},
		{"203.0.113.7", ""},
		{"2606:4700::6810:84e5", ""},
		{"100.128.0.1", ""}, // just past the CGNAT /10
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			got := forbiddenAddr(netip.MustParseAddr(tt.addr))
			assert.Equal(t, tt.want, got)
		})
	}
}

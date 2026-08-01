package safehttp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rejectingPolicy returns a policy whose lookup only knows "localhost"
// (→ 127.0.0.1) and whose dialer fails the test if it is ever reached — every
// table case below must be rejected before any connection attempt.
func rejectingPolicy(t *testing.T, opts ...Option) *Policy {
	t.Helper()
	base := []Option{
		WithLookup(func(_ context.Context, host string) ([]netip.Addr, error) {
			if host == "localhost" {
				return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
			}
			return nil, fmt.Errorf("unexpected DNS lookup for %q", host)
		}),
		WithDialer(func(_ context.Context, _, addr string) (net.Conn, error) {
			t.Fatalf("dial reached for %q; the URL should have been rejected earlier", addr)
			return nil, nil
		}),
	}
	return NewPolicy(append(base, opts...)...)
}

// TestCheckURLRejectsUnsafeURLs is the address policy every safe client shares:
// no scheme outside the allowlist, and no address in private, loopback,
// link-local, CGNAT, multicast, or unspecified space.
func TestCheckURLRejectsUnsafeURLs(t *testing.T) {
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
			u, err := url.Parse(tt.url)
			require.NoError(t, err)

			err = rejectingPolicy(t).CheckURL(context.Background(), u)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestCheckURLSchemesConfigurable: a policy that permits plain http (the
// remote-connector policy — self-hosted integrations are not all on TLS) still
// enforces the address half, and still rejects a scheme outside its set.
func TestCheckURLSchemesConfigurable(t *testing.T) {
	p := rejectingPolicy(t, WithSchemes("http", "https"))

	for _, tt := range []struct {
		name    string
		url     string
		wantErr string
	}{
		{"http private IP", "http://10.0.0.5/", "private"},
		{"http loopback", "http://127.0.0.1/", "loopback"},
		{"http metadata IP", "http://169.254.169.254/", "link-local"},
		{"ftp still rejected", "ftp://example.com/", "http or https only"},
		{"file still rejected", "file:///etc/passwd", "http or https only"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.url)
			require.NoError(t, err)

			err = p.CheckURL(context.Background(), u)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// TestDialVettedRejectsForbiddenAddress: the dial is the enforcement point, so
// it must reject on its own — not merely because a URL check ran first.
func TestDialVettedRejectsForbiddenAddress(t *testing.T) {
	p := rejectingPolicy(t)

	_, err := p.DialVetted(context.Background(), "tcp", "169.254.169.254:80")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "link-local")
}

// TestDialVettedRebindingIsRechecked: a name that passed the URL check but
// resolves to private space at connect time is still refused, because the dial
// re-resolves rather than trusting the earlier answer.
func TestDialVettedRebindingIsRechecked(t *testing.T) {
	var calls int
	p := NewPolicy(
		WithLookup(func(_ context.Context, _ string) ([]netip.Addr, error) {
			calls++
			if calls == 1 {
				return []netip.Addr{netip.MustParseAddr("203.0.113.7")}, nil
			}
			return []netip.Addr{netip.MustParseAddr("10.0.0.5")}, nil
		}),
		WithDialer(func(_ context.Context, _, addr string) (net.Conn, error) {
			t.Fatalf("dial reached for %q after the address flipped to private space", addr)
			return nil, nil
		}),
	)

	u, err := url.Parse("https://rebind.example/")
	require.NoError(t, err)
	require.NoError(t, p.CheckURL(context.Background(), u), "first answer is public, so the URL check passes")

	_, err = p.DialVetted(context.Background(), "tcp", "rebind.example:443")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "private")
}

// TestTransportDisablesProxy guards the property a comment alone cannot: an
// environment proxy would do the connecting, and the vetted dial would never
// run.
func TestTransportDisablesProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9")

	assert.Nil(t, NewPolicy().Transport().Proxy)
}

func TestCheckStaticURL(t *testing.T) {
	p := NewPolicy(WithSchemes("http", "https"))

	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		{"public host", "https://cms.example.com/", ""},
		{"public host plain http", "http://cms.example.com/", ""},
		{"hostname is not resolved here", "https://internal-name/", ""},
		{"loopback literal", "https://127.0.0.1/", "loopback"},
		{"private literal", "http://192.168.1.10/", "private"},
		{"metadata literal", "http://169.254.169.254/", "link-local"},
		{"IPv6 loopback literal", "http://[::1]/", "loopback"},
		{"IPv4-mapped private literal", "http://[::ffff:10.0.0.5]/", "private"},
		{"bare host, no scheme", "cms.example.com", "not allowed"},
		{"file scheme", "file:///etc/passwd", "not allowed"},
		{"no host", "https:///wp-json", "no host"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := p.CheckStaticURL(tt.url)
			if tt.wantErr == "" {
				require.NoError(t, err)
				require.NotNil(t, u)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

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
			assert.Equal(t, tt.want, ForbiddenAddr(netip.MustParseAddr(tt.addr)))
		})
	}
}

// TestClientRejectsRedirectIntoPrivateSpace drives a real client through a real
// redirect: the public origin answers 302 to an RFC 1918 address, and the hop
// must be refused rather than followed.
func TestClientRejectsRedirectIntoPrivateSpace(t *testing.T) {
	ts, p := newPolicyTestServer(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/redirect-private", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "https://10.0.0.5/secret", http.StatusFound)
		})
	})
	defer ts.Close()

	resp, err := policyGet(t, p, "https://example.com/redirect-private")
	if resp != nil {
		resp.Body.Close()
	}
	require.Error(t, err)
	assert.Contains(t, err.Error(), "private")
}

// TestClientCapsRedirectChain: a loop terminates on the hop cap rather than
// spinning.
func TestClientCapsRedirectChain(t *testing.T) {
	ts, p := newPolicyTestServer(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "https://example.com/loop", http.StatusFound)
		})
	})
	defer ts.Close()

	resp, err := policyGet(t, p, "https://example.com/loop")
	if resp != nil {
		resp.Body.Close()
	}
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redirects")
}

// TestClientHappyPath: a public host still works end to end through the vetted
// dialer, so the policy is not simply refusing everything.
func TestClientHappyPath(t *testing.T) {
	ts, p := newPolicyTestServer(t, func(mux *http.ServeMux) {
		mux.HandleFunc("/ok", func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprint(w, "reached")
		})
	})
	defer ts.Close()

	resp, err := policyGet(t, p, "https://example.com/ok")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := ReadCapped(resp.Body, 1024)
	require.NoError(t, err)
	assert.Equal(t, "reached", string(body))
}

// policyGet issues a GET through a client built from p, so the request goes
// the whole way: vetted dial, redirect policy, and all.
func policyGet(t *testing.T, p *Policy, rawURL string) (*http.Response, error) {
	t.Helper()
	client := p.Client(5 * time.Second)
	t.Cleanup(client.CloseIdleConnections)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, rawURL, nil)
	require.NoError(t, err)
	return client.Do(req)
}

func TestReadCapped(t *testing.T) {
	_, err := ReadCapped(strings.NewReader("0123456789"), 4)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds")

	data, err := ReadCapped(strings.NewReader("0123"), 4)
	require.NoError(t, err)
	assert.Equal(t, "0123", string(data))
}

// newPolicyTestServer wires a policy to a TLS httptest server: "example.com"
// resolves to a public (documentation-range) address through the lookup seam,
// every vetted dial lands on the test server's listener, and the server's root
// CA is trusted. The httptest certificate is valid for "example.com", so full
// certificate verification stays on.
func newPolicyTestServer(t *testing.T, routes func(*http.ServeMux)) (*httptest.Server, *Policy) {
	t.Helper()
	mux := http.NewServeMux()
	routes(mux)
	ts := httptest.NewTLSServer(mux)

	transport, ok := ts.Client().Transport.(*http.Transport)
	require.True(t, ok, "httptest client transport should be *http.Transport")
	tsAddr := ts.Listener.Addr().String()

	p := NewPolicy(
		WithLookup(func(_ context.Context, host string) ([]netip.Addr, error) {
			if host == "example.com" {
				return []netip.Addr{netip.MustParseAddr("203.0.113.7")}, nil
			}
			return nil, fmt.Errorf("unexpected DNS lookup for %q", host)
		}),
		WithDialer(func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, tsAddr)
		}),
		WithTLSConfig(transport.TLSClientConfig),
	)
	return ts, p
}

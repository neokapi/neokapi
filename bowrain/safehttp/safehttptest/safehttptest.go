// Package safehttptest lets a test reach a local fixture server through the
// SSRF policy in bowrain/safehttp, without relaxing any of its checks.
//
// The policy refuses loopback and private address space, which is every
// address an httptest listener can bind to. Rather than stub the policy out —
// which would leave the shipping code untested — [Serve] replaces DNS
// resolution and the final connect: the fixture is addressed by a public
// hostname that resolves into the documentation range, the policy vets that
// answer exactly as it would in production, and only the connect lands on the
// listener. Everything a test proves, it proves about the real client.
//
// This package is test support and belongs in no production binary.
package safehttptest

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/neokapi/neokapi/bowrain/safehttp"
)

// Host is the name a fixture server answers as. The httptest TLS certificate
// is issued for "example.com", so certificate verification stays on for a TLS
// fixture.
const Host = "example.com"

// PrivateHost resolves into RFC 1918 space. It is how a test reaches the case
// that matters most: an ordinary-looking name whose addresses are private,
// which only a check at connect time can catch.
const PrivateHost = "internal.example.com"

// publicAddr is the documentation-range address Host resolves to (RFC 5737).
const publicAddr = "203.0.113.7"

// privateAddr is what PrivateHost resolves to.
const privateAddr = "10.0.0.5"

// Serve points the safehttp policy at ts for the duration of t and returns the
// base URL a client should be configured with — "http://example.com:PORT", or
// https for a TLS fixture.
//
// Its effect is process-wide, so a test that calls it must not run in parallel
// with one that depends on real resolution.
func Serve(t testing.TB, ts *httptest.Server) string {
	t.Helper()

	tsAddr := ts.Listener.Addr().String()
	_, port, err := net.SplitHostPort(tsAddr)
	if err != nil {
		t.Fatalf("safehttptest: test server address %q: %v", tsAddr, err)
	}

	opts := []safehttp.Option{
		safehttp.WithLookup(func(_ context.Context, host string) ([]netip.Addr, error) {
			switch host {
			case Host:
				return []netip.Addr{netip.MustParseAddr(publicAddr)}, nil
			case PrivateHost:
				return []netip.Addr{netip.MustParseAddr(privateAddr)}, nil
			}
			return nil, fmt.Errorf("safehttptest: unexpected DNS lookup for %q", host)
		}),
		safehttp.WithDialer(func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, tsAddr)
		}),
	}

	scheme := "http"
	if ts.TLS != nil {
		scheme = "https"
		transport, ok := ts.Client().Transport.(*http.Transport)
		if !ok {
			t.Fatalf("safehttptest: httptest client transport is %T, want *http.Transport", ts.Client().Transport)
		}
		opts = append(opts, safehttp.WithTLSConfig(transport.TLSClientConfig))
	}

	t.Cleanup(safehttp.InstallTestNetwork(opts...))
	return fmt.Sprintf("%s://%s:%s", scheme, Host, port)
}

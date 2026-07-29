// Package safehttp builds HTTP clients that will not reach private address
// space.
//
// Every platform path that dials a host somebody else chose — a CMS base URL,
// a self-hosted analytics endpoint, a brand-source page — has to go through
// one of these clients. Without that, a saved integration config is a request
// forgery primitive aimed at the server's own network: link-local metadata
// endpoints, RFC 1918 neighbours, loopback services that trust whoever
// connects to them.
//
// The protection is the dialer, not the URL check. Three properties carry it:
//
//   - The host is re-resolved and re-vetted at connect time, and only an
//     address that passed is dialed. A URL-time check alone leaves the window
//     between check and connect open to a DNS answer that changes underneath
//     it.
//   - Environment proxies are disabled. A proxy does the connecting, so the
//     vetted dialer would never run and the policy would be advisory.
//   - Every redirect hop re-enters the same policy, under a hop cap, so a
//     public page cannot bounce the client inward.
//
// A [Policy] is safe for concurrent use and cheap to build; construct one per
// client rather than sharing mutable state.
package safehttp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// DefaultMaxRedirects caps the redirect chain a safe client will follow.
const DefaultMaxRedirects = 5

// LookupFunc resolves a hostname to IP addresses. It is a seam so the address
// policy can be exercised without real DNS.
type LookupFunc func(ctx context.Context, host string) ([]netip.Addr, error)

// DialFunc opens a raw connection to an already-vetted "ip:port" address. It
// is a seam so a happy path can be pointed at an httptest listener without
// real network egress.
type DialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// Policy is the scheme and address policy a safe client enforces.
type Policy struct {
	schemes      []string
	lookup       LookupFunc
	dial         DialFunc
	tlsConfig    *tls.Config
	maxRedirects int
}

// Option configures a [Policy].
type Option func(*Policy)

// WithSchemes restricts the URL schemes the policy accepts. The default is
// https only.
func WithSchemes(schemes ...string) Option {
	return func(p *Policy) { p.schemes = schemes }
}

// WithMaxRedirects caps the redirect chain length.
func WithMaxRedirects(n int) Option {
	return func(p *Policy) { p.maxRedirects = n }
}

// WithLookup replaces DNS resolution. For tests.
func WithLookup(f LookupFunc) Option {
	return func(p *Policy) { p.lookup = f }
}

// WithDialer replaces the raw dial of a vetted address. For tests.
func WithDialer(f DialFunc) Option {
	return func(p *Policy) { p.dial = f }
}

// WithTLSConfig replaces the client TLS configuration — normally system roots
// with a TLS 1.2 floor. For tests that trust an httptest root CA.
func WithTLSConfig(c *tls.Config) Option {
	return func(p *Policy) { p.tlsConfig = c }
}

// NewPolicy builds a policy: https only, [DefaultMaxRedirects] hops, real DNS,
// real dialing.
func NewPolicy(opts ...Option) *Policy {
	p := &Policy{
		schemes:      []string{"https"},
		lookup:       defaultLookup,
		dial:         (&net.Dialer{}).DialContext,
		maxRedirects: DefaultMaxRedirects,
	}
	for _, opt := range opts {
		opt(p)
	}
	for _, opt := range loadTestOptions() {
		opt(p)
	}
	return p
}

func defaultLookup(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

// Client returns an http.Client that enforces the policy: proxy-free, dialing
// only vetted addresses, re-checking every redirect hop.
//
// The transport is per-client and hand-constructed, so its IdleConnTimeout is
// zero (= keep idle connections forever). A caller that builds a client per
// call must CloseIdleConnections when it is done, or leak the keep-alive
// connection, its readLoop goroutine, and an fd on every call.
func (p *Policy) Client(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		Transport:     p.Transport(),
		CheckRedirect: p.CheckRedirect,
	}
}

// Transport returns the policy's http.Transport, for a caller that needs to
// layer its own round tripper (retry, instrumentation) over the vetted dial.
// Such a caller must also set [Policy.CheckRedirect] on its client: redirects
// are followed above the transport, so the transport alone does not see them.
func (p *Policy) Transport() *http.Transport {
	tlsConfig := p.tlsConfig
	if tlsConfig == nil {
		tlsConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return &http.Transport{
		// Never through an environment proxy: the proxy would do the
		// connecting, and the vetted dial below would never run.
		Proxy:           nil,
		DialContext:     p.DialVetted,
		TLSClientConfig: tlsConfig,
	}
}

// CheckRedirect is an http.Client CheckRedirect that caps the chain and
// re-applies the full scheme and address policy on every hop, so a public
// page cannot redirect the client into private space.
func (p *Policy) CheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= p.maxRedirects {
		return fmt.Errorf("safehttp: stopped after %d redirects", p.maxRedirects)
	}
	return p.CheckURL(req.Context(), req.URL)
}

// CheckURL enforces the scheme and address policy for one request hop,
// resolving the host.
func (p *Policy) CheckURL(ctx context.Context, u *url.URL) error {
	if !p.allowsScheme(u.Scheme) {
		return fmt.Errorf("safehttp: URL scheme %q not allowed (%s only)", u.Scheme, joinSchemes(p.schemes))
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("safehttp: URL has no host")
	}
	_, err := p.VetHost(ctx, host)
	return err
}

// CheckStaticURL parses rawURL and enforces everything that can be decided
// without a network round trip: a permitted scheme, a present host, and — when
// the host is an IP literal — a permitted address. It is the check for
// configuration time, so a plainly unusable integration URL fails when it is
// saved rather than on first use.
//
// It is not a substitute for the dialer. A hostname's addresses are whatever
// DNS says at connect time, which is why the dial re-vets them; CheckStaticURL
// deliberately performs no lookup so that saving a config cannot be turned
// into a DNS probe.
func (p *Policy) CheckStaticURL(rawURL string) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("safehttp: invalid URL: %w", err)
	}
	if !p.allowsScheme(u.Scheme) {
		return nil, fmt.Errorf("safehttp: URL scheme %q not allowed (%s only)", u.Scheme, joinSchemes(p.schemes))
	}
	host := u.Hostname()
	if host == "" {
		return nil, errors.New("safehttp: URL has no host")
	}
	if err := CheckLiteralHost(host); err != nil {
		return nil, err
	}
	return u, nil
}

// VetHost resolves host and rejects it when ANY resolved address falls in a
// forbidden range. It returns the vetted addresses so a dialer can connect to
// exactly what was checked.
func (p *Policy) VetHost(ctx context.Context, host string) ([]netip.Addr, error) {
	// IP literal: check directly, no DNS involved. url.Hostname() and
	// net.SplitHostPort both strip IPv6 brackets already.
	if ip, err := netip.ParseAddr(host); err == nil {
		if reason := ForbiddenAddr(ip); reason != "" {
			return nil, fmt.Errorf("safehttp: host %s rejected: %s address", host, reason)
		}
		return []netip.Addr{ip}, nil
	}
	addrs, err := p.lookup(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("safehttp: resolve %s: %w", host, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("safehttp: resolve %s: no addresses", host)
	}
	for _, ip := range addrs {
		if reason := ForbiddenAddr(ip); reason != "" {
			return nil, fmt.Errorf("safehttp: host %s rejected: resolves to %s (%s address)", host, ip, reason)
		}
	}
	return addrs, nil
}

// DialVetted is the transport's DialContext: it re-resolves and re-checks the
// host at connect time (closing the rebinding window between the URL check and
// the dial) and connects only to an address that passed the check.
func (p *Policy) DialVetted(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("safehttp: dial %s: %w", addr, err)
	}
	addrs, err := p.VetHost(ctx, host)
	if err != nil {
		return nil, err
	}
	var firstErr error
	for _, ip := range addrs {
		conn, dialErr := p.dial(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		if firstErr == nil {
			firstErr = dialErr
		}
	}
	return nil, fmt.Errorf("safehttp: dial %s: %w", host, firstErr)
}

func (p *Policy) allowsScheme(scheme string) bool {
	for _, s := range p.schemes {
		if strings.EqualFold(scheme, s) {
			return true
		}
	}
	return false
}

// joinSchemes renders the allowed schemes for an error message: "https", or
// "http or https".
func joinSchemes(schemes []string) string {
	switch len(schemes) {
	case 0:
		return "none"
	case 1:
		return schemes[0]
	}
	return strings.Join(schemes[:len(schemes)-1], ", ") + " or " + schemes[len(schemes)-1]
}

// VetPublicHost resolves host against the default policy and rejects it when
// any resolved address falls in a forbidden range. It exists for paths that
// hand a host to a network client this package does not own — a git
// subprocess, say. A client that re-resolves DNS itself must treat the vet as
// a pre-check rather than a rebinding-proof pin, and should have redirect
// following disabled.
func VetPublicHost(ctx context.Context, host string) error {
	_, err := NewPolicy().VetHost(ctx, host)
	return err
}

// CheckLiteralHost rejects host when it is an IP literal in a forbidden range.
// A hostname is accepted: resolving it is the dialer's job.
func CheckLiteralHost(host string) error {
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return nil
	}
	if reason := ForbiddenAddr(ip); reason != "" {
		return fmt.Errorf("safehttp: host %s rejected: %s address", host, reason)
	}
	return nil
}

// cgnatPrefix is the carrier-grade NAT shared address space (RFC 6598).
var cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")

// ForbiddenAddr returns a non-empty reason when ip must not be dialed:
// loopback, RFC 1918 private / RFC 4193 unique-local, link-local
// (169.254.0.0/16, fe80::/10), CGNAT (100.64.0.0/10), multicast, or
// unspecified. IPv4-mapped IPv6 addresses are unmapped before checking so
// ::ffff:10.0.0.5 cannot smuggle a private IPv4 address through.
func ForbiddenAddr(ip netip.Addr) string {
	ip = ip.Unmap()
	switch {
	case !ip.IsValid():
		return "invalid"
	case ip.IsLoopback():
		return "loopback"
	case ip.IsPrivate():
		return "private"
	case ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast():
		return "link-local"
	case ip.IsUnspecified():
		return "unspecified"
	case ip.IsMulticast():
		return "multicast"
	case cgnatPrefix.Contains(ip):
		return "carrier-grade NAT"
	}
	return ""
}

// ReadCapped reads at most max bytes from r and errors when the body is larger
// than that.
func ReadCapped(r io.Reader, max int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("response body exceeds %d bytes", max)
	}
	return data, nil
}

// Drain discards up to max bytes of a response body so the connection can be
// reused, without the body reaching a caller. It is what to do with an error
// response from a host somebody else chose: reflecting that body back to the
// requester turns a blind forged request into a read of whatever it reached.
func Drain(r io.Reader, max int64) {
	_, _ = io.Copy(io.Discard, io.LimitReader(r, max))
}

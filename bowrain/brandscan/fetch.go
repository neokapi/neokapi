// Package brandscan ingests brand source material (pasted text, public web
// pages, uploaded files) into a single plain-text corpus for AI brand
// onboarding. It reuses the framework's format readers for text extraction
// and hardens the network path against SSRF for user-supplied URLs.
package brandscan

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"
)

// Limits applied by FetchURL.
const (
	// fetchTimeout bounds the whole fetch (connect, redirects, body read).
	fetchTimeout = 10 * time.Second
	// fetchMaxBytes caps the response body read from a user-supplied URL.
	fetchMaxBytes = 1 << 20 // 1 MiB
	// fetchMaxRedirects caps the redirect chain length.
	fetchMaxRedirects = 5
)

// FetchURL fetches a user-supplied HTTPS URL with SSRF protections and
// returns the page's visible text, extracted via the framework's HTML
// reader (scripts and styles never surface as text).
//
// Protections: https scheme only; the host must not resolve to a loopback,
// private (RFC 1918 / unique-local), link-local, CGNAT, multicast, or
// unspecified address; every redirect hop is re-checked; the connection
// dials only vetted addresses (DNS-rebinding safe); the body is capped at
// fetchMaxBytes and must carry an HTML content type.
func FetchURL(ctx context.Context, rawURL string) (string, error) {
	return newFetcher().fetch(ctx, rawURL)
}

// VetPublicHost resolves host and rejects it when any resolved address falls
// in a forbidden range — the same address policy FetchURL enforces (loopback,
// private, link-local, CGNAT, multicast, unspecified). It exists for other
// paths that hand a user-supplied host to a network client the fetcher does
// not own (the brand-scan repository clone). Callers whose client re-resolves
// DNS itself (a git subprocess) must treat the vet as a pre-check, not a
// rebinding-proof pin, and should disable redirect following on that client.
func VetPublicHost(ctx context.Context, host string) error {
	_, err := newFetcher().vetHost(ctx, host)
	return err
}

// lookupFunc resolves a hostname to IP addresses. Injectable for tests so
// the SSRF checks can be exercised without real DNS.
type lookupFunc func(ctx context.Context, host string) ([]netip.Addr, error)

// dialFunc opens a raw TCP connection to a vetted "ip:port" address.
// Injectable for tests so the happy path can be pointed at an
// httptest.Server without real network egress.
type dialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// fetcher is a hardened HTTPS fetcher for user-supplied URLs.
type fetcher struct {
	lookup       lookupFunc
	dial         dialFunc
	tlsConfig    *tls.Config // nil = system roots; tests inject the httptest root
	maxBody      int64
	timeout      time.Duration
	maxRedirects int
}

func newFetcher() *fetcher {
	return &fetcher{
		lookup:       defaultLookup,
		dial:         (&net.Dialer{}).DialContext,
		maxBody:      fetchMaxBytes,
		timeout:      fetchTimeout,
		maxRedirects: fetchMaxRedirects,
	}
}

func defaultLookup(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

func (f *fetcher) fetch(ctx context.Context, rawURL string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("brandscan: invalid URL: %w", err)
	}
	if err := f.checkURL(ctx, u); err != nil {
		return "", err
	}

	client := &http.Client{
		Transport: &http.Transport{
			// User-supplied URLs never go through environment proxies: a
			// proxy would bypass the vetted-dial SSRF protection below.
			Proxy:           nil,
			DialContext:     f.dialVetted,
			TLSClientConfig: f.tlsConfig,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= f.maxRedirects {
				return fmt.Errorf("brandscan: stopped after %d redirects", f.maxRedirects)
			}
			// Re-apply the full scheme + address policy on every hop so a
			// public page cannot redirect the fetcher into private space.
			return f.checkURL(req.Context(), req.URL)
		},
	}
	// The transport is per-call and hand-constructed, so its IdleConnTimeout
	// is zero (= keep idle connections forever): without this, every fetch
	// would leak its keep-alive connection, readLoop goroutine, and fd.
	defer client.CloseIdleConnections()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("brandscan: build request: %w", err)
	}
	req.Header.Set("User-Agent", "neokapi-brandscan/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("brandscan: fetch %s: %w", u.Host, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("brandscan: fetch %s: unexpected status %s", u.Host, resp.Status)
	}
	contentType := resp.Header.Get("Content-Type")
	mediaType, _, mtErr := mime.ParseMediaType(contentType)
	if mtErr != nil || !isHTMLMediaType(mediaType) {
		return "", fmt.Errorf("brandscan: fetch %s: content type %q is not HTML", u.Host, contentType)
	}

	body, err := readCapped(resp.Body, f.maxBody)
	if err != nil {
		return "", fmt.Errorf("brandscan: fetch %s: %w", u.Host, err)
	}

	text, err := htmlToText(ctx, u.String(), body)
	if err != nil {
		return "", fmt.Errorf("brandscan: extract text from %s: %w", u.Host, err)
	}
	return text, nil
}

// checkURL enforces the scheme and address policy for one request hop.
func (f *fetcher) checkURL(ctx context.Context, u *url.URL) error {
	if u.Scheme != "https" {
		return fmt.Errorf("brandscan: URL scheme %q not allowed (https only)", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("brandscan: URL has no host")
	}
	_, err := f.vetHost(ctx, host)
	return err
}

// vetHost resolves host and rejects it when ANY resolved address falls in a
// forbidden range. It returns the vetted addresses so the dialer can connect
// to exactly what was checked.
func (f *fetcher) vetHost(ctx context.Context, host string) ([]netip.Addr, error) {
	// IP literal: check directly, no DNS involved. url.Hostname() and
	// net.SplitHostPort both strip IPv6 brackets already.
	if ip, err := netip.ParseAddr(host); err == nil {
		if reason := forbiddenAddr(ip); reason != "" {
			return nil, fmt.Errorf("brandscan: host %s rejected: %s address", host, reason)
		}
		return []netip.Addr{ip}, nil
	}
	addrs, err := f.lookup(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("brandscan: resolve %s: %w", host, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("brandscan: resolve %s: no addresses", host)
	}
	for _, ip := range addrs {
		if reason := forbiddenAddr(ip); reason != "" {
			return nil, fmt.Errorf("brandscan: host %s rejected: resolves to %s (%s address)", host, ip, reason)
		}
	}
	return addrs, nil
}

// dialVetted is the transport's DialContext: it re-resolves and re-checks the
// host at connect time (closing the DNS-rebinding window between the URL
// check and the dial) and connects only to an address that passed the check.
func (f *fetcher) dialVetted(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("brandscan: dial %s: %w", addr, err)
	}
	addrs, err := f.vetHost(ctx, host)
	if err != nil {
		return nil, err
	}
	var firstErr error
	for _, ip := range addrs {
		conn, dialErr := f.dial(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		if firstErr == nil {
			firstErr = dialErr
		}
	}
	return nil, fmt.Errorf("brandscan: dial %s: %w", host, firstErr)
}

// cgnatPrefix is the carrier-grade NAT shared address space (RFC 6598).
var cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")

// forbiddenAddr returns a non-empty reason when ip must not be fetched:
// loopback, RFC 1918 private / RFC 4193 unique-local, link-local
// (169.254.0.0/16, fe80::/10), CGNAT (100.64.0.0/10), multicast, or
// unspecified. IPv4-mapped IPv6 addresses are unmapped before checking so
// ::ffff:10.0.0.5 cannot smuggle a private IPv4 address through.
func forbiddenAddr(ip netip.Addr) string {
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

// isHTMLMediaType reports whether the media type is HTML-ish.
func isHTMLMediaType(mediaType string) bool {
	return mediaType == "text/html" || mediaType == "application/xhtml+xml"
}

// readCapped reads at most max bytes from r and errors when the body is
// larger than that.
func readCapped(r io.Reader, max int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("response body exceeds %d bytes", max)
	}
	return data, nil
}

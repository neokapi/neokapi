// Package brandscan ingests brand source material (pasted text, public web
// pages, uploaded files) into a single plain-text corpus for AI brand
// onboarding. It reuses the framework's format readers for text extraction
// and hardens the network path against SSRF for user-supplied URLs.
package brandscan

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"time"

	"github.com/neokapi/neokapi/bowrain/safehttp"
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
// The network half is [safehttp]: https scheme only; the host must not resolve
// to a loopback, private (RFC 1918 / unique-local), link-local, CGNAT,
// multicast, or unspecified address; every redirect hop is re-checked; the
// connection dials only vetted addresses (rebinding safe). On top of that this
// package caps the body at fetchMaxBytes and requires an HTML content type.
func FetchURL(ctx context.Context, rawURL string) (string, error) {
	return newFetcher().fetch(ctx, rawURL)
}

// fetcher is a hardened HTTPS fetcher for user-supplied URLs: the shared
// address policy plus this package's body and content-type limits.
type fetcher struct {
	policy  *safehttp.Policy
	maxBody int64
	timeout time.Duration
}

// newFetcher builds the fetcher FetchURL uses. Extra options are the test
// seams (lookup, dial, TLS roots) — production passes none.
func newFetcher(opts ...safehttp.Option) *fetcher {
	base := []safehttp.Option{
		safehttp.WithSchemes("https"),
		safehttp.WithMaxRedirects(fetchMaxRedirects),
	}
	return &fetcher{
		policy:  safehttp.NewPolicy(append(base, opts...)...),
		maxBody: fetchMaxBytes,
		timeout: fetchTimeout,
	}
}

func (f *fetcher) fetch(ctx context.Context, rawURL string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	u, err := f.policy.CheckStaticURL(rawURL)
	if err != nil {
		return "", err
	}
	if err := f.policy.CheckURL(ctx, u); err != nil {
		return "", err
	}

	client := f.policy.Client(f.timeout)
	// The transport is per-call, so its idle connections would outlive the
	// fetch: without this, every fetch leaks its keep-alive connection, its
	// readLoop goroutine, and an fd.
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

	body, err := safehttp.ReadCapped(resp.Body, f.maxBody)
	if err != nil {
		return "", fmt.Errorf("brandscan: fetch %s: %w", u.Host, err)
	}

	text, err := htmlToText(ctx, u.String(), body)
	if err != nil {
		return "", fmt.Errorf("brandscan: extract text from %s: %w", u.Host, err)
	}
	return text, nil
}

// isHTMLMediaType reports whether the media type is HTML-ish.
func isHTMLMediaType(mediaType string) bool {
	return mediaType == "text/html" || mediaType == "application/xhtml+xml"
}

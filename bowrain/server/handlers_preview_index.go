package server

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/safehttp"
)

// The story index a collection's preview host publishes, fetched here rather
// than by the browser.
//
// The browser cannot do it. A reviewer's page is app.<domain> and the host is
// the customer's own — a published Storybook, a running site — so the fetch is
// cross-origin twice over: it needs the application's content policy to name an
// origin nobody can enumerate in advance, AND it needs that host to return
// Access-Control-Allow-Origin, which a static bucket or an internal CDN has no
// reason to send. Ours happens to; most will not, and the failure would look
// like the feature being broken rather than like a header being absent.
//
// So the server fetches it. connect-src stays 'self', CORS stops being the
// customer's problem, and the URL fetched is the one STORED ON THE COLLECTION —
// never one a client passed in, which is what keeps this from being an
// open proxy. safehttp refuses private addresses, non-https schemes and
// redirect chains, so a declared URL cannot be aimed at the metadata service
// or at something inside the VPC.

// previewIndexMaxBytes caps what a preview host can make this server hold. A
// Storybook index for a large design system is a few hundred kilobytes; this is
// generous and still bounded, because "whatever the customer serves" is not a
// size this process gets to be surprised by.
const previewIndexMaxBytes = 8 << 20 // 8 MiB

// previewIndexTimeout bounds the fetch. A reviewer is waiting behind it.
const previewIndexTimeout = 10 * time.Second

// HandlePreviewIndex proxies the collection's preview host index.
//
// GET /…/collections/:ref/:cid/preview/index
func (s *Server) HandlePreviewIndex(c echo.Context) error {
	if err := s.requirePermission(c, platauth.PermViewContent); err != nil {
		return err
	}
	if s.ContentStore == nil {
		return apiErr(c, http.StatusServiceUnavailable, "content store not configured")
	}

	projectID := c.Param("id")
	col, err := s.ContentStore.GetCollection(c.Request().Context(), projectID, c.Param("cid"))
	if err != nil || col == nil {
		return apiErr(c, http.StatusNotFound, "collection not found")
	}
	if col.PreviewKind == "" || col.PreviewURL == "" {
		// Not an error: a collection with no components behind it declares no
		// host, and the reviewer offers document reading for it.
		return apiErr(c, http.StatusNotFound, "this collection declares no preview host")
	}

	indexURL, err := previewIndexURL(col.PreviewURL)
	if err != nil {
		return apiErr(c, http.StatusBadRequest, err.Error())
	}

	body, contentType, err := s.fetchPreviewIndex(c, indexURL)
	if err != nil {
		// Returned rather than written: a 502 says the failure is on our side
		// of the wire, and whatever produced it — a DNS refusal, a TLS error,
		// the address policy — wrote its message for an operator's log, not for
		// the network. serverErrStatus logs it against the request's reference
		// and answers with the generic envelope. The reviewer is not left
		// guessing: the surface already knows which host it asked about and
		// names it.
		return serverErrStatus(c, http.StatusBadGateway,
			fmt.Errorf("read preview index %s: %w", indexURL, err))
	}

	if contentType == "" {
		contentType = echo.MIMEApplicationJSON
	}
	return c.Blob(http.StatusOK, contentType, body)
}

// previewIndexURL is the index a host publishes for the URL a recipe declared.
//
// Storybook publishes it at `index.json` under the host root, which is the one
// kind this version can resolve a view within — see host/venue/schema's
// PreviewSpec for why the kind travels at all.
func previewIndexURL(previewURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(previewURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("the declared preview host %q is not an absolute URL", previewURL)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", fmt.Errorf("the declared preview host %q must be http or https", previewURL)
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/index.json"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

// fetchPreviewIndex reads the index through the SSRF-safe client.
func (s *Server) fetchPreviewIndex(c echo.Context, indexURL string) ([]byte, string, error) {
	client := safehttp.NewPolicy().Client(previewIndexTimeout)

	req, err := http.NewRequestWithContext(c.Request().Context(), http.MethodGet, indexURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", echo.MIMEApplicationJSON)

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("host answered %d", resp.StatusCode)
	}

	// Bounded read: one more byte than the cap tells us it was exceeded rather
	// than leaving a truncated document to fail as malformed JSON later.
	body, err := io.ReadAll(io.LimitReader(resp.Body, previewIndexMaxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) > previewIndexMaxBytes {
		return nil, "", errors.New("index is larger than this server will hold")
	}
	return body, resp.Header.Get(echo.HeaderContentType), nil
}

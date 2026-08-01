package connector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupWordPressServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	posts := []wpPost{
		{
			ID:       1,
			Slug:     "hello-world",
			Title:    wpContent{Rendered: "Hello World"},
			Content:  wpContent{Rendered: "<p>Welcome to WordPress.</p>"},
			Excerpt:  wpContent{Rendered: "A welcome post"},
			Modified: "2024-01-15T10:30:00",
		},
		{
			ID:       2,
			Slug:     "sample-page",
			Title:    wpContent{Rendered: "Sample Page"},
			Content:  wpContent{Rendered: "<p>This is a sample page.</p>"},
			Modified: "2024-01-16T08:00:00",
		},
	}

	mux.HandleFunc("/wp-json/wp/v2/posts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(posts)
	})

	mux.HandleFunc("/wp-json/wp/v2/posts/1", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(posts[0])
	})

	return httptest.NewServer(mux)
}

func TestWordPressFetch(t *testing.T) {
	srv := setupWordPressServer(t)
	defer srv.Close()

	c, err := NewWordPressConnector(map[string]string{
		"url": withTestNetwork(t, srv),
	})
	require.NoError(t, err)
	assert.Equal(t, platconn.CategoryCMS, c.Category())

	items, err := c.Fetch(t.Context(), platconn.FetchOptions{})
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "Hello World", items[0].Name)
	assert.GreaterOrEqual(t, len(items[0].Blocks), 2) // title + content
	assert.Equal(t, "posts/hello-world", items[0].Path)
}

func TestWordPressPublish(t *testing.T) {
	srv := setupWordPressServer(t)
	defer srv.Close()

	c, err := NewWordPressConnector(map[string]string{
		"url": withTestNetwork(t, srv),
	})
	require.NoError(t, err)

	items, err := c.Fetch(t.Context(), platconn.FetchOptions{})
	require.NoError(t, err)

	err = c.Publish(t.Context(), items[:1], platconn.PublishOptions{})
	require.NoError(t, err)
}

func TestWordPressStatus(t *testing.T) {
	srv := setupWordPressServer(t)
	defer srv.Close()

	c, err := NewWordPressConnector(map[string]string{
		"url": withTestNetwork(t, srv),
	})
	require.NoError(t, err)

	status, err := c.Status(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 2, status.ItemCount)
}

// A WordPress site URL is tenant input that the server dials from inside its
// own network. The tests below pin the two halves of that: which addresses the
// connector will reach, and what it says when the far end answers badly.

// TestWordPressRejectsPrivateAddressURL: a config naming private address space
// is refused when the connector is built, so a bad integration fails at save
// time rather than becoming a request the server makes on someone's behalf.
func TestWordPressRejectsPrivateAddressURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		{"loopback", "http://127.0.0.1:8080", "loopback"},
		{"loopback by IPv6", "http://[::1]/", "loopback"},
		{"link-local metadata", "http://169.254.169.254/", "link-local"},
		{"RFC 1918", "http://10.0.0.5/", "private"},
		{"RFC 1918 other block", "https://192.168.1.10/", "private"},
		{"unique-local IPv6", "https://[fd00::1]/", "private"},
		{"IPv4-mapped private", "http://[::ffff:10.0.0.5]/", "private"},
		{"CGNAT", "https://100.64.1.2/", "carrier-grade NAT"},
		{"unspecified", "http://0.0.0.0/", "unspecified"},
		{"not an http scheme", "file:///etc/passwd", "not allowed"},
		{"no scheme at all", "blog.example.com", "not allowed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewWordPressConnector(map[string]string{"url": tt.url})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Nil(t, c)
		})
	}
}

// TestWordPressRejectsHostResolvingToPrivateSpace: a public-looking hostname
// whose addresses are private is stopped at the dial, where the addresses are
// finally known — a name check alone would let this through.
func TestWordPressRejectsHostResolvingToPrivateSpace(t *testing.T) {
	srv := setupWordPressServer(t)
	defer srv.Close()

	withTestNetwork(t, srv)

	c, err := NewWordPressConnector(map[string]string{
		"url": "http://" + testPrivateHost,
	})
	require.NoError(t, err, "the name looks ordinary, so building the connector succeeds")

	_, err = c.Fetch(t.Context(), platconn.FetchOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "private")
}

// TestWordPressRejectsRedirectIntoPrivateSpace: a site that answers 302 toward
// RFC 1918 must not be followed. The first hop is legitimate, so only a policy
// applied per hop catches this.
func TestWordPressRejectsRedirectIntoPrivateSpace(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/wp-json/wp/v2/posts", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://10.0.0.5/wp-json/wp/v2/posts", http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := NewWordPressConnector(map[string]string{"url": withTestNetwork(t, srv)})
	require.NoError(t, err)

	_, err = c.Fetch(t.Context(), platconn.FetchOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "private")
}

// TestWordPressErrorOmitsUpstreamBody: the operator learns the request failed
// and with what status. What answered stays where it is — echoing it back would
// make a failed fetch a way to read whatever the server could reach.
func TestWordPressErrorOmitsUpstreamBody(t *testing.T) {
	const secret = "SUPER_SECRET_INTERNAL_PAYLOAD"

	mux := http.NewServeMux()
	mux.HandleFunc("/wp-json/wp/v2/posts", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(secret))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c, err := NewWordPressConnector(map[string]string{"url": withTestNetwork(t, srv)})
	require.NoError(t, err)

	_, err = c.Fetch(t.Context(), platconn.FetchOptions{})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), secret, "the upstream body must not reach the caller")
	assert.Contains(t, err.Error(), "403", "the status is the useful part, and is kept")
}

// TestWordPressAcceptsPublicHost is the other half of the guard: the policy
// admits an ordinary public site, so it is a boundary rather than a blanket
// refusal.
func TestWordPressAcceptsPublicHost(t *testing.T) {
	for _, rawURL := range []string{
		"https://blog.example.com",
		"http://blog.example.com",
		"https://blog.example.com:8443/subdir",
		"https://203.0.113.7/",
	} {
		t.Run(rawURL, func(t *testing.T) {
			c, err := NewWordPressConnector(map[string]string{"url": rawURL})
			require.NoError(t, err)
			assert.NotNil(t, c)
		})
	}
}

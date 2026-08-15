package backend

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neokapi/neokapi/bowrain/editorclient"
	"github.com/neokapi/neokapi/host/venue/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// connectedApp builds an App in the connected state pointed at serverURL, as if
// ServerConnect had authenticated it. ProxyRequest reaches serverURL directly
// with http.DefaultClient; remoteHTTP only has to be non-nil to pass the gate.
func connectedApp(serverURL string) *App {
	a := &App{}
	a.serverURL = serverURL
	a.connState = StateConnected
	a.authInfo = &config.StoredAuth{AccessToken: "tok-123"}
	a.remoteHTTP = editorclient.New(serverURL, "tok-123")
	return a
}

func TestProxyRequestForwardsWithBearer(t *testing.T) {
	var gotAuth, gotMethod, gotPath, gotBody, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		gotMethod = r.Method
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	resp, err := connectedApp(srv.URL).ProxyRequest("POST", "/api/v1/acme/tasks", `{"title":"x"}`)
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.Status)
	assert.JSONEq(t, `{"ok":true}`, resp.Body)
	assert.Equal(t, "Bearer tok-123", gotAuth)
	assert.Equal(t, "POST", gotMethod)
	assert.Equal(t, "/api/v1/acme/tasks", gotPath)
	assert.Equal(t, `{"title":"x"}`, gotBody)
	assert.Equal(t, "application/json", gotCT)
}

func TestProxyRequestNon2xxIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer srv.Close()

	// The composite's RestApiAdapter interprets the status itself (mirroring
	// fetch), so a non-2xx is a normal ProxyResponse, not a Go error.
	resp, err := connectedApp(srv.URL).ProxyRequest("GET", "/missing", "")
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.Status)
	assert.Equal(t, "not found", resp.Body)
}

func TestProxyRequestRequiresConnection(t *testing.T) {
	_, err := (&App{}).ProxyRequest("GET", "/x", "")
	assert.ErrorIs(t, err, errNotConnected)
}

func TestProxyMultipartRebuildsForm(t *testing.T) {
	var gotAuth, gotField, gotFileName, gotFileBody, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		// t.Error, not require: the handler runs off the test goroutine, where
		// require's FailNow does not stop the test (testifylint go-require).
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotField = r.FormValue("ws")
		f, hdr, err := r.FormFile("files")
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		defer f.Close()
		gotFileName = hdr.Filename
		b, _ := io.ReadAll(f)
		gotFileBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"uploaded":1}`))
	}))
	defer srv.Close()

	// dataB64 "aGVsbG8=" decodes to "hello".
	payload := `{"fields":{"ws":"acme"},"files":[{"field":"files","filename":"a.txt","contentType":"text/plain","dataB64":"aGVsbG8="}]}`
	resp, err := connectedApp(srv.URL).ProxyMultipart("POST", "/api/v1/acme/brand/scan/uploads", payload)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.Status)
	assert.JSONEq(t, `{"uploaded":1}`, resp.Body)
	assert.Equal(t, "Bearer tok-123", gotAuth)
	assert.Contains(t, gotCT, "multipart/form-data")
	assert.Equal(t, "acme", gotField)
	assert.Equal(t, "a.txt", gotFileName)
	assert.Equal(t, "hello", gotFileBody)
}

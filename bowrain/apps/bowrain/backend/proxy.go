package backend

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// proxy.go is the desktop's generic REST transport. The frontend composite
// adapter (bowrain/apps/bowrain/frontend/src/api/desktopAdapter.ts) layers the
// shared RestApiAdapter over ProxyRequest so every server surface the desktop
// does not implement with a dedicated Wails binding still reaches the connected
// bowrain server — authenticated with the keychain token, Go-side, so the
// webview never holds the token and there is no cross-origin/CORS concern.
//
// Unlike govRequest (governance.go), which decodes into a typed out and treats
// non-2xx as an error, ProxyRequest returns the raw status + body verbatim: the
// shared RestApiAdapter interprets status codes itself (mirroring fetch), so the
// composite reuses the web adapter's error/refresh handling unchanged.

// proxyTimeout bounds each proxied REST call. Longer than governanceTimeout
// because some surfaces (dashboards, large lists) are heavier.
const proxyTimeout = 30 * time.Second

// ProxyResponse is the raw result of a ProxyRequest.
type ProxyResponse struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
}

// ProxyRequest forwards an arbitrary REST call (method + path, e.g.
// "/api/v1/acme/projects") to the connected bowrain server with the keychain
// Bearer token, returning the raw HTTP status and body. body is the request
// payload (a JSON string, or "" for none). A non-2xx status is returned in
// ProxyResponse.Status, not as an error; err is non-nil only for a missing
// connection/token or a transport failure.
func (a *App) ProxyRequest(method, path, body string) (ProxyResponse, error) {
	a.mu.RLock()
	serverURL := a.serverURL
	auth := a.authInfo
	connected := a.connState == StateConnected && a.remoteHTTP != nil
	a.mu.RUnlock()

	if !connected {
		return ProxyResponse{}, errNotConnected
	}
	if auth == nil || auth.AccessToken == "" {
		return ProxyResponse{}, errors.New("no auth token available")
	}

	var reqBody io.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	}

	ctx, cancel := context.WithTimeout(context.Background(), proxyTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, serverURL+path, reqBody)
	if err != nil {
		return ProxyResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ProxyResponse{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ProxyResponse{}, err
	}
	return ProxyResponse{Status: resp.StatusCode, Body: string(respBody)}, nil
}

// proxyMultipartFile is one file part of a ProxyMultipart upload. Data is
// base64-encoded because the Wails bridge carries JSON strings, not bytes.
type proxyMultipartFile struct {
	Field       string `json:"field"`
	Filename    string `json:"filename"`
	ContentType string `json:"contentType"`
	DataB64     string `json:"dataB64"`
}

// proxyMultipartPayload is the JSON envelope the frontend sends when a request
// body is a FormData: the string fields plus the file parts to re-assemble into
// a real multipart/form-data body server-side.
type proxyMultipartPayload struct {
	Fields map[string]string    `json:"fields"`
	Files  []proxyMultipartFile `json:"files"`
}

// ProxyMultipart is the multipart/form-data companion to ProxyRequest. The
// string-only ProxyRequest cannot carry file uploads, so surfaces that POST a
// FormData (brand-scan sources, collection uploads) route here instead: the
// frontend serializes the form into proxyMultipartPayload (files base64-encoded)
// and this rebuilds a genuine multipart body, forwarding it to the connected
// server with the keychain Bearer token. Same connection/auth contract and raw
// status+body result as ProxyRequest.
func (a *App) ProxyMultipart(method, path, payloadJSON string) (ProxyResponse, error) {
	a.mu.RLock()
	serverURL := a.serverURL
	auth := a.authInfo
	connected := a.connState == StateConnected && a.remoteHTTP != nil
	a.mu.RUnlock()

	if !connected {
		return ProxyResponse{}, errNotConnected
	}
	if auth == nil || auth.AccessToken == "" {
		return ProxyResponse{}, errors.New("no auth token available")
	}

	var payload proxyMultipartPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return ProxyResponse{}, err
	}

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range payload.Fields {
		if err := w.WriteField(k, v); err != nil {
			return ProxyResponse{}, err
		}
	}
	for _, f := range payload.Files {
		data, err := base64.StdEncoding.DecodeString(f.DataB64)
		if err != nil {
			return ProxyResponse{}, err
		}
		part, err := w.CreateFormFile(f.Field, f.Filename)
		if err != nil {
			return ProxyResponse{}, err
		}
		if _, err := part.Write(data); err != nil {
			return ProxyResponse{}, err
		}
	}
	if err := w.Close(); err != nil {
		return ProxyResponse{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), proxyTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, serverURL+path, &body)
	if err != nil {
		return ProxyResponse{}, err
	}
	req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ProxyResponse{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ProxyResponse{}, err
	}
	return ProxyResponse{Status: resp.StatusCode, Body: string(respBody)}, nil
}

package backend

// Optional, one-way "Connect / Publish to Bowrain" for the desktop app.
//
// kapi : Bowrain :: git : GitHub. The desktop app is the local, single-user
// authoring tool; this feature is the on-ramp to the platform — the
// "git remote add + push" moment — without forcing a switch to the bowrain
// clients. It is ONE-WAY (publish up), not a sync client: rich multi-user /
// governed editing stays in the bowrain apps.
//
// BOUNDARY: this file talks to a bowrain server entirely over its public REST
// API using net/http + crypto + the bowrain/plugin/schema extension types
// (the only allowed bowrain import). It does NOT import bowrain/core,
// bowrain/connector, or any bowrain server package. The OAuth flow is the
// server's own "desktop login" loopback flow: the desktop opens a browser at
// <server>/api/v1/auth/desktop/login and the server performs the full OIDC +
// PKCE dance with its identity provider, redirecting back to a localhost
// loopback URL the desktop briefly hosts. The desktop never speaks to the
// identity provider directly.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	bowrainschema "github.com/neokapi/neokapi/bowrain/plugin/schema"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/pkg/browser"
	"gopkg.in/yaml.v3"
)

// openBrowser opens a URL in the user's default browser. Indirected through a
// package var so tests can drive the loopback flow without spawning a real
// browser process.
var openBrowser = browser.OpenURL

// BowrainConnection is the frontend-facing view of a project's bowrain link.
type BowrainConnection struct {
	// Connected reports whether the recipe declares a server: block.
	Connected bool `json:"connected"`
	// ServerURL is the bare server URL (scheme + host), empty if not connected.
	ServerURL string `json:"server_url"`
	// ProjectURL is the full compound project URL from the recipe, if any.
	ProjectURL string `json:"project_url"`
	// ProjectID is the server-side project ID, if claimed/provisioned.
	ProjectID string `json:"project_id"`
	// Authenticated reports whether a valid (non-expired) token is in the keychain.
	Authenticated bool `json:"authenticated"`
	// UserEmail is the logged-in user's email, if authenticated.
	UserEmail string `json:"user_email"`
}

// GetBowrainConnection reports the bowrain connection state for a project tab.
//
// Connection has two stages:
//
//   - "Connected" — the user authenticated (token in keychain + server in auth
//     metadata) but no project exists on the server yet. The recipe has no
//     server: block (a bare server URL is not a valid server spec — the schema
//     requires a project ID).
//   - "Published" — PublishBowrain created/claimed the project and wrote a
//     valid compound URL into the recipe's server: block.
//
// The recipe's server: block lives in KapiProject.Extras (yaml-inline,
// json:"-"), so it is invisible to the frontend's GetProject. This method
// surfaces both the recipe state and the keychain auth state.
func (a *App) GetBowrainConnection(tabID string) (*BowrainConnection, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("tab %q not found", tabID)
	}
	conn := &BowrainConnection{}

	// Published state: a valid server: block in the recipe.
	spec, err := readServerSpec(op.Project)
	if err != nil {
		return nil, err
	}
	if spec != nil && spec.URL != "" {
		conn.Connected = true
		conn.ProjectURL = spec.URL
		conn.ServerURL = spec.ServerURL()
		conn.ProjectID = spec.ProjectID()
	}

	// Resolve the server to check auth against: the recipe's server (if
	// published) or the server recorded in the stored auth metadata (if
	// connected-not-yet-published).
	serverURL := conn.ServerURL
	if serverURL == "" {
		if auth, err := loadBowrainAuth(""); err == nil {
			serverURL = auth.ServerURL
		}
	}
	if serverURL != "" {
		if auth, err := loadBowrainAuth(serverURL); err == nil && auth.AccessToken != "" {
			conn.Authenticated = true
			conn.Connected = true // authenticated counts as connected
			conn.ServerURL = serverURL
			conn.UserEmail = auth.User.Email
		}
	}
	return conn, nil
}

// ConnectBowrainResult is returned from ConnectBowrain.
type ConnectBowrainResult struct {
	ServerURL string `json:"server_url"`
	UserEmail string `json:"user_email"`
}

// ConnectBowrain authenticates the user against a bowrain server via the
// browser-based desktop OAuth flow (OIDC Authorization Code + PKCE, brokered
// by the server) and stores the resulting token in the OS keychain, plus the
// chosen server in the auth metadata (shared with `kapi sync`).
//
// It does NOT yet write a server: block into the recipe — a bare server URL is
// not a valid server spec (the bowrain schema requires a project ID). The
// recipe's server: block is written by PublishBowrain once a server-side
// project exists. This keeps "authenticated" and "has a server-side project"
// as distinct, honest states.
func (a *App) ConnectBowrain(tabID, serverURL string) (*ConnectBowrainResult, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("tab %q not found", tabID)
	}
	serverURL = normalizeServerURL(serverURL)
	if serverURL == "" {
		return nil, fmt.Errorf("server URL is required")
	}
	if op.Path == "" {
		return nil, fmt.Errorf("save the project to disk before connecting to a server")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	auth, err := a.runDesktopLogin(ctx, serverURL)
	if err != nil {
		return nil, fmt.Errorf("authenticate: %w", err)
	}
	if err := saveBowrainAuth(*auth); err != nil {
		return nil, fmt.Errorf("store credentials: %w", err)
	}

	a.emitEvent("bowrain:connected", map[string]string{"server_url": serverURL, "user": auth.User.Email})
	return &ConnectBowrainResult{ServerURL: serverURL, UserEmail: auth.User.Email}, nil
}

// PublishBowrainResult is returned from PublishBowrain.
type PublishBowrainResult struct {
	ProjectID  string `json:"project_id"`
	ProjectURL string `json:"project_url"`
	// ClaimToken is returned when the project was created anonymously and is
	// not yet claimed into a workspace. Surfaced so the UI can guide the user.
	ClaimToken string `json:"claim_token,omitempty"`
	// Delegated is always true: the desktop provisions the connection +
	// initial server-side project, but ongoing content sync is delegated to
	// `kapi sync` / the bowrain apps. The UI surfaces this honestly.
	Delegated bool `json:"delegated"`
	// SyncHint is a human-readable instruction for completing the publish.
	SyncHint string `json:"sync_hint"`
}

// PublishBowrain performs the one-way "publish up" step: it creates (or
// reuses) the server-side project, claims it into the authenticated user's
// workspace, and records the compound project URL in the recipe.
//
// Honest scope: the full block-level content push is a multi-round Merkle-tree
// diff negotiation owned by the kapi-bowrain plugin. Duplicating it here would
// pull bowrain logic into the clean desktop app, so PublishBowrain
// provisions the connection (project create + claim + recipe URL) and
// delegates ongoing content sync to `kapi sync`. The result clearly tells the
// user this.
func (a *App) PublishBowrain(tabID string) (*PublishBowrainResult, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("tab %q not found", tabID)
	}
	spec, err := readServerSpec(op.Project)
	if err != nil {
		return nil, err
	}

	// Resolve the server: the recipe's (if already published) or the one
	// recorded at Connect time (in the auth metadata).
	serverURL := ""
	if spec != nil {
		serverURL = spec.ServerURL()
	}
	if serverURL == "" {
		if a, err := loadBowrainAuth(""); err == nil {
			serverURL = a.ServerURL
		}
	}
	if serverURL == "" {
		return nil, fmt.Errorf("project is not connected to a server — run Connect first")
	}

	auth, err := loadBowrainAuth(serverURL)
	if err != nil || auth.AccessToken == "" {
		return nil, fmt.Errorf("not authenticated with %s — run Connect first", serverURL)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := &bowrainRESTClient{ServerURL: serverURL, HTTPClient: http.DefaultClient}

	res := &PublishBowrainResult{Delegated: true}

	// If the recipe already names a project, reuse it; otherwise create one.
	var projectID, workspace, stream string
	if spec != nil {
		projectID = spec.ProjectID()
		workspace = spec.Workspace()
		stream = spec.Stream
	}
	if projectID == "" {
		name := projectDisplayName(op.Project, op.Path)
		created, err := client.CreateAnonymousProject(ctx, anonymousProjectRequest{
			Name:                  name,
			DefaultSourceLanguage: string(op.Project.Defaults.SourceLanguage),
			TargetLanguages:       localeStrings(op.Project.Defaults.TargetLanguages),
		})
		if err != nil {
			return nil, fmt.Errorf("create project: %w", err)
		}
		projectID = created.ProjectID
		res.ClaimToken = created.ClaimToken

		// Claim it into the authenticated user's workspace so it is owned, not
		// orphaned. A claim failure is non-fatal: the project still exists and
		// can be claimed later via the claim token.
		if claimed, err := client.ClaimProject(ctx, auth.AccessToken, created.ClaimToken); err == nil {
			workspace = claimed.WorkspaceSlug
			res.ClaimToken = "" // consumed
		}
	}

	projectURL := bowrainschema.FormatProjectURL(serverURL, workspace, projectID)
	if err := a.writeServerBlock(tabID, op, &bowrainschema.ServerSpec{URL: projectURL, Stream: stream}); err != nil {
		return nil, fmt.Errorf("update recipe: %w", err)
	}

	res.ProjectID = projectID
	res.ProjectURL = projectURL
	res.SyncHint = "Project provisioned. Push content with `kapi sync` or open it in the Bowrain apps."
	a.emitEvent("bowrain:published", map[string]string{"project_id": projectID, "project_url": projectURL})
	return res, nil
}

// DisconnectBowrain removes the server: block from the recipe and clears the
// stored credentials for that server. The server-side project (if any) is left
// untouched — disconnect is local-only.
func (a *App) DisconnectBowrain(tabID string) error {
	op := a.getOpenProject(tabID)
	if op == nil {
		return fmt.Errorf("tab %q not found", tabID)
	}
	spec, _ := readServerSpec(op.Project)
	serverURL := ""
	if spec != nil {
		serverURL = spec.ServerURL()
	}
	if serverURL == "" {
		// Connected-not-published: resolve from the stored auth metadata.
		if auth, err := loadBowrainAuth(""); err == nil {
			serverURL = auth.ServerURL
		}
	}

	a.mu.Lock()
	if op.Project.Extras != nil {
		delete(op.Project.Extras, "server")
	}
	a.mu.Unlock()

	if op.Path != "" {
		if err := project.Save(op.Path, op.Project); err != nil {
			return fmt.Errorf("save recipe: %w", err)
		}
	}
	if serverURL != "" {
		_ = deleteBowrainAuth(serverURL)
	}
	a.emitEvent("bowrain:disconnected", nil)
	return nil
}

// --- Server block helpers ---

// readServerSpec decodes the recipe's server: block from Extras. Returns nil
// (no error) when the recipe has no server block.
func readServerSpec(proj *project.KapiProject) (*bowrainschema.ServerSpec, error) {
	if proj == nil || proj.Extras == nil {
		return nil, nil
	}
	node, ok := proj.Extras["server"]
	if !ok {
		return nil, nil
	}
	var spec bowrainschema.ServerSpec
	if err := node.Decode(&spec); err != nil {
		return nil, fmt.Errorf("decode server block: %w", err)
	}
	return &spec, nil
}

// writeServerBlock encodes a ServerSpec into the recipe's Extras["server"]
// node and saves the recipe to disk. The bowrain/plugin/schema decoder
// (blank-imported by main.go) validates the block on the next load, so the
// spec must be valid (a compound URL with a project ID) before it is written —
// a bare server URL is not a valid server: block.
func (a *App) writeServerBlock(tabID string, op *openProject, spec *bowrainschema.ServerSpec) error {
	if err := spec.Validate(); err != nil {
		return err
	}
	var node yaml.Node
	if err := node.Encode(spec); err != nil {
		return fmt.Errorf("encode server block: %w", err)
	}

	a.mu.Lock()
	if op.Project.Extras == nil {
		op.Project.Extras = make(map[string]yaml.Node)
	}
	op.Project.Extras["server"] = node
	a.mu.Unlock()

	if op.Path != "" {
		if err := project.Save(op.Path, op.Project); err != nil {
			return fmt.Errorf("save recipe: %w", err)
		}
	}
	return nil
}

// --- Desktop OAuth (loopback) flow ---

// runDesktopLogin executes the server-brokered desktop OAuth flow:
//
//  1. Generate a PKCE verifier + S256 challenge.
//  2. Start a one-shot loopback HTTP server on 127.0.0.1:<random>.
//  3. Open the browser at <server>/api/v1/auth/desktop/login with the loopback
//     redirect_uri + code_challenge.
//  4. The server performs the OIDC + PKCE dance with its identity provider and
//     redirects the browser back to the loopback with token & refresh_token.
//  5. Receive the tokens on the loopback and shut it down.
func (a *App) runDesktopLogin(ctx context.Context, serverURL string) (*bowrainStoredAuth, error) {
	verifier, err := generateCodeVerifier()
	if err != nil {
		return nil, err
	}
	challenge := computeCodeChallenge(verifier)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start loopback listener: %w", err)
	}
	defer ln.Close()
	redirectURI := fmt.Sprintf("http://%s/callback", ln.Addr().String())

	type result struct {
		auth *bowrainStoredAuth
		err  error
	}
	resultCh := make(chan result, 1)
	var once sync.Once

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if errStr := q.Get("error"); errStr != "" {
			msg := q.Get("error_description")
			if msg == "" {
				msg = errStr
			}
			writeAuthResultPage(w, false, msg)
			once.Do(func() { resultCh <- result{err: fmt.Errorf("server reported: %s", msg)} })
			return
		}
		token := q.Get("token")
		if token == "" {
			writeAuthResultPage(w, false, "no token in callback")
			once.Do(func() { resultCh <- result{err: fmt.Errorf("callback missing token")} })
			return
		}
		auth := &bowrainStoredAuth{
			ServerURL:    serverURL,
			AccessToken:  token,
			RefreshToken: q.Get("refresh_token"),
			Expiry:       time.Now().Add(15 * time.Minute),
			User: bowrainStoredUser{
				Email: q.Get("user"),
				Name:  q.Get("name"),
			},
		}
		writeAuthResultPage(w, true, auth.User.Email)
		once.Do(func() { resultCh <- result{auth: auth} })
	})

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	loginURL := buildDesktopLoginURL(serverURL, redirectURI, challenge)
	if err := openBrowser(loginURL); err != nil {
		// Non-fatal: surface the URL so the user can open it manually.
		a.emitEvent("bowrain:open-url", map[string]string{"url": loginURL})
	}

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("login timed out or was cancelled: %w", ctx.Err())
	case res := <-resultCh:
		if res.err != nil {
			return nil, res.err
		}
		return res.auth, nil
	}
}

// buildDesktopLoginURL constructs the server's desktop-login authorization URL.
func buildDesktopLoginURL(serverURL, redirectURI, codeChallenge string) string {
	q := url.Values{
		"redirect_uri":          {redirectURI},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}
	return strings.TrimRight(serverURL, "/") + "/api/v1/auth/desktop/login?" + q.Encode()
}

// generateCodeVerifier creates a cryptographically random PKCE code verifier
// per RFC 7636 (43 chars of URL-safe base64).
func generateCodeVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// computeCodeChallenge computes the S256 challenge: BASE64URL(SHA256(verifier)).
func computeCodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func writeAuthResultPage(w http.ResponseWriter, ok bool, detail string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	title := "Connected to Bowrain"
	body := "Signed in as " + detail + ". You can return to Kapi."
	if !ok {
		title = "Sign-in failed"
		body = detail
	}
	_, _ = io.WriteString(w, `<!DOCTYPE html><html><head><meta charset="utf-8"><title>`+
		title+`</title></head><body style="font-family:system-ui;text-align:center;padding:60px">`+
		`<h1>`+title+`</h1><p>`+body+`</p></body></html>`)
}

// --- REST client (boundary-clean; talks to the public bowrain server API) ---

type bowrainRESTClient struct {
	ServerURL  string
	HTTPClient *http.Client
}

func (c *bowrainRESTClient) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

type anonymousProjectRequest struct {
	Name                  string   `json:"name"`
	DefaultSourceLanguage string   `json:"default_source_language"`
	TargetLanguages       []string `json:"target_languages,omitempty"`
}

type anonymousProjectResponse struct {
	ProjectID  string `json:"project_id"`
	ClaimToken string `json:"claim_token"`
}

// CreateAnonymousProject creates a server-side project with a claim token.
// No auth required (POST /api/v1/projects/anonymous).
func (c *bowrainRESTClient) CreateAnonymousProject(ctx context.Context, req anonymousProjectRequest) (*anonymousProjectResponse, error) {
	if req.Name == "" || req.DefaultSourceLanguage == "" {
		return nil, fmt.Errorf("project name and source language are required")
	}
	var out anonymousProjectResponse
	if err := c.do(ctx, http.MethodPost, "/api/v1/projects/anonymous", "", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type claimResponse struct {
	ProjectID     string `json:"project_id"`
	WorkspaceSlug string `json:"workspace_slug"`
}

// ClaimProject claims an anonymous project into the authenticated user's
// workspace (POST /api/v1/projects/claim, JWT required).
func (c *bowrainRESTClient) ClaimProject(ctx context.Context, token, claimToken string) (*claimResponse, error) {
	if claimToken == "" {
		return nil, fmt.Errorf("claim token is required")
	}
	var out claimResponse
	body := map[string]string{"claim_token": claimToken}
	if err := c.do(ctx, http.MethodPost, "/api/v1/projects/claim", token, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// do issues a JSON request and decodes the JSON response into out (if non-nil).
func (c *bowrainRESTClient) do(ctx context.Context, method, path, token string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reader = strings.NewReader(string(data))
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.ServerURL, "/")+path, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s failed (HTTP %d): %s", method, path, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// --- small helpers ---

// normalizeServerURL trims whitespace and a trailing slash and ensures a
// scheme is present (defaulting to https for bare hosts).
func normalizeServerURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.TrimRight(u.Scheme+"://"+u.Host, "/")
}

func localeStrings(locales []model.LocaleID) []string {
	out := make([]string, len(locales))
	for i, l := range locales {
		out[i] = string(l)
	}
	return out
}

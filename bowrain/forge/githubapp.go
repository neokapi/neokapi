package forge

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// GitHubApp authenticates as a registered GitHub App: a short-lived RS256 JWT
// signed with the app's private key opens the app API, and per-installation
// access tokens (minted through it, cached until shortly before expiry) do the
// actual repository work. One App instance serves every repository the app is
// installed on — connectors carry no tokens of their own in app mode.
type GitHubApp struct {
	appID         string
	key           *rsa.PrivateKey
	webhookSecret string

	http    *http.Client
	baseURL string // test override; "" = https://api.github.com

	mu     sync.Mutex
	tokens map[int64]installationToken // installation id → cached token
}

type installationToken struct {
	token     string
	expiresAt time.Time
}

// NewGitHubApp builds an App client from the app id, the PEM-encoded private
// key GitHub generated at registration, and the app's webhook secret.
func NewGitHubApp(appID, privateKeyPEM, webhookSecret string) (*GitHubApp, error) {
	if appID == "" {
		return nil, errors.New("github app: app id is required")
	}
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, errors.New("github app: private key is not PEM")
	}
	var key *rsa.PrivateKey
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		key = k
	} else if k8, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rk, ok := k8.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("github app: private key is not RSA")
		}
		key = rk
	} else {
		return nil, fmt.Errorf("github app: parse private key: %w", err)
	}
	if webhookSecret == "" {
		return nil, errors.New("github app: webhook secret is required")
	}
	return &GitHubApp{
		appID:         appID,
		key:           key,
		webhookSecret: webhookSecret,
		http:          &http.Client{Timeout: 30 * time.Second},
		tokens:        map[int64]installationToken{},
	}, nil
}

// WebhookSecret returns the secret the app-level webhook endpoint verifies
// against.
func (a *GitHubApp) WebhookSecret() string { return a.webhookSecret }

// SetAPIBase overrides the GitHub API base URL — GitHub Enterprise Server
// serves the v3 API under the instance host (https://<host>/api/v3).
func (a *GitHubApp) SetAPIBase(u string) { a.baseURL = strings.TrimSuffix(u, "/") }

func (a *GitHubApp) api() string {
	if a.baseURL != "" {
		return a.baseURL
	}
	return "https://api.github.com"
}

// appJWT mints the short-lived app JWT (RS256, ≤10 minutes per GitHub's
// contract; 60s of clock-drift backdating).
func (a *GitHubApp) appJWT() (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    a.appID,
		IssuedAt:  jwt.NewNumericDate(now.Add(-60 * time.Second)),
		ExpiresAt: jwt.NewNumericDate(now.Add(9 * time.Minute)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(a.key)
}

func (a *GitHubApp) doApp(ctx context.Context, method, u string, out any) error {
	tok, err := a.appJWT()
	if err != nil {
		return fmt.Errorf("github app jwt: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("github app %s %s: %s: %s", method, u, resp.Status, strings.TrimSpace(string(msg)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// InstallationForRepo resolves the app installation covering a repository —
// how a connector configured with only a repo path finds its installation.
func (a *GitHubApp) InstallationForRepo(ctx context.Context, repoPath string) (int64, error) {
	var out struct {
		ID int64 `json:"id"`
	}
	if err := a.doApp(ctx, http.MethodGet, fmt.Sprintf("%s/repos/%s/installation", a.api(), repoPath), &out); err != nil {
		return 0, fmt.Errorf("resolve installation for %s (is the app installed on the repository?): %w", repoPath, err)
	}
	return out.ID, nil
}

// InstallationToken returns an access token for an installation, minting a
// fresh one through the app JWT when the cached token is within five minutes
// of its one-hour expiry.
func (a *GitHubApp) InstallationToken(ctx context.Context, installationID int64) (string, error) {
	a.mu.Lock()
	if t, ok := a.tokens[installationID]; ok && time.Until(t.expiresAt) > 5*time.Minute {
		a.mu.Unlock()
		return t.token, nil
	}
	a.mu.Unlock()

	tok, err := a.appJWT()
	if err != nil {
		return "", fmt.Errorf("github app jwt: %w", err)
	}
	u := fmt.Sprintf("%s/app/installations/%d/access_tokens", a.api(), installationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := a.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("github app installation token: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	var out struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}

	a.mu.Lock()
	a.tokens[installationID] = installationToken{token: out.Token, expiresAt: out.ExpiresAt}
	a.mu.Unlock()
	return out.Token, nil
}

// InstallationRepo is one repository an app installation covers.
type InstallationRepo struct {
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
}

// ListInstallationRepos lists every repository an installation grants access
// to — what the post-install setup page offers for binding to projects.
func (a *GitHubApp) ListInstallationRepos(ctx context.Context, installationID int64) ([]InstallationRepo, error) {
	token, err := a.InstallationToken(ctx, installationID)
	if err != nil {
		return nil, err
	}
	var repos []InstallationRepo
	for page := 1; ; page++ {
		u := fmt.Sprintf("%s/installation/repositories?per_page=100&page=%d", a.api(), page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		resp, err := a.http.Do(req)
		if err != nil {
			return nil, err
		}
		var out struct {
			TotalCount   int                `json:"total_count"`
			Repositories []InstallationRepo `json:"repositories"`
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			return nil, fmt.Errorf("github installation repositories: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()
		repos = append(repos, out.Repositories...)
		if len(out.Repositories) < 100 || len(repos) >= out.TotalCount {
			break
		}
	}
	return repos, nil
}

// RepoTreePaths reads a repository's file listing (blob paths) at a ref via
// the git trees API — one recursive call, no clone. maxEntries caps how many
// blob paths are returned; the second result reports whether the listing was
// truncated (by GitHub's own recursive-tree limit or by the cap). The ref may
// be a branch name or "HEAD" for the default branch.
func (a *GitHubApp) RepoTreePaths(ctx context.Context, installationID int64, repoPath, ref string, maxEntries int) ([]string, bool, error) {
	token, err := a.InstallationToken(ctx, installationID)
	if err != nil {
		return nil, false, err
	}
	if ref == "" {
		ref = "HEAD"
	}
	u := fmt.Sprintf("%s/repos/%s/git/trees/%s?recursive=1", a.api(), repoPath, url.PathEscape(ref))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, false, fmt.Errorf("github repository tree %s@%s: %s: %s", repoPath, ref, resp.Status, strings.TrimSpace(string(msg)))
	}
	var out struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"tree"`
		Truncated bool `json:"truncated"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, false, err
	}
	truncated := out.Truncated
	paths := make([]string, 0, len(out.Tree))
	for _, e := range out.Tree {
		if e.Type != "blob" {
			continue
		}
		if maxEntries > 0 && len(paths) >= maxEntries {
			truncated = true
			break
		}
		paths = append(paths, e.Path)
	}
	return paths, truncated, nil
}

// TokenForRepo resolves the repository's installation and returns an access
// token for it — the one-call surface the forge connector uses in app mode.
func (a *GitHubApp) TokenForRepo(ctx context.Context, repoPath string) (string, error) {
	instID, err := a.InstallationForRepo(ctx, repoPath)
	if err != nil {
		return "", err
	}
	return a.InstallationToken(ctx, instID)
}

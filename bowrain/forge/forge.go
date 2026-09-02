// Package forge talks to git forges — GitHub and GitLab — for the delivery
// tier: verifying inbound push webhooks and keeping one open pull/merge
// request per project up to date with the produced translations.
//
// The clients are deliberately thin: the three calls the delivery loop needs
// (find the open PR for a head branch, create one, update its body) are
// single REST requests each, so they are made with net/http directly rather
// than a forge SDK.
package forge

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/resilience"
)

// Kind identifies a supported forge.
type Kind string

const (
	KindGitHub Kind = "github"
	KindGitLab Kind = "gitlab"
)

// KindForRepo infers the forge kind from a repository URL. github.com maps to
// GitHub; anything else defaults to GitLab, which covers gitlab.com and
// self-managed GitLab instances — the common self-hosted case. An explicit
// connector config overrides the inference.
func KindForRepo(repoURL string) Kind {
	u, err := url.Parse(repoURL)
	if err == nil && strings.EqualFold(strings.TrimPrefix(u.Hostname(), "www."), "github.com") {
		return KindGitHub
	}
	return KindGitLab
}

// Repo is the owner/name coordinate of a repository on its forge, parsed from
// the clone URL. Path holds "owner/name" (GitHub) or the full namespaced path
// (GitLab, which nests groups).
type Repo struct {
	Host string // e.g. github.com, gitlab.example.com
	Path string // e.g. acme/website or group/subgroup/website
}

// ParseRepo extracts the forge coordinate from an https clone URL.
func ParseRepo(repoURL string) (Repo, error) {
	u, err := url.Parse(repoURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		// Covers ssh:// and scp-like (git@host:path) remotes alike: the forge
		// API and token auth only exist over https.
		return Repo{}, fmt.Errorf("forge delivery needs an https repo url (got %q): the API and token auth run over https", repoURL)
	}
	path := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	if path == "" || !strings.Contains(path, "/") {
		return Repo{}, fmt.Errorf("repo url %q has no owner/name path", repoURL)
	}
	return Repo{Host: u.Hostname(), Path: path}, nil
}

// PR is the delivery pull/merge request as the forge reports it.
type PR struct {
	Number int    // PR number (GitHub) or MR IID (GitLab)
	URL    string // web URL
}

// DeliveryRequest describes the one PR/MR the delivery loop maintains.
type DeliveryRequest struct {
	Repo       Repo
	HeadBranch string // the delivery branch the server pushed
	BaseBranch string // the branch the PR merges into
	Title      string
	Body       string // replaced on every delivery so the PR carries the latest report
	Labels     []string
}

// Client is the forge-side surface the delivery loop needs.
type Client interface {
	// EnsureDeliveryPR finds the open PR/MR for the head branch and updates
	// its title/body, or creates it. It is idempotent per (repo, head branch):
	// repeated deliveries converge on one open request.
	EnsureDeliveryPR(ctx context.Context, req DeliveryRequest) (PR, error)
}

// NewClient returns the client for a forge kind. The token is an API token
// with rights to read and create pull/merge requests on the repository —
// a GitHub fine-grained PAT (contents + pull requests) or a GitLab project
// access token (api scope).
// The client is circuit-broken per forge kind, so a GitHub outage never
// short-circuits GitLab delivery (and vice versa). An injected client is
// guarded too rather than trusted: a caller supplying a transport is
// customising how we reach the forge, not opting out of the platform's
// failure policy.
func NewClient(kind Kind, token string, httpClient *http.Client) Client {
	name := "forge:gitlab"
	if kind == KindGitHub {
		name = "forge:github"
	}
	httpClient = resilience.Guard(httpClient, name, resilience.KindForge, 30*time.Second)
	switch kind {
	case KindGitHub:
		return &githubClient{token: token, http: httpClient}
	default:
		return &gitlabClient{token: token, http: httpClient}
	}
}

// ---------------------------------------------------------------------------
// GitHub

type githubClient struct {
	token   string
	http    *http.Client
	baseURL string // test override; "" = https://api.github.com
}

func (g *githubClient) api(host string) string {
	if g.baseURL != "" {
		return g.baseURL
	}
	if strings.EqualFold(host, "github.com") {
		return "https://api.github.com"
	}
	// GitHub Enterprise Server serves its v3 API under the instance host.
	return "https://" + host + "/api/v3"
}

func (g *githubClient) do(ctx context.Context, method, u string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("github %s %s: %s: %s", method, u, resp.Status, strings.TrimSpace(string(msg)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (g *githubClient) EnsureDeliveryPR(ctx context.Context, req DeliveryRequest) (PR, error) {
	api := g.api(req.Repo.Host)
	owner, _, _ := strings.Cut(req.Repo.Path, "/")

	// One open PR per head branch: GitHub filters by "owner:branch".
	var open []struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
	}
	listURL := fmt.Sprintf("%s/repos/%s/pulls?state=open&head=%s",
		api, req.Repo.Path, url.QueryEscape(owner+":"+req.HeadBranch))
	if err := g.do(ctx, http.MethodGet, listURL, nil, &open); err != nil {
		return PR{}, err
	}

	if len(open) > 0 {
		pr := open[0]
		patchURL := fmt.Sprintf("%s/repos/%s/pulls/%d", api, req.Repo.Path, pr.Number)
		if err := g.do(ctx, http.MethodPatch, patchURL, map[string]string{
			"title": req.Title, "body": req.Body,
		}, nil); err != nil {
			return PR{}, err
		}
		return PR{Number: pr.Number, URL: pr.HTMLURL}, nil
	}

	var created struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
	}
	if err := g.do(ctx, http.MethodPost, fmt.Sprintf("%s/repos/%s/pulls", api, req.Repo.Path), map[string]any{
		"title": req.Title,
		"body":  req.Body,
		"head":  req.HeadBranch,
		"base":  req.BaseBranch,
	}, &created); err != nil {
		return PR{}, err
	}
	// Labels are best-effort: a label that doesn't exist in the repo must
	// never fail a delivery.
	if len(req.Labels) > 0 {
		_ = g.do(ctx, http.MethodPost,
			fmt.Sprintf("%s/repos/%s/issues/%d/labels", api, req.Repo.Path, created.Number),
			map[string][]string{"labels": req.Labels}, nil)
	}
	return PR{Number: created.Number, URL: created.HTMLURL}, nil
}

// ---------------------------------------------------------------------------
// GitLab

type gitlabClient struct {
	token   string
	http    *http.Client
	baseURL string // test override; "" = https://<host>/api/v4
}

func (g *gitlabClient) api(host string) string {
	if g.baseURL != "" {
		return g.baseURL
	}
	return "https://" + host + "/api/v4"
}

func (g *gitlabClient) do(ctx context.Context, method, u string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("PRIVATE-TOKEN", g.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("gitlab %s %s: %s: %s", method, u, resp.Status, strings.TrimSpace(string(msg)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (g *gitlabClient) EnsureDeliveryPR(ctx context.Context, req DeliveryRequest) (PR, error) {
	api := g.api(req.Repo.Host)
	project := url.PathEscape(req.Repo.Path)

	var open []struct {
		IID    int    `json:"iid"`
		WebURL string `json:"web_url"`
	}
	listURL := fmt.Sprintf("%s/projects/%s/merge_requests?state=opened&source_branch=%s",
		api, project, url.QueryEscape(req.HeadBranch))
	if err := g.do(ctx, http.MethodGet, listURL, nil, &open); err != nil {
		return PR{}, err
	}

	if len(open) > 0 {
		mr := open[0]
		putURL := fmt.Sprintf("%s/projects/%s/merge_requests/%d", api, project, mr.IID)
		if err := g.do(ctx, http.MethodPut, putURL, map[string]string{
			"title": req.Title, "description": req.Body,
		}, nil); err != nil {
			return PR{}, err
		}
		return PR{Number: mr.IID, URL: mr.WebURL}, nil
	}

	var created struct {
		IID    int    `json:"iid"`
		WebURL string `json:"web_url"`
	}
	payload := map[string]any{
		"title":         req.Title,
		"description":   req.Body,
		"source_branch": req.HeadBranch,
		"target_branch": req.BaseBranch,
	}
	if len(req.Labels) > 0 {
		// GitLab creates unknown labels on the fly, so labels are safe inline.
		payload["labels"] = strings.Join(req.Labels, ",")
	}
	if err := g.do(ctx, http.MethodPost, fmt.Sprintf("%s/projects/%s/merge_requests", api, project), payload, &created); err != nil {
		return PR{}, err
	}
	return PR{Number: created.IID, URL: created.WebURL}, nil
}

// ---------------------------------------------------------------------------
// Inbound webhooks

// VerifyGitHubSignature checks the X-Hub-Signature-256 header — an HMAC-SHA256
// of the raw request body keyed with the webhook secret — in constant time.
func VerifyGitHubSignature(secret string, body []byte, header string) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	want, err := hex.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), want)
}

// VerifyGitLabToken checks the X-Gitlab-Token header — GitLab sends the
// secret verbatim — in constant time.
func VerifyGitLabToken(secret, header string) bool {
	return len(secret) > 0 && hmac.Equal([]byte(secret), []byte(header))
}

// PushEvent is the slice of a forge push webhook the delivery tier acts on.
type PushEvent struct {
	Branch   string // branch the commits landed on (parsed from the ref)
	RepoPath string // owner/name (GitHub full_name; GitLab path_with_namespace)
}

// ParsePushEvent decodes a push webhook payload from either forge. Non-push
// payloads (or non-branch refs, e.g. tags) return ok=false — callers ignore
// them rather than erroring, since forges send many event types to one hook.
func ParsePushEvent(kind Kind, body []byte) (PushEvent, bool) {
	const branchRefPrefix = "refs/heads/"
	switch kind {
	case KindGitHub:
		var p struct {
			Ref        string `json:"ref"`
			Repository struct {
				FullName string `json:"full_name"`
			} `json:"repository"`
		}
		if err := json.Unmarshal(body, &p); err != nil || !strings.HasPrefix(p.Ref, branchRefPrefix) {
			return PushEvent{}, false
		}
		return PushEvent{Branch: strings.TrimPrefix(p.Ref, branchRefPrefix), RepoPath: p.Repository.FullName}, true
	default:
		var p struct {
			ObjectKind string `json:"object_kind"`
			Ref        string `json:"ref"`
			Project    struct {
				PathWithNamespace string `json:"path_with_namespace"`
			} `json:"project"`
		}
		if err := json.Unmarshal(body, &p); err != nil || p.ObjectKind != "push" || !strings.HasPrefix(p.Ref, branchRefPrefix) {
			return PushEvent{}, false
		}
		return PushEvent{Branch: strings.TrimPrefix(p.Ref, branchRefPrefix), RepoPath: p.Project.PathWithNamespace}, true
	}
}

// InstallationEvent is the slice of a GitHub App installation-lifecycle webhook
// the ownership record acts on.
type InstallationEvent struct {
	Action         string // "created", "deleted", "suspend", "added", "removed", …
	InstallationID int64  // the installation the delivery is about
	Account        string // the organization or user the app is installed on
}

// ParseInstallationEvent decodes the app-level installation-lifecycle payloads
// ("installation" and "installation_repositories"), which both nest the same
// installation object. Payloads carrying no installation id return ok=false —
// callers ignore them rather than erroring, since one app hook receives every
// event type GitHub sends.
//
// The account is read for display and operator diagnosis only. Nothing here
// names a workspace, and nothing here can: the delivery is authentic but
// anonymous, so it establishes that an installation EXISTS, never whose it is.
func ParseInstallationEvent(body []byte) (InstallationEvent, bool) {
	var p struct {
		Action       string `json:"action"`
		Installation struct {
			ID      int64 `json:"id"`
			Account struct {
				Login string `json:"login"`
			} `json:"account"`
		} `json:"installation"`
	}
	if err := json.Unmarshal(body, &p); err != nil || p.Installation.ID == 0 {
		return InstallationEvent{}, false
	}
	return InstallationEvent{
		Action:         p.Action,
		InstallationID: p.Installation.ID,
		Account:        p.Installation.Account.Login,
	}, true
}

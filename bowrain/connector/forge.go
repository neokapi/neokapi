package connector

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	venueconn "github.com/neokapi/neokapi/core/venue/connector"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/bowrain/forge"
	"github.com/neokapi/neokapi/core/registry"
)

// forgeCheckoutRoot is the directory every forge connector's working checkout
// lives under. It exists as a var only so connector tests can point it at a
// t.TempDir(), in the style of allowedGitProtocols; production code never
// changes it.
var forgeCheckoutRoot = filepath.Join(os.TempDir(), "neokapi-forge")

// forgeCheckoutPath names the checkout directory for a connector, derived
// entirely from values the server can vouch for.
//
// Both inputs reach here from tenant-supplied config, so neither is joined
// into a path as-is: an id of "../.." would otherwise walk out of the root.
// The digest carries the identity — it is what makes two connectors distinct
// and one connector stable across restarts — while the slug is there only so
// an operator looking at the directory can tell which connector it belongs to.
func forgeCheckoutPath(id, repoPath string) string {
	sum := sha256.Sum256([]byte(id + "\x00" + repoPath))
	name := hex.EncodeToString(sum[:8])
	if slug := checkoutSlug(id); slug != "" {
		name = slug + "-" + name
	}
	return filepath.Join(forgeCheckoutRoot, name)
}

// checkoutSlug reduces s to a short, unmistakably safe directory-name fragment:
// ASCII alphanumerics, dash and underscore only. Anything else — separators,
// dots, spaces, non-ASCII — is dropped rather than substituted, so no input can
// produce "..", a leading dash, or a nested path.
func checkoutSlug(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		}
		if b.Len() >= 32 {
			break
		}
	}
	return strings.Trim(b.String(), "-_")
}

// DefaultDeliveryBranch is the stable head branch the forge connector pushes
// deliveries to. One branch means one open pull/merge request per project that
// every delivery updates in place — the bot pattern reviewers already know —
// instead of a new request per run.
const DefaultDeliveryBranch = "bowrain/translations"

// ForgeConnector is the delivery tier over a GitHub or GitLab repository: the
// git connector's clone/fetch for source ingestion, plus branch-and-PR
// delivery for produced translations. Publish writes the project's content
// into a delivery branch and keeps one open pull/merge request against the
// base branch, so translations always arrive as a reviewable unit — never a
// direct push to the tracked branch.
//
// Config, beyond the git connector's keys (repo, branch, patterns, …):
//
//	token           forge API token (secret) — pulls/pushes over https and
//	                opens the PR/MR. GitHub: fine-grained PAT with contents +
//	                pull-requests write. GitLab: project access token, api scope.
//	webhook_secret  secret the forge signs inbound push webhooks with (secret)
//	project_id      the Bowrain project this repository feeds
//	forge           "github" | "gitlab" (default: inferred from the repo host)
//	delivery_branch head branch for deliveries (default bowrain/translations)
//	pr_title        title for the delivery PR/MR (default "Update translations")
//	pr_labels       comma-separated labels for the delivery PR/MR
type ForgeConnector struct {
	git   *GitConnector
	kind  forge.Kind
	repo  forge.Repo
	token string           // static token; empty in app mode
	app   *forge.GitHubApp // set in app mode: tokens minted per delivery

	projectID      string
	deliveryBranch string
	prTitle        string
	prLabels       []string
	gitUserName    string
	gitUserEmail   string

	// newClient builds the forge API client for a resolved token; tests
	// inject a fake.
	newClient func(token string) forge.Client
}

// NewForgeConnector builds a forge connector from config. The repo URL must be
// https — token auth and the forge API don't exist over ssh remotes.
func NewForgeConnector(formatReg *registry.FormatRegistry, config map[string]string) (*ForgeConnector, error) {
	return NewForgeConnectorWithApp(formatReg, config, nil)
}

// NewForgeConnectorWithApp builds a forge connector that may authenticate as
// a GitHub App: with `auth: app` in the config, the connector carries no
// token of its own — a per-installation access token is minted for every
// delivery. Static-token configs behave exactly as NewForgeConnector.
func NewForgeConnectorWithApp(formatReg *registry.FormatRegistry, config map[string]string, app *forge.GitHubApp) (*ForgeConnector, error) {
	repo, err := forge.ParseRepo(config["repo"])
	if err != nil {
		return nil, err
	}
	appMode := config["auth"] == "app"
	if appMode && app == nil {
		return nil, errors.New("forge connector: auth 'app' needs the server's GitHub App configured (GITHUB_APP_ID / GITHUB_APP_PRIVATE_KEY / GITHUB_APP_WEBHOOK_SECRET)")
	}
	token := config["token"]
	if token == "" && !appMode {
		return nil, errors.New("forge connector requires 'token' config (forge API token), or auth 'app' on a server with a GitHub App")
	}
	projectID := config["project_id"]
	if projectID == "" {
		return nil, errors.New("forge connector requires 'project_id' config (the Bowrain project this repository feeds)")
	}

	// The git connector does the clone/fetch/file mechanics; default its id
	// so the two types never collide on the same repo basename.
	gitCfg := make(map[string]string, len(config))
	maps.Copy(gitCfg, config)
	if gitCfg["id"] == "" {
		gitCfg["id"] = "forge-" + repo.Path[strings.LastIndex(repo.Path, "/")+1:]
	}
	// The checkout location belongs to the server, not to the config. A forge
	// connector's config arrives over the workspace connectors API, so honoring
	// a local_path in it would let the tenant choose which directory on the
	// host the server reads through and publishes into. Overwrite it rather
	// than reject it: the key is meaningless for a forge connector either way,
	// and a delivery should not start failing over a field that never had an
	// effect worth keeping.
	gitCfg["local_path"] = forgeCheckoutPath(gitCfg["id"], repo.Path)
	git, err := NewGitConnector(formatReg, gitCfg)
	if err != nil {
		return nil, err
	}

	kind := forge.Kind(config["forge"])
	switch kind {
	case forge.KindGitHub, forge.KindGitLab:
	case "":
		kind = forge.KindForRepo(config["repo"])
	default:
		return nil, fmt.Errorf("forge connector: unknown forge %q (github or gitlab)", config["forge"])
	}
	if appMode && kind != forge.KindGitHub {
		return nil, errors.New("forge connector: auth 'app' is a GitHub App. GitLab uses a project access token")
	}

	deliveryBranch := config["delivery_branch"]
	if deliveryBranch == "" {
		deliveryBranch = DefaultDeliveryBranch
	}
	if err := validateBranch(deliveryBranch); err != nil {
		return nil, fmt.Errorf("delivery_branch: %w", err)
	}
	if deliveryBranch == git.branch {
		return nil, errors.New("delivery_branch must differ from the tracked branch: deliveries arrive as a PR, never a direct push")
	}

	prTitle := config["pr_title"]
	if prTitle == "" {
		prTitle = "Update translations"
	}
	var prLabels []string
	for l := range strings.SplitSeq(config["pr_labels"], ",") {
		if l = strings.TrimSpace(l); l != "" {
			prLabels = append(prLabels, l)
		}
	}

	gitUserName := config["git_user_name"]
	if gitUserName == "" {
		gitUserName = "Bowrain Bot"
	}
	gitUserEmail := config["git_user_email"]
	if gitUserEmail == "" {
		gitUserEmail = "bot@bowrain.cloud"
	}

	fc := &ForgeConnector{
		git:            git,
		kind:           kind,
		repo:           repo,
		token:          token,
		app:            appIfMode(appMode, app),
		projectID:      projectID,
		deliveryBranch: deliveryBranch,
		prTitle:        prTitle,
		prLabels:       prLabels,
		gitUserName:    gitUserName,
		gitUserEmail:   gitUserEmail,
	}
	fc.newClient = func(token string) forge.Client { return forge.NewClient(fc.kind, token, nil) }
	return fc, nil
}

// appIfMode keeps the app handle only when the config asked for app auth, so
// a static-token connector on an app-enabled server stays static.
func appIfMode(appMode bool, app *forge.GitHubApp) *forge.GitHubApp {
	if appMode {
		return app
	}
	return nil
}

// tokenFor resolves the credential for one delivery: the static token, or a
// freshly minted installation token in app mode.
func (c *ForgeConnector) tokenFor(ctx context.Context) (string, error) {
	if c.app != nil {
		return c.app.TokenForRepo(ctx, c.repo.Path)
	}
	return c.token, nil
}

// ProjectID returns the Bowrain project this repository feeds.
func (c *ForgeConnector) ProjectID() string { return c.projectID }

// Kind returns the forge this connector talks to.
func (c *ForgeConnector) Kind() forge.Kind { return c.kind }

// RepoPath returns the owner/name path of the repository on its forge, the
// coordinate inbound push webhooks carry.
func (c *ForgeConnector) RepoPath() string { return c.repo.Path }

// BaseBranch returns the tracked branch — the one whose pushes trigger
// convergence and the one deliveries merge into.
func (c *ForgeConnector) BaseBranch() string { return c.git.branch }

func (c *ForgeConnector) ID() string                   { return c.git.ID() }
func (c *ForgeConnector) Name() string                 { return c.git.Name() }
func (c *ForgeConnector) Category() venueconn.Category { return venueconn.CategoryCode }
func (c *ForgeConnector) Status(ctx context.Context) (*venueconn.SyncStatus, error) {
	// Status probes the remote (clone/pull + list), so it needs the same
	// per-call credential as Fetch/List — without it, app-mode status of a
	// private repository fails on every poll.
	if err := c.authGit(ctx); err != nil {
		return nil, err
	}
	return c.git.Status(ctx)
}
func (c *ForgeConnector) Configure(config map[string]string) error { return c.git.Configure(config) }
func (c *ForgeConnector) Close() error                             { return c.git.Close() }

// authGit resolves the delivery credential and injects it into the git
// connector's remote commands. Without this, app-mode fetches of private
// repositories fail — the App has no static token, so the clone must carry a
// freshly minted installation token.
func (c *ForgeConnector) authGit(ctx context.Context) error {
	token, err := c.tokenFor(ctx)
	if err != nil {
		return fmt.Errorf("resolve forge credential: %w", err)
	}
	if token == "" {
		return nil
	}
	c.git.SetAuthEnv(c.tokenAuthEnv(token))
	return nil
}

// Fetch reads source content from the tracked branch.
func (c *ForgeConnector) Fetch(ctx context.Context, opts platconn.FetchOptions) ([]*platconn.ContentItem, error) {
	if err := c.authGit(ctx); err != nil {
		return nil, err
	}
	return c.git.Fetch(ctx, opts)
}

// List lists content on the tracked branch.
func (c *ForgeConnector) List(ctx context.Context) ([]*platconn.ContentItem, error) {
	if err := c.authGit(ctx); err != nil {
		return nil, err
	}
	return c.git.List(ctx)
}

// tokenAuthEnv returns GIT_CONFIG_* environment variables that inject the
// forge token as an https Authorization header — environment, not argv, so the
// credential never shows up in a process listing. The basic-auth username is
// the forge's token convention; both forges only check the password half.
func (c *ForgeConnector) tokenAuthEnv(token string) []string {
	user := "oauth2"
	if c.kind == forge.KindGitHub {
		user = "x-access-token"
	}
	basic := base64.StdEncoding.EncodeToString([]byte(user + ":" + token))
	return []string{
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.extraHeader",
		"GIT_CONFIG_VALUE_0=Authorization: Basic " + basic,
	}
}

// deliveryGit runs a git command in the clone with token auth injected.
func (c *ForgeConnector) deliveryGit(ctx context.Context, token string, args ...string) *exec.Cmd {
	cmd := gitCommand(ctx, append(c.git.globalArgs(), args...)...)
	cmd.Env = append(cmd.Env, c.tokenAuthEnv(token)...)
	return cmd
}

// Publish delivers content as a pull/merge request: write the items into the
// delivery branch (recreated from the tracked branch every time, so a
// delivery never depends on the previous one), push it, and create or update
// the one open PR/MR for that branch. opts.Metadata may carry pr_title and
// pr_body; opts.Message is the commit message. With nothing to deliver — the
// produced files match the tracked branch — no branch is pushed and no PR is
// touched.
func (c *ForgeConnector) Publish(ctx context.Context, items []*platconn.ContentItem, opts platconn.PublishOptions) error {
	if err := c.git.validate(); err != nil {
		return err
	}
	// Authenticate the clone/pull the same way Fetch/List/Status do: in app
	// mode there is no static token, so the delivery clone in ensureRepo must
	// carry a freshly minted installation token or it fails with "could not
	// read Username". (The checkout/push below use deliveryGit's explicit
	// token; ensureRepo relies on the connector's auth env.)
	if err := c.authGit(ctx); err != nil {
		return err
	}
	// Refresh the tracked branch first: the delivery branches from its tip.
	if err := c.git.ensureRepo(ctx); err != nil {
		return err
	}
	if err := c.git.ensureFileConnector(); err != nil {
		return err
	}

	token, err := c.tokenFor(ctx)
	if err != nil {
		return err
	}

	// Recreate the delivery branch from the tracked branch tip. -B resets it
	// if a previous delivery left it behind.
	checkout := c.deliveryGit(ctx, token, "-C", c.git.localPath, "checkout", "-B", c.deliveryBranch, c.git.branch)
	if out, err := checkout.CombinedOutput(); err != nil {
		return fmt.Errorf("git checkout %s: %s: %w", c.deliveryBranch, string(out), err)
	}
	// Whatever happens next, leave the clone back on the tracked branch so a
	// later Fetch reads source, not a delivery.
	defer func() {
		back := gitCommand(context.WithoutCancel(ctx), "-C", c.git.localPath, "checkout", c.git.branch)
		_, _ = back.CombinedOutput()
	}()

	if err := c.git.fileConnector.Publish(ctx, items, opts); err != nil {
		return err
	}

	addCmd := gitCommand(ctx, "-C", c.git.localPath, "add", "-A")
	if out, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %s: %w", string(out), err)
	}

	// Nothing produced beyond what the tracked branch already has: no
	// delivery. (diff --cached exits 1 when there ARE staged changes.)
	diffCmd := gitCommand(ctx, "-C", c.git.localPath, "diff", "--cached", "--quiet")
	if err := diffCmd.Run(); err == nil {
		return nil
	}

	message := opts.Message
	if message == "" {
		message = "Update translations"
	}
	// Explicit committer identity: the server has no global gitconfig, and a
	// commit without one fails outright.
	commitCmd := gitCommand(ctx, "-C", c.git.localPath,
		"-c", "user.name="+c.gitUserName, "-c", "user.email="+c.gitUserEmail,
		"commit", "-m", message)
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %s: %w", string(out), err)
	}

	// Force-push: the branch was recreated from the tracked tip, so its
	// history is deliberately rewritten on every delivery.
	pushCmd := c.deliveryGit(ctx, token, "-C", c.git.localPath, "push", "--force", "origin", "--", c.deliveryBranch)
	if out, err := pushCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git push %s: %s: %w", c.deliveryBranch, string(out), err)
	}

	title := c.prTitle
	if t := opts.Metadata["pr_title"]; t != "" {
		title = t
	}
	body := opts.Metadata["pr_body"]
	if body == "" {
		body = "Translations produced by the kapi loop running on Bowrain."
	}

	pr, err := c.newClient(token).EnsureDeliveryPR(ctx, forge.DeliveryRequest{
		Repo:       c.repo,
		HeadBranch: c.deliveryBranch,
		BaseBranch: c.git.branch,
		Title:      title,
		Body:       body,
		Labels:     c.prLabels,
	})
	if err != nil {
		return fmt.Errorf("delivery branch pushed, but ensuring the pull request failed: %w", err)
	}
	slog.Info("forge delivery: pull request up to date",
		"connector", c.ID(), "project", c.projectID, "pr", pr.URL)
	return nil
}

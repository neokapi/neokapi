package connector

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/core/registry"
)

// gitAllowProtocol is the value forced into GIT_ALLOW_PROTOCOL for every git
// invocation, restricting git's transport helpers to the safe set. This is
// defense-in-depth on top of validateRepoURL: even if a crafted URL slipped
// through, git itself would refuse dangerous transports such as ext::, fd::,
// or local file paths.
const gitAllowProtocol = "https:ssh:git"

// scpLikeURLRe matches the scp-like SSH syntax git accepts, e.g.
// "git@github.com:org/repo.git" or "user@host.example.com:path/to/repo".
// The host part may not contain a slash (which would make it a path) and the
// whole expression must not begin with '-' (option injection).
var scpLikeURLRe = regexp.MustCompile(`^[A-Za-z0-9_.+~-]+@[A-Za-z0-9._-]+:[^\x00-\x1f]+$`)

// refNameRe is a conservative allowlist for git branch / ref names. It permits
// alphanumerics plus a handful of separators commonly seen in branch names
// (slash, dot, dash, underscore). It deliberately excludes whitespace, control
// characters, and shell/option metacharacters.
var refNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

// validateRepoURL rejects repository URLs that could be abused for remote code
// execution or option injection when passed to git. Only the https, ssh and
// git transports are permitted, plus the scp-like "user@host:path" SSH form.
// Dangerous transports (ext::, fd::, file://, transport helpers) and any value
// beginning with '-' are rejected.
func validateRepoURL(repoURL string) error {
	if repoURL == "" {
		return errors.New("git connector requires a non-empty 'repo' URL")
	}
	if strings.HasPrefix(repoURL, "-") {
		return fmt.Errorf("git repo URL %q may not begin with '-'", repoURL)
	}
	if strings.ContainsAny(repoURL, "\x00") {
		return fmt.Errorf("git repo URL %q contains a NUL byte", repoURL)
	}
	for _, r := range repoURL {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("git repo URL %q contains a control character", repoURL)
		}
	}

	// Explicit URL schemes (scheme://...). Reject anything not allowlisted,
	// including ext::, fd::, file://, and helper transports such as
	// "ext::sh -c ...". Note that ext:: / fd:: use a single colon and are
	// caught by the scheme-prefix checks below.
	lower := strings.ToLower(repoURL)
	switch {
	case strings.HasPrefix(lower, "https://"),
		strings.HasPrefix(lower, "ssh://"),
		strings.HasPrefix(lower, "git://"):
		return nil
	case strings.HasPrefix(lower, "ext::"),
		strings.HasPrefix(lower, "fd::"),
		strings.HasPrefix(lower, "file://"),
		strings.HasPrefix(lower, "http://"):
		return fmt.Errorf("git repo URL %q uses a disallowed transport", repoURL)
	}

	// Reject any remaining "scheme://" or "transport::" form that wasn't
	// explicitly allowlisted above.
	if i := strings.Index(repoURL, "://"); i >= 0 {
		return fmt.Errorf("git repo URL %q uses a disallowed transport", repoURL)
	}
	if strings.Contains(repoURL, "::") {
		return fmt.Errorf("git repo URL %q uses a disallowed transport helper", repoURL)
	}

	// scp-like SSH syntax: user@host:path.
	if scpLikeURLRe.MatchString(repoURL) {
		return nil
	}

	return fmt.Errorf("git repo URL %q is not an allowed https/ssh/git URL", repoURL)
}

// validateBranch rejects branch / ref names that could be parsed as options or
// that contain control characters or whitespace.
func validateBranch(branch string) error {
	if branch == "" {
		return errors.New("git branch may not be empty")
	}
	if strings.HasPrefix(branch, "-") {
		return fmt.Errorf("git branch %q may not begin with '-'", branch)
	}
	if !refNameRe.MatchString(branch) {
		return fmt.Errorf("git branch %q contains disallowed characters", branch)
	}
	return nil
}

// validate checks the connector's repo URL and branch before any git exec.
func (c *GitConnector) validate() error {
	if err := validateRepoURL(c.repoURL); err != nil {
		return err
	}
	return validateBranch(c.branch)
}

// gitCommand builds an exec.Cmd for git with GIT_ALLOW_PROTOCOL pinned to the
// safe transport set as defense-in-depth.
func gitCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(), "GIT_ALLOW_PROTOCOL="+gitAllowProtocol)
	return cmd
}

// GitConnector fetches and publishes localization content from Git repositories.
// It clones/pulls repos, discovers resource files via glob patterns,
// and uses a FileConnector internally for reading/writing.
type GitConnector struct {
	id             string
	name           string
	repoURL        string
	branch         string
	localPath      string
	patterns       []string // glob patterns to discover resource files
	formatRegistry *registry.FormatRegistry
	fileConnector  *FileConnector
	config         map[string]string
}

// NewGitConnector creates a new GitConnector.
func NewGitConnector(formatReg *registry.FormatRegistry, config map[string]string) (*GitConnector, error) {
	repoURL := config["repo"]
	if repoURL == "" {
		return nil, errors.New("git connector requires 'repo' config")
	}
	if err := validateRepoURL(repoURL); err != nil {
		return nil, err
	}
	branch := config["branch"]
	if branch == "" {
		branch = "main"
	}
	if err := validateBranch(branch); err != nil {
		return nil, err
	}
	localPath := config["local_path"]
	if localPath == "" {
		localPath = filepath.Join(os.TempDir(), "neokapi-git-"+filepath.Base(repoURL))
	}
	id := config["id"]
	if id == "" {
		id = "git-" + filepath.Base(repoURL)
	}
	patterns := strings.Split(config["patterns"], ",")
	if len(patterns) == 1 && patterns[0] == "" {
		patterns = []string{"**/*.html", "**/*.json", "**/*.yaml", "**/*.yml", "**/*.properties"}
	}

	return &GitConnector{
		id:             id,
		name:           config["name"],
		repoURL:        repoURL,
		branch:         branch,
		localPath:      localPath,
		patterns:       patterns,
		formatRegistry: formatReg,
		config:         config,
	}, nil
}

func (c *GitConnector) ID() string                  { return c.id }
func (c *GitConnector) Name() string                { return c.name }
func (c *GitConnector) Category() platconn.Category { return platconn.CategoryCode }

func (c *GitConnector) Configure(config map[string]string) error {
	for k, v := range config {
		c.config[k] = v
	}
	return nil
}

func (c *GitConnector) Close() error { return nil }

// ensureRepo clones or pulls the repository.
func (c *GitConnector) ensureRepo(ctx context.Context) error {
	if err := c.validate(); err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(c.localPath, ".git")); err == nil {
		// Repo exists, pull latest. The "--" separator ensures the remote and
		// branch are treated as positional arguments, never options.
		cmd := gitCommand(ctx, "-C", c.localPath, "pull", "origin", "--", c.branch)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git pull: %s: %w", string(out), err)
		}
		return nil
	}

	// Clone. The "--" separator ensures the repo URL and local path are treated
	// as positional arguments, never options.
	cmd := gitCommand(ctx, "clone", "--branch", c.branch, "--single-branch", "--", c.repoURL, c.localPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone: %s: %w", string(out), err)
	}
	return nil
}

// ensureFileConnector lazily initializes the internal file connector.
func (c *GitConnector) ensureFileConnector() error {
	if c.fileConnector != nil {
		return nil
	}
	fc, err := NewFileConnector(c.formatRegistry, map[string]string{
		"id":   c.id + "-files",
		"path": c.localPath,
	})
	if err != nil {
		return err
	}
	c.fileConnector = fc
	return nil
}

func (c *GitConnector) Fetch(ctx context.Context, opts platconn.FetchOptions) ([]*platconn.ContentItem, error) {
	if err := c.ensureRepo(ctx); err != nil {
		return nil, err
	}
	if err := c.ensureFileConnector(); err != nil {
		return nil, err
	}

	if len(opts.Paths) == 0 {
		// Discover files matching patterns.
		items, err := c.List(ctx)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			opts.Paths = append(opts.Paths, item.Path)
		}
	}

	return c.fileConnector.Fetch(ctx, opts)
}

func (c *GitConnector) Publish(ctx context.Context, items []*platconn.ContentItem, opts platconn.PublishOptions) error {
	if err := c.validate(); err != nil {
		return err
	}
	if err := c.ensureFileConnector(); err != nil {
		return err
	}

	if err := c.fileConnector.Publish(ctx, items, opts); err != nil {
		return err
	}

	// Commit and push.
	message := opts.Message
	if message == "" {
		message = "Update translations"
	}

	// Stage all changes.
	addCmd := gitCommand(ctx, "-C", c.localPath, "add", "-A")
	if out, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %s: %w", string(out), err)
	}

	// Commit. The "--" separator keeps the message bound to -m and prevents any
	// trailing positional argument from being parsed as an option.
	commitCmd := gitCommand(ctx, "-C", c.localPath, "commit", "-m", message)
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %s: %w", string(out), err)
	}

	// Push — if this fails, roll back the commit to avoid leaving the repo
	// in a dirty state with unpushed changes. The "--" separator ensures the
	// branch is treated as a positional refspec, never an option.
	pushCmd := gitCommand(ctx, "-C", c.localPath, "push", "origin", "--", c.branch)
	if out, err := pushCmd.CombinedOutput(); err != nil {
		// Attempt to undo the commit while keeping the working tree intact.
		resetCmd := gitCommand(ctx, "-C", c.localPath, "reset", "--soft", "HEAD~1")
		if resetOut, resetErr := resetCmd.CombinedOutput(); resetErr != nil {
			return fmt.Errorf("git push: %s: %w (rollback also failed: %s: %w)", string(out), err, string(resetOut), resetErr)
		}
		return fmt.Errorf("git push (rolled back commit): %s: %w", string(out), err)
	}
	return nil
}

func (c *GitConnector) List(ctx context.Context) ([]*platconn.ContentItem, error) {
	if err := c.ensureRepo(ctx); err != nil {
		return nil, err
	}
	if err := c.ensureFileConnector(); err != nil {
		return nil, err
	}

	allItems, err := c.fileConnector.List(ctx)
	if err != nil {
		return nil, err
	}

	// Filter by glob patterns.
	var filtered []*platconn.ContentItem
	for _, item := range allItems {
		for _, pattern := range c.patterns {
			matched, _ := filepath.Match(pattern, item.Path)
			if matched {
				filtered = append(filtered, item)
				break
			}
			// Try matching just the filename for simple patterns.
			matched, _ = filepath.Match(pattern, filepath.Base(item.Path))
			if matched {
				filtered = append(filtered, item)
				break
			}
		}
	}
	return filtered, nil
}

func (c *GitConnector) Status(ctx context.Context) (*platconn.SyncStatus, error) {
	items, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	return &platconn.SyncStatus{
		ConnectorID: c.id,
		LastSync:    time.Now(),
		ItemCount:   len(items),
	}, nil
}

package connector

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/registry"
	platconn "github.com/neokapi/neokapi/platform/connector"
)

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
		return nil, fmt.Errorf("git connector requires 'repo' config")
	}
	branch := config["branch"]
	if branch == "" {
		branch = "main"
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
	if _, err := os.Stat(filepath.Join(c.localPath, ".git")); err == nil {
		// Repo exists, pull latest.
		cmd := exec.CommandContext(ctx, "git", "-C", c.localPath, "pull", "origin", c.branch)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git pull: %s: %w", string(out), err)
		}
		return nil
	}

	// Clone.
	cmd := exec.CommandContext(ctx, "git", "clone", "--branch", c.branch, "--single-branch", c.repoURL, c.localPath)
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
	addCmd := exec.CommandContext(ctx, "git", "-C", c.localPath, "add", "-A")
	if out, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %s: %w", string(out), err)
	}

	// Commit.
	commitCmd := exec.CommandContext(ctx, "git", "-C", c.localPath, "commit", "-m", message)
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %s: %w", string(out), err)
	}

	// Push — if this fails, roll back the commit to avoid leaving the repo
	// in a dirty state with unpushed changes.
	pushCmd := exec.CommandContext(ctx, "git", "-C", c.localPath, "push", "origin", c.branch)
	if out, err := pushCmd.CombinedOutput(); err != nil {
		// Attempt to undo the commit while keeping the working tree intact.
		resetCmd := exec.CommandContext(ctx, "git", "-C", c.localPath, "reset", "--soft", "HEAD~1")
		if resetOut, resetErr := resetCmd.CombinedOutput(); resetErr != nil {
			return fmt.Errorf("git push: %s: %w (rollback also failed: %s: %v)", string(out), err, string(resetOut), resetErr)
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

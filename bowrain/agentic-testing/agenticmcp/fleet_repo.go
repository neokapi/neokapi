package agenticmcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// slugRe validates workspace slugs to prevent path traversal.
var slugRe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// isValidSlug returns true if slug is safe for use in filesystem paths.
func isValidSlug(slug string) bool {
	return slug != "" && slugRe.MatchString(slug)
}

// GitFleetRepo implements FleetRepo by cloning and operating on the
// agentic-fleet git repository.
type GitFleetRepo struct {
	// RepoURL is the git clone URL (HTTPS or SSH).
	RepoURL string

	// Token is an optional PAT for HTTPS auth (inserted into URL).
	Token string

	// LocalDir is the local checkout path. If empty, a temp dir is used.
	LocalDir string

	// CommitAuthor is the git author for commits (e.g., "coordinator").
	CommitAuthor string
}

// ensureClone clones or pulls the fleet repo.
func (r *GitFleetRepo) ensureClone(ctx context.Context) (string, error) {
	dir := r.LocalDir
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "fleet-repo")
	}

	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		// Already cloned — pull.
		cmd := exec.CommandContext(ctx, "git", "-C", dir, "pull", "--ff-only")
		if out, err := cmd.CombinedOutput(); err != nil {
			// Non-fatal: fetch instead if ff-only fails.
			cmd2 := exec.CommandContext(ctx, "git", "-C", dir, "fetch", "--all")
			cmd2.CombinedOutput() //nolint:errcheck
			_ = out
		}
		return dir, nil
	}

	// Clone.
	url := r.repoURL()
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", url, dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git clone: %s: %w", string(out), err)
	}
	return dir, nil
}

// repoURL returns the clone URL with token injected if needed.
func (r *GitFleetRepo) repoURL() string {
	if r.Token != "" && strings.HasPrefix(r.RepoURL, "https://") {
		return strings.Replace(r.RepoURL, "https://", "https://x-access-token:"+r.Token+"@", 1)
	}
	return r.RepoURL
}

// ListWorkspaces scans workspaces/*/plan.yaml in the fleet repo.
func (r *GitFleetRepo) ListWorkspaces(ctx context.Context) ([]WorkspaceMeta, error) {
	dir, err := r.ensureClone(ctx)
	if err != nil {
		return nil, err
	}

	wsDir := filepath.Join(dir, "workspaces")
	entries, err := os.ReadDir(wsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read workspaces dir: %w", err)
	}

	var workspaces []WorkspaceMeta
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "." || entry.Name() == ".." {
			continue
		}
		slug := entry.Name()

		plan, err := r.readPlan(filepath.Join(wsDir, slug, "plan.yaml"))
		if err != nil {
			continue // skip workspaces without a valid plan
		}

		status := r.readStatus(filepath.Join(wsDir, slug, "status.yaml"))

		workspaces = append(workspaces, WorkspaceMeta{
			Slug:               slug,
			Phase:              status.Phase,
			ProjectName:        plan.GetProjectName(),
			UpstreamRepo:       plan.GetUpstreamRepo(),
			TargetLanguages:    plan.GetTargetLanguages(),
			Mode:               plan.Mode,
			ProjectCount:       1,
			UntranslatedBlocks: map[string]int{}, // Empty, not nil — avoids MCP schema validation errors.
			Health:             "healthy",
		})
	}
	return workspaces, nil
}

// GetWorkspacePlan reads and parses a workspace's plan.yaml.
func (r *GitFleetRepo) GetWorkspacePlan(ctx context.Context, slug string) (*WorkspacePlan, error) {
	if !isValidSlug(slug) {
		return nil, fmt.Errorf("invalid workspace slug: %q", slug)
	}
	dir, err := r.ensureClone(ctx)
	if err != nil {
		return nil, err
	}
	return r.readPlan(filepath.Join(dir, "workspaces", slug, "plan.yaml"))
}

// CommitFile writes a file and commits it to the fleet repo.
func (r *GitFleetRepo) CommitFile(ctx context.Context, path, content, message string) (string, error) {
	dir, err := r.ensureClone(ctx)
	if err != nil {
		return "", err
	}

	fullPath := filepath.Join(dir, filepath.Clean(path))

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}

	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	author := r.CommitAuthor
	if author == "" {
		author = "coordinator"
	}

	// Configure git identity for this repo.
	for _, cfg := range [][]string{
		{"config", "user.name", "agent-" + author},
		{"config", "user.email", "agent-" + author + "@bowrain.cloud"},
	} {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, cfg...)...)
		cmd.CombinedOutput() //nolint:errcheck
	}

	// Stage, commit, push.
	cmds := [][]string{
		{"add", filepath.Clean(path)},
		{"commit", "-m", message},
		{"push"},
	}
	for _, args := range cmds {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("git %s: %s: %w", args[0], string(out), err)
		}
	}

	// Get commit SHA.
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// MemoryLogEntry represents a git commit that touched agent memory files.
type MemoryLogEntry struct {
	SHA       string   `json:"sha"`
	Agent     string   `json:"agent"`
	Message   string   `json:"message"`
	Timestamp string   `json:"timestamp"`
	Files     []string `json:"files,omitempty"`
}

// ListMemoryLog returns recent git commits that touched agent memory paths.
func (r *GitFleetRepo) ListMemoryLog(ctx context.Context, limit int) ([]MemoryLogEntry, error) {
	dir, err := r.ensureClone(ctx)
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		limit = 50
	}

	// git log with custom format: SHA|author|timestamp|message
	// Filter to commits touching workspaces/*/agents/*/memory/
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "log",
		fmt.Sprintf("--max-count=%d", limit),
		"--format=%H|%an|%aI|%s",
		"--name-only",
		"--diff-filter=ACMR",
		"--", "workspaces/*/agents/*/memory/*")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// No commits matching the path is not an error.
		if len(out) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("git log: %s: %w", string(out), err)
	}

	return parseMemoryLog(string(out)), nil
}

// parseMemoryLog parses the git log output into MemoryLogEntry structs.
func parseMemoryLog(output string) []MemoryLogEntry {
	var entries []MemoryLogEntry
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return nil
	}

	var current *MemoryLogEntry
	for _, line := range lines {
		if line == "" {
			if current != nil {
				current.Agent = extractAgent(current.Files)
				entries = append(entries, *current)
				current = nil
			}
			continue
		}

		parts := strings.SplitN(line, "|", 4)
		if len(parts) == 4 {
			// Header line: SHA|author|timestamp|message
			if current != nil {
				current.Agent = extractAgent(current.Files)
				entries = append(entries, *current)
			}
			sha := parts[0]
			if len(sha) > 8 {
				sha = sha[:8]
			}
			current = &MemoryLogEntry{
				SHA:       sha,
				Agent:     parts[1],
				Timestamp: parts[2],
				Message:   parts[3],
			}
		} else if current != nil && strings.Contains(line, "/memory/") {
			// File path line
			current.Files = append(current.Files, line)
		}
	}
	if current != nil {
		current.Agent = extractAgent(current.Files)
		entries = append(entries, *current)
	}
	return entries
}

// extractAgent extracts the agent name from memory file paths like
// workspaces/excalidraw-l10n/agents/alex/memory/foo.md
func extractAgent(files []string) string {
	for _, f := range files {
		parts := strings.Split(f, "/")
		for i, p := range parts {
			if p == "agents" && i+1 < len(parts) {
				return parts[i+1]
			}
		}
	}
	return "unknown"
}

// AgentFileInfo contains file content and git metadata.
type AgentFileInfo struct {
	Content    string `json:"content"`
	LastAuthor string `json:"last_author,omitempty"`
	LastDate   string `json:"last_date,omitempty"`
}

// ReadAgentFile reads a file from an agent's directory with git metadata.
func (r *GitFleetRepo) ReadAgentFile(ctx context.Context, workspace, agent, filename string) (string, error) {
	dir, err := r.ensureClone(ctx)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "workspaces", workspace, "agents", agent, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read agent file: %w", err)
	}
	return string(data), nil
}

// ReadAgentFileInfo reads a file with last-modified metadata from git log.
func (r *GitFleetRepo) ReadAgentFileInfo(ctx context.Context, workspace, agent, filename string) (*AgentFileInfo, error) {
	dir, err := r.ensureClone(ctx)
	if err != nil {
		return nil, err
	}
	relPath := filepath.Join("workspaces", workspace, "agents", agent, filename)
	fullPath := filepath.Join(dir, relPath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("read agent file: %w", err)
	}

	info := &AgentFileInfo{Content: string(data)}

	// Get last commit info for this file.
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "log", "-1", "--format=%an|%aI", "--", relPath)
	out, err := cmd.CombinedOutput()
	if err == nil {
		if author, date, ok := strings.Cut(strings.TrimSpace(string(out)), "|"); ok {
			info.LastAuthor = author
			info.LastDate = date
		}
	}

	return info, nil
}

// readPlan parses a plan.yaml file.
func (r *GitFleetRepo) readPlan(path string) (*WorkspacePlan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read plan: %w", err)
	}
	var plan WorkspacePlan
	if err := yaml.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("parse plan: %w", err)
	}
	return &plan, nil
}

// WorkspaceStatus is the parsed status.yaml for a workspace.
type WorkspaceStatus struct {
	Phase          string `yaml:"phase"`
	CurrentRelease string `yaml:"current_release"`
}

// GetWorkspaceStatus reads and parses a workspace's status.yaml.
func (r *GitFleetRepo) GetWorkspaceStatus(ctx context.Context, slug string) (*WorkspaceStatus, error) {
	if !isValidSlug(slug) {
		return nil, fmt.Errorf("invalid workspace slug: %q", slug)
	}
	dir, err := r.ensureClone(ctx)
	if err != nil {
		return nil, err
	}
	status := r.readStatus(filepath.Join(dir, "workspaces", slug, "status.yaml"))
	return &status, nil
}

// readStatus reads status.yaml, returning defaults if missing.
func (r *GitFleetRepo) readStatus(path string) WorkspaceStatus {
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkspaceStatus{Phase: "unknown"}
	}
	var status WorkspaceStatus
	if err := yaml.Unmarshal(data, &status); err != nil {
		return WorkspaceStatus{Phase: "unknown"}
	}
	return status
}

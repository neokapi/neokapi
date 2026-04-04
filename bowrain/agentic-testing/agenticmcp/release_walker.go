package agenticmcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	agentictesting "github.com/neokapi/neokapi/bowrain/agentic-testing"
)

// GitReleaseWalker implements ReleaseWalker by cloning the upstream fork on demand,
// discovering content via glob patterns, and pushing source blocks to Bowrain.
// It is fully self-contained — all it needs is fleet repo access, a Bowrain client,
// and a GitHub token for cloning forks.
type GitReleaseWalker struct {
	// Fleet provides access to workspace plans and status in the fleet repo.
	Fleet FleetRepo

	// Bowrain is the REST client for pushing source blocks.
	Bowrain *agentictesting.BowrainClient

	// GitHubToken is used to clone forks via HTTPS. If empty, SSH is assumed.
	GitHubToken string

	// CacheDir is a directory for caching fork clones across walks.
	// If empty, os.TempDir()/agentic-forks/ is used.
	CacheDir string
}

// WalkRelease advances a workspace to the specified (or next) release tag.
func (w *GitReleaseWalker) WalkRelease(ctx context.Context, workspaceSlug, projectID, tag string) (*ReleaseResult, error) {
	// 1. Read plan and status.
	plan, err := w.Fleet.GetWorkspacePlan(ctx, workspaceSlug)
	if err != nil {
		return nil, fmt.Errorf("read plan: %w", err)
	}
	status, err := w.Fleet.GetWorkspaceStatus(ctx, workspaceSlug)
	if err != nil {
		return nil, fmt.Errorf("read status: %w", err)
	}

	// 1b. Resolve project ID if not provided — look up by name from plan.yaml.
	if projectID == "" {
		projectName := plan.GetProjectName()
		if projectName == "" {
			return nil, errors.New("plan.yaml has no project name and no project_id was provided")
		}
		proj, err := w.Bowrain.FindProjectByName(ctx, workspaceSlug, projectName)
		if err != nil {
			return nil, fmt.Errorf("resolve project %q: %w", projectName, err)
		}
		projectID = proj.ID
	}

	// 2. Determine which tag to walk to.
	if tag == "" {
		next, err := nextTag(plan.ReleaseStrategy.Tags, status.CurrentRelease)
		if err != nil {
			return nil, err
		}
		tag = next
	}

	// 3. Clone or update the fork.
	forkDir, err := w.ensureFork(ctx, plan)
	if err != nil {
		return nil, fmt.Errorf("ensure fork: %w", err)
	}

	// 4. Merge the upstream tag into the fork.
	if err := mergeUpstreamTag(ctx, forkDir, tag); err != nil {
		return nil, fmt.Errorf("merge upstream %s: %w", tag, err)
	}

	// 5. Discover source file using the glob pattern.
	sourcePath, err := discoverSourceFile(forkDir, plan.Content.SourceFilePattern)
	if err != nil {
		return nil, fmt.Errorf("discover source: %w", err)
	}

	// 6. Strip target language files from the fork.
	if err := stripTargetFiles(ctx, forkDir, sourcePath, plan.GetTargetLanguages(), tag); err != nil {
		return nil, fmt.Errorf("strip targets: %w", err)
	}

	// 7. Extract and flatten the source JSON.
	flat, err := extractFlatJSON(forkDir, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("extract source: %w", err)
	}

	// 8. Push source blocks to Bowrain.
	blocks := flatToBlocks(flat, filepath.Base(sourcePath))
	if err := w.Bowrain.PushBlocks(ctx, workspaceSlug, projectID, blocks); err != nil {
		return nil, fmt.Errorf("push blocks: %w", err)
	}

	// 9. Create a stream for this release tag.
	_ = w.Bowrain.CreateStream(ctx, workspaceSlug, projectID, tag, "Release "+tag)

	// 10. Update status.yaml in the fleet repo.
	statusContent := fmt.Sprintf("phase: active\ncurrent_release: %s\n", tag)
	_, err = w.Fleet.CommitFile(ctx,
		fmt.Sprintf("workspaces/%s/status.yaml", workspaceSlug),
		statusContent,
		fmt.Sprintf("walk: advance %s to %s", workspaceSlug, tag),
	)
	if err != nil {
		return nil, fmt.Errorf("update status: %w", err)
	}

	return &ReleaseResult{
		Tag:         tag,
		BlocksAdded: len(blocks),
	}, nil
}

// ensureFork clones or updates the fork repo locally. Reads fork URL from plan.yaml.
func (w *GitReleaseWalker) ensureFork(ctx context.Context, plan *WorkspacePlan) (string, error) {
	forkGH := plan.ForkRepo()
	if forkGH == "" {
		return "", errors.New("plan.yaml has no upstream.fork")
	}
	upstreamGH := plan.GetUpstreamRepo()

	// Derive a stable local directory name from the fork slug.
	cacheDir := w.CacheDir
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "agentic-forks")
	}
	// Use the fork name as directory (e.g., "agentic-excalidraw").
	_, forkName, ok := strings.Cut(forkGH, "/")
	if !ok {
		return "", fmt.Errorf("invalid fork repo %q (expected owner/name)", forkGH)
	}
	forkDir := filepath.Join(cacheDir, forkName)

	forkURL := w.githubURL(forkGH)
	upstreamURL := w.githubURL(upstreamGH)

	if _, err := os.Stat(filepath.Join(forkDir, ".git")); err == nil {
		// Already cloned — fetch latest.
		cmd := exec.CommandContext(ctx, "git", "-C", forkDir, "fetch", "--all", "--tags")
		cmd.CombinedOutput() //nolint:errcheck // best-effort fetch; non-fatal if it fails
		return forkDir, nil
	}

	// Clone the fork.
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir cache: %w", err)
	}
	cmd := exec.CommandContext(ctx, "git", "clone", forkURL, forkDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git clone fork: %s: %w", string(out), err)
	}

	// Add upstream remote pointing at the original repo.
	if upstreamGH != "" {
		cmd = exec.CommandContext(ctx, "git", "-C", forkDir, "remote", "add", "upstream", upstreamURL)
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("git remote add upstream: %s: %w", string(out), err)
		}
		// Fetch upstream tags.
		cmd = exec.CommandContext(ctx, "git", "-C", forkDir, "fetch", "upstream", "--tags")
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("git fetch upstream: %s: %w", string(out), err)
		}
	}

	// Configure git identity for commits (target stripping).
	for _, cfg := range [][]string{
		{"config", "user.name", "agentic-walker"},
		{"config", "user.email", "walker@bowrain.cloud"},
	} {
		cmd = exec.CommandContext(ctx, "git", append([]string{"-C", forkDir}, cfg...)...)
		cmd.CombinedOutput() //nolint:errcheck // best-effort git config; non-fatal if it fails
	}

	return forkDir, nil
}

// githubURL returns the HTTPS clone URL for a GitHub owner/name, with token if available.
func (w *GitReleaseWalker) githubURL(ownerName string) string {
	if ownerName == "" {
		return ""
	}
	base := "https://github.com/" + ownerName + ".git"
	if w.GitHubToken != "" {
		return "https://x-access-token:" + w.GitHubToken + "@github.com/" + ownerName + ".git"
	}
	return base
}

// nextTag returns the tag after current in the sequence.
func nextTag(tags []string, current string) (string, error) {
	if len(tags) == 0 {
		return "", errors.New("no tags in release strategy")
	}
	if current == "" {
		return tags[0], nil
	}
	for i, t := range tags {
		if t == current && i+1 < len(tags) {
			return tags[i+1], nil
		}
	}
	return "", fmt.Errorf("no tag after %q in release strategy (at end of sequence or not found)", current)
}

// mergeUpstreamTag fetches and merges an upstream tag into the fork.
func mergeUpstreamTag(ctx context.Context, forkDir, tag string) error {
	cmds := [][]string{
		{"fetch", "upstream", fmt.Sprintf("refs/tags/%s:refs/tags/%s", tag, tag)},
		{"merge", tag, "--no-edit", "-m", "Merge upstream " + tag},
	}
	for _, args := range cmds {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", forkDir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %s: %w", args[0], string(out), err)
		}
	}
	return nil
}

// discoverSourceFile finds the source locale file in the working tree using a glob pattern.
func discoverSourceFile(forkDir, pattern string) (string, error) {
	cmd := exec.CommandContext(context.Background(), "git", "-C", forkDir, "ls-files")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git ls-files: %w", err)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		matched, err := filepath.Match(pattern, line)
		if err != nil {
			continue
		}
		if matched {
			return line, nil
		}
		// filepath.Match doesn't support **. For "**/suffix" patterns,
		// check if the file path ends with the suffix.
		if strings.HasPrefix(pattern, "**/") {
			suffix := pattern[3:]
			if strings.HasSuffix(line, suffix) || strings.Contains(line, "/"+suffix) {
				return line, nil
			}
		}
	}
	return "", fmt.Errorf("no file matching %q found in %s", pattern, forkDir)
}

// deriveLocalePath replaces the source filename with a locale-specific one.
// e.g., "src/locales/en.json" + "fr-FR" → "src/locales/fr-FR.json"
func deriveLocalePath(sourcePath, locale string) string {
	dir := filepath.Dir(sourcePath)
	ext := filepath.Ext(sourcePath)
	return filepath.Join(dir, locale+ext)
}

// stripTargetFiles removes target locale files from the fork and commits the removal.
func stripTargetFiles(ctx context.Context, forkDir, sourcePath string, targetLocales []string, tag string) error {
	var removed []string
	for _, locale := range targetLocales {
		localePath := deriveLocalePath(sourcePath, locale)
		fullPath := filepath.Join(forkDir, localePath)
		if _, err := os.Stat(fullPath); err != nil {
			continue // File doesn't exist at this tag — skip.
		}
		if err := os.Remove(fullPath); err != nil {
			return fmt.Errorf("remove %s: %w", localePath, err)
		}
		removed = append(removed, localePath)
	}

	if len(removed) == 0 {
		return nil
	}

	// Stage removals and commit.
	args := append([]string{"-C", forkDir, "rm", "--cached", "--"}, removed...)
	cmd := exec.CommandContext(ctx, "git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git rm: %s: %w", string(out), err)
	}

	cmd = exec.CommandContext(ctx, "git", "-C", forkDir, "commit", "-m",
		"strip target locales for "+tag)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit strip: %s: %w", string(out), err)
	}
	return nil
}

// extractFlatJSON reads a JSON file and flattens nested keys to dot-separated paths.
func extractFlatJSON(forkDir, relPath string) (map[string]string, error) {
	fullPath := filepath.Join(forkDir, relPath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", relPath, err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse %s: %w", relPath, err)
	}

	flat := make(map[string]string)
	flattenJSON("", raw, flat)
	return flat, nil
}

// flattenJSON recursively flattens a nested map into dot-separated keys.
func flattenJSON(prefix string, m map[string]any, out map[string]string) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		switch val := v.(type) {
		case map[string]any:
			flattenJSON(key, val, out)
		default:
			out[key] = fmt.Sprintf("%v", val)
		}
	}
}

// flatToBlocks converts a flat key→value map to PushBlock slice.
func flatToBlocks(flat map[string]string, itemName string) []agentictesting.PushBlock {
	blocks := make([]agentictesting.PushBlock, 0, len(flat))
	for key, text := range flat {
		blocks = append(blocks, agentictesting.PushBlock{
			ID:       key,
			Text:     text,
			Name:     key,
			ItemName: itemName,
		})
	}
	return blocks
}

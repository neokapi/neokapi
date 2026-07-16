package connector

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/bowrain/forge"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewForgeConnectorValidation(t *testing.T) {
	reg := registry.NewFormatRegistry()
	base := func() map[string]string {
		return map[string]string{
			"repo":       "https://github.com/acme/site.git",
			"token":      "tok",
			"project_id": "proj-1",
		}
	}

	c, err := NewForgeConnector(reg, base())
	require.NoError(t, err)
	assert.Equal(t, forge.KindGitHub, c.Kind())
	assert.Equal(t, "acme/site", c.RepoPath())
	assert.Equal(t, "main", c.BaseBranch())
	assert.Equal(t, DefaultDeliveryBranch, c.deliveryBranch)
	assert.Equal(t, "proj-1", c.ProjectID())

	cfg := base()
	delete(cfg, "token")
	_, err = NewForgeConnector(reg, cfg)
	require.ErrorContains(t, err, "token")

	cfg = base()
	delete(cfg, "project_id")
	_, err = NewForgeConnector(reg, cfg)
	require.ErrorContains(t, err, "project_id")

	cfg = base()
	cfg["repo"] = "git@github.com:acme/site.git"
	_, err = NewForgeConnector(reg, cfg)
	require.ErrorContains(t, err, "https")

	cfg = base()
	cfg["delivery_branch"] = "main"
	_, err = NewForgeConnector(reg, cfg)
	require.ErrorContains(t, err, "must differ")

	cfg = base()
	cfg["forge"] = "bitbucket"
	_, err = NewForgeConnector(reg, cfg)
	require.ErrorContains(t, err, "unknown forge")

	cfg = base()
	cfg["repo"] = "https://gitlab.example.com/group/site.git"
	c, err = NewForgeConnector(reg, cfg)
	require.NoError(t, err)
	assert.Equal(t, forge.KindGitLab, c.Kind(), "self-managed hosts default to GitLab")
}

// fakeForgeClient records the delivery request instead of calling a forge.
type fakeForgeClient struct {
	req    *forge.DeliveryRequest
	called int
}

func (f *fakeForgeClient) EnsureDeliveryPR(_ context.Context, req forge.DeliveryRequest) (forge.PR, error) {
	f.called++
	f.req = &req
	return forge.PR{Number: 1, URL: "https://example.com/pr/1"}, nil
}

// gitRun is a test helper: run git in dir, fail the test on error.
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return string(out)
}

// TestForgeConnectorPublish_BranchAndPR drives the whole delivery against a
// real local bare repository: the produced file must land on the delivery
// branch (never the tracked branch), the PR must be ensured with the report
// body, and a delivery with no changes must do neither.
func TestForgeConnectorPublish_BranchAndPR(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	// The fixture origin is a local bare repo (file transport), which the
	// production protocol pin rightly blocks; widen it for this test only.
	prev := allowedGitProtocols
	allowedGitProtocols = gitAllowProtocol + ":file"
	t.Cleanup(func() { allowedGitProtocols = prev })
	tmp := t.TempDir()

	// Bare "origin" with an initial commit on main carrying the source file.
	origin := filepath.Join(tmp, "origin.git")
	gitRun(t, tmp, "init", "--bare", "-b", "main", origin)
	seed := filepath.Join(tmp, "seed")
	gitRun(t, tmp, "clone", origin, seed)
	require.NoError(t, os.WriteFile(filepath.Join(seed, "en.txt"), []byte("Hello\n"), 0o644))
	gitRun(t, seed, "add", "-A")
	gitRun(t, seed, "commit", "-m", "seed")
	gitRun(t, seed, "push", "origin", "main")

	// The connector's clone, pre-created so ensureRepo takes the pull path and
	// origin stays the local bare repo (the https repo URL is only validated,
	// never dialed).
	clone := filepath.Join(tmp, "clone")
	gitRun(t, tmp, "clone", origin, clone)

	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	c, err := NewForgeConnector(reg, map[string]string{
		"repo":       "https://github.com/acme/site.git",
		"token":      "tok",
		"project_id": "proj-1",
		"local_path": clone,
		"pr_labels":  "translations, kapi",
	})
	require.NoError(t, err)
	fake := &fakeForgeClient{}
	c.newClient = func() forge.Client { return fake }

	items := []*platconn.ContentItem{{
		ID:     "fr.txt",
		Path:   "fr.txt",
		Format: "plaintext",
		Blocks: []*model.Block{
			model.NewBlock("greeting", "Bonjour"),
		},
	}}
	err = c.Publish(context.Background(), items, platconn.PublishOptions{
		Message: "chore: update translations",
		Metadata: map[string]string{
			"pr_title": "Update translations (converged)",
			"pr_body":  "## report",
		},
	})
	require.NoError(t, err)

	// The delivery branch on origin carries the file; main does not.
	assert.Contains(t, gitRun(t, origin, "show", DefaultDeliveryBranch+":fr.txt"), "Bonjour")
	lsMain := gitRun(t, origin, "ls-tree", "--name-only", "main")
	assert.NotContains(t, lsMain, "fr.txt", "tracked branch must never receive direct pushes")

	// The PR was ensured against the right coordinates with the report body.
	require.Equal(t, 1, fake.called)
	assert.Equal(t, DefaultDeliveryBranch, fake.req.HeadBranch)
	assert.Equal(t, "main", fake.req.BaseBranch)
	assert.Equal(t, "Update translations (converged)", fake.req.Title)
	assert.Equal(t, "## report", fake.req.Body)
	assert.Equal(t, []string{"translations", "kapi"}, fake.req.Labels)

	// The clone is back on the tracked branch, ready for the next Fetch.
	assert.Contains(t, gitRun(t, clone, "branch", "--show-current"), "main")

	// Re-delivering while the PR is unmerged updates it in place (the branch
	// is recreated from the tracked tip, so the same output re-delivers).
	err = c.Publish(context.Background(), items, platconn.PublishOptions{Message: "again"})
	require.NoError(t, err)
	assert.Equal(t, 2, fake.called, "re-delivery keeps the one PR fresh")

	// Once the delivery is merged into the tracked branch, producing the same
	// output changes nothing against its tip: no push, no PR call — the loop
	// terminates instead of re-opening merged work.
	gitRun(t, seed, "pull", "origin", "main")
	gitRun(t, seed, "fetch", "origin", DefaultDeliveryBranch)
	gitRun(t, seed, "merge", "--no-edit", "origin/"+DefaultDeliveryBranch)
	gitRun(t, seed, "push", "origin", "main")
	err = c.Publish(context.Background(), items, platconn.PublishOptions{Message: "third"})
	require.NoError(t, err)
	assert.Equal(t, 2, fake.called, "merged output must not re-deliver")
}

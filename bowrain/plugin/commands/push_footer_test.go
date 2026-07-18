package commands

import (
	"testing"

	bproject "github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	"github.com/neokapi/neokapi/bowrain/plugin/schema"
	"github.com/stretchr/testify/assert"
)

func footerProject(url string, converge schema.ConvergePolicy) *bproject.Project {
	return &bproject.Project{
		Recipe: &bproject.Recipe{
			Server: &bproject.ServerSpec{URL: url, Converge: converge},
		},
	}
}

func TestApplyLoopStatus_WorkspaceProjectDefaultsOnPush(t *testing.T) {
	out := output.PushOutput{}
	applyLoopStatus(&out, footerProject("https://bowrain.example.com/acme/proj123", ""), "main")

	// An empty converge resolves to the connected-project default.
	assert.Equal(t, "on-push", out.Converge)
	assert.Equal(t, "https://bowrain.example.com/acme/p/proj123/s/main", out.ProjectURL)
	assert.Equal(t, "https://bowrain.example.com/acme/tasks", out.ReviewURL)
}

func TestApplyLoopStatus_ManualPolicyAndStream(t *testing.T) {
	out := output.PushOutput{}
	applyLoopStatus(&out, footerProject("https://bowrain.example.com/acme/proj123", "manual"), "v2.0")

	assert.Equal(t, "manual", out.Converge)
	assert.Equal(t, "https://bowrain.example.com/acme/p/proj123/s/v2.0", out.ProjectURL)
	assert.Equal(t, "https://bowrain.example.com/acme/tasks", out.ReviewURL)
}

func TestApplyLoopStatus_EmptyStreamFallsBackToMain(t *testing.T) {
	out := output.PushOutput{}
	applyLoopStatus(&out, footerProject("https://bowrain.example.com/acme/proj123", ""), "")

	assert.Equal(t, "https://bowrain.example.com/acme/p/proj123/s/main", out.ProjectURL)
}

func TestApplyLoopStatus_DirectProjectHasPolicyButNoWebURLs(t *testing.T) {
	// A direct (workspace-less) compound URL cannot address the web surfaces,
	// which live under /<workspace>/ — the policy still prints.
	out := output.PushOutput{}
	applyLoopStatus(&out, footerProject("https://bowrain.example.com/projects/proj123", ""), "main")

	assert.Equal(t, "on-push", out.Converge)
	assert.Empty(t, out.ProjectURL)
	assert.Empty(t, out.ReviewURL)
}

func TestApplyLoopStatus_NoServerBlockIsSilent(t *testing.T) {
	out := output.PushOutput{}
	applyLoopStatus(&out, &bproject.Project{Recipe: &bproject.Recipe{}}, "main")

	assert.Empty(t, out.Converge)
	assert.Empty(t, out.ProjectURL)
	assert.Empty(t, out.ReviewURL)
}

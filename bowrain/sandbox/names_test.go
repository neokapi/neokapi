package sandbox

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A file sent into the sandbox lands inside /workspace or is refused before
// anything reaches Docker.
func TestValidateWorkspaceName(t *testing.T) {
	for _, ok := range []string{"main.go", "src/app.json", "a/b/c.txt", "./x.txt"} {
		assert.NoError(t, validateWorkspaceName(ok), ok)
	}
	for _, bad := range []string{"", ".", "/etc/passwd", "../x", "a/../../x", `a\b`, "a\x00b", "/"} {
		assert.Error(t, validateWorkspaceName(bad), "%q", bad)
	}
}

func TestCopyFilesRefusesANameOutsideTheWorkspace(t *testing.T) {
	d := &DockerSandbox{baseURL: "http://127.0.0.1:0"}
	err := d.copyFiles(context.Background(), "c1", map[string][]byte{"../escape.sh": []byte("x")})
	require.ErrorContains(t, err, "relative path inside the workspace")
}

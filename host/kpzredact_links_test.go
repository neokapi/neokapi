//go:build unix

package host

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPolicyDetector_RulesLinkOut asserts the rules file a policy names is
// read through the project root: a regular one compiles, and one that is a
// link out of the project is refused.
func TestPolicyDetector_RulesLinkOut(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "rules.yaml")
	rules := []byte("version: v1\n")
	require.NoError(t, os.WriteFile(outside, rules, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "rules.yaml"), rules, 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "linked.yaml")))

	_, err := policyDetector(&project.RedactionSpec{Enabled: true, Rules: "rules.yaml"}, root)
	require.NoError(t, err, "a rules file inside the project compiles")

	_, err = policyDetector(&project.RedactionSpec{Enabled: true, Rules: "linked.yaml"}, root)
	assert.Error(t, err, "a rules file reached through a link out of the project is refused")
}

package tools_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/tools"
)

// redact and unredact given a vault an earlier kapi wrote refuse it, and the
// refusal names the vault and what to run.
func TestRedactAndUnredact_RefuseAVaultAnEarlierKapiWrote(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vault.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"version": 1, "values": []}`), 0o600))

	_, err := tools.NewRedactTool(&tools.RedactConfig{Rules: beanRules(), VaultPath: path})
	require.ErrorIs(t, err, redaction.ErrOlderVault)
	assert.Equal(t, path+" is a redaction vault from an earlier kapi (version 1). Move it aside and run the redaction again to write a new one.", err.Error())

	_, err = tools.NewUnredactTool(&tools.UnredactConfig{VaultPath: path})
	require.ErrorIs(t, err, redaction.ErrOlderVault)
	assert.Equal(t, path+" is a redaction vault from an earlier kapi (version 1). Run the redaction again to write a new one, then restore from it.", err.Error())
}

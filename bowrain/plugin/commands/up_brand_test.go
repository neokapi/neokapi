package commands

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/cli"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// brandReportCmd builds a bare cobra command with captured stdout/stderr, the
// way reportBrandPush sees the up command at runtime.
func brandReportCmd() (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	cmd := &cobra.Command{Use: "server-up"}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	return cmd, stdout, stderr
}

// TestReportBrandPush_TextMatchesPushFooter asserts up's push phase renders the
// exact footer wording `kapi push` uses for the brand-profile upsert.
func TestReportBrandPush_TextMatchesPushFooter(t *testing.T) {
	tests := []struct {
		name string
		res  *PushBrandResult
		want string
	}{
		{
			name: "created",
			res:  &PushBrandResult{Name: "Acme Voice", Action: "created", Version: 1},
			want: "Brand profile: \"Acme Voice\" created in the workspace brand hub (v1)\n",
		},
		{
			name: "updated reports the new version",
			res:  &PushBrandResult{Name: "Acme Voice", Action: "updated", Version: 4},
			want: "Brand profile: \"Acme Voice\" updated → v4 (previous version kept in history)\n",
		},
		{
			name: "unchanged",
			res:  &PushBrandResult{Name: "Acme Voice", Action: "unchanged", Version: 2},
			want: "Brand profile: \"Acme Voice\" unchanged\n",
		},
		{
			name: "403 degrades to a skipped note",
			res:  &PushBrandResult{Name: "Acme Voice", Action: "skipped", Reason: "requires the manage-brand permission"},
			want: "Brand profile: \"Acme Voice\" not pushed (requires the manage-brand permission)\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, stdout, stderr := brandReportCmd()
			reportBrandPush(cmd, tc.res, false)
			assert.Equal(t, tc.want, stderr.String(), "the footer renders on stderr, next to the push messages")
			assert.Empty(t, stdout.String(), "text mode writes nothing to stdout")
		})
	}
}

// TestReportBrandPush_JSONLine asserts the --json venue emits one discriminated
// NDJSON line with the same field names `kapi push --json` uses.
func TestReportBrandPush_JSONLine(t *testing.T) {
	cmd, stdout, stderr := brandReportCmd()
	reportBrandPush(cmd, &PushBrandResult{Name: "Acme Voice", Action: "updated", Version: 4}, true)

	assert.Empty(t, stderr.String(), "JSON mode writes nothing to stderr")
	var line map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &line))
	assert.Equal(t, "brand_profile", line["type"])
	assert.Equal(t, "Acme Voice", line["brand_profile"])
	assert.Equal(t, "updated", line["brand_profile_action"])
	assert.Equal(t, float64(4), line["brand_profile_version"])
	assert.NotContains(t, line, "brand_profile_reason", "empty reason is omitted")
}

// TestReportBrandPush_JSONSkippedCarriesReason asserts a degraded 403 skip
// still surfaces its reason on the NDJSON line.
func TestReportBrandPush_JSONSkippedCarriesReason(t *testing.T) {
	cmd, stdout, _ := brandReportCmd()
	reportBrandPush(cmd, &PushBrandResult{Name: "Acme Voice", Action: "skipped", Reason: "requires the manage-brand permission"}, true)

	var line map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &line))
	assert.Equal(t, "skipped", line["brand_profile_action"])
	assert.Equal(t, "requires the manage-brand permission", line["brand_profile_reason"])
	assert.NotContains(t, line, "brand_profile_version", "a skip has no stored version")
}

// TestReportBrandPush_NilResultIsSilent covers the silent skips (unauthenticated,
// unclaimed project, no bound profile, --no-brand): brandPush returns nil and
// nothing renders in either venue.
func TestReportBrandPush_NilResultIsSilent(t *testing.T) {
	for _, jsonOut := range []bool{false, true} {
		cmd, stdout, stderr := brandReportCmd()
		reportBrandPush(cmd, nil, jsonOut)
		assert.Empty(t, stdout.String())
		assert.Empty(t, stderr.String())
	}
}

// TestReportBrandPush_QuietSuppressesTextNotJSON asserts --quiet silences the
// text footer while the structured NDJSON line still travels (matching how the
// rest of up treats --quiet vs --json).
func TestReportBrandPush_QuietSuppressesTextNotJSON(t *testing.T) {
	prev := app
	app = &cli.App{Quiet: true}
	t.Cleanup(func() { app = prev })

	cmd, stdout, stderr := brandReportCmd()
	reportBrandPush(cmd, &PushBrandResult{Name: "Acme Voice", Action: "created", Version: 1}, false)
	assert.Empty(t, stderr.String(), "quiet suppresses the text footer")
	assert.Empty(t, stdout.String())

	cmd, stdout, _ = brandReportCmd()
	reportBrandPush(cmd, &PushBrandResult{Name: "Acme Voice", Action: "created", Version: 1}, true)
	assert.NotEmpty(t, stdout.String(), "the NDJSON line is structured output, not chatter")
}

// TestUpCmdHasNoBrandFlag asserts the up verb mirrors push's --no-brand opt-out.
func TestUpCmdHasNoBrandFlag(t *testing.T) {
	f := upCmd.Flags().Lookup("no-brand")
	require.NotNil(t, f, "kapi up must expose --no-brand, mirroring kapi push")
	assert.Equal(t, "false", f.DefValue, "brand push is default-on")
}

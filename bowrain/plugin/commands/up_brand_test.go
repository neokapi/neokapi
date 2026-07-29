package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"syscall"
	"testing"

	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
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
// exact footer wording `kapi push` uses for the voice it carried.
//
// The created/updated/unchanged wording this used to assert is gone with the
// separate REST upsert: the voice now travels inside the push, so the client
// reports that it carried it and the worker decides which of those it was.
func TestReportBrandPush_TextMatchesPushFooter(t *testing.T) {
	tests := []struct {
		name string
		res  *PushBrandResult
		want string
	}{
		{
			name: "carried with the push",
			res:  &PushBrandResult{Name: "Acme Voice", Action: "carried"},
			want: "Brand profile: \"Acme Voice\" carried to the workspace brand hub\n",
		},
		{
			name: "a dry run says what it would carry",
			res:  &PushBrandResult{Name: "Acme Voice", Action: "would-push"},
			want: "Would push brand profile \"Acme Voice\" to the workspace brand hub\n",
		},
		{
			name: "--no-brand degrades to a skipped note",
			res:  &PushBrandResult{Name: "Acme Voice", Action: "skipped", Reason: "--no-brand"},
			want: "Brand profile: \"Acme Voice\" not pushed (--no-brand)\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd, stdout, stderr := brandReportCmd()
			require.NoError(t, reportBrandPush(cmd, nil, tc.res, false))
			assert.Equal(t, tc.want, stderr.String(), "the footer renders on stderr, next to the push messages")
			assert.Empty(t, stdout.String(), "text mode writes nothing to stdout")
		})
	}
}

// TestReportBrandPush_JSONLine asserts the --json venue emits one discriminated
// NDJSON line with the same field names `kapi push --json` uses.
func TestReportBrandPush_JSONLine(t *testing.T) {
	cmd, stdout, stderr := brandReportCmd()
	require.NoError(t, reportBrandPush(cmd, nil, &PushBrandResult{Name: "Acme Voice", Action: "carried"}, true))

	assert.Empty(t, stderr.String(), "JSON mode writes nothing to stderr")
	var line map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &line))
	assert.Equal(t, "brand_profile", line["type"])
	assert.Equal(t, "Acme Voice", line["brand_profile"])
	assert.Equal(t, "carried", line["brand_profile_action"])
	assert.NotContains(t, line, "brand_profile_reason", "empty reason is omitted")
	assert.NotContains(t, line, "brand_profile_version", "the version is the worker's to decide")
}

// TestReportBrandPush_JSONSkippedCarriesReason asserts a skipped governance
// push still surfaces its reason on the NDJSON line.
func TestReportBrandPush_JSONSkippedCarriesReason(t *testing.T) {
	cmd, stdout, _ := brandReportCmd()
	require.NoError(t, reportBrandPush(cmd, nil, &PushBrandResult{Name: "Acme Voice", Action: "skipped", Reason: "--no-brand"}, true))

	var line map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &line))
	assert.Equal(t, "skipped", line["brand_profile_action"])
	assert.Equal(t, "--no-brand", line["brand_profile_reason"])
	assert.NotContains(t, line, "brand_profile_version", "a skip has no stored version")
}

// TestReportBrandPush_NilResultIsSilent covers the silent skips (unconnected
// project, no bound profile): the push carries no governance result and nothing
// renders in either venue.
func TestReportBrandPush_NilResultIsSilent(t *testing.T) {
	for _, jsonOut := range []bool{false, true} {
		cmd, stdout, stderr := brandReportCmd()
		require.NoError(t, reportBrandPush(cmd, nil, nil, jsonOut))
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
	require.NoError(t, reportBrandPush(cmd, nil, &PushBrandResult{Name: "Acme Voice", Action: "carried"}, false))
	assert.Empty(t, stderr.String(), "quiet suppresses the text footer")
	assert.Empty(t, stdout.String())

	cmd, stdout, _ = brandReportCmd()
	require.NoError(t, reportBrandPush(cmd, nil, &PushBrandResult{Name: "Acme Voice", Action: "carried"}, true))
	assert.NotEmpty(t, stdout.String(), "the NDJSON line is structured output, not chatter")
}

// nthWriteFailer fails the nth Write (1-based) with err and succeeds otherwise.
// The failure sits at the writer so json.Encoder stays in the path, exercising the
// same wrapping a real stdout failure has.
type nthWriteFailer struct {
	n      int
	err    error
	writes int
	buf    bytes.Buffer
}

func (w *nthWriteFailer) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == w.n {
		return 0, w.err
	}
	return w.buf.Write(p)
}

// TestReportBrandPush_ReportsTruncation is the regression for the dropped
// `_ = enc.Encode(...)` at up.go's brand-push record. Unlike host's
// PrintUpResult, this discriminated record's error was thrown away, so a consumer
// could be told a brand profile travelled when its line never landed.
func TestReportBrandPush_ReportsTruncation(t *testing.T) {
	cmd := &cobra.Command{Use: "server-up"}
	w := &nthWriteFailer{n: 1, err: &os.PathError{Op: "write", Path: "/out.ndjson", Err: syscall.ENOSPC}}
	cmd.SetOut(w)

	err := reportBrandPush(cmd, nil, &PushBrandResult{Name: "Acme Voice", Action: "carried"}, true)
	require.Error(t, err, "a record the consumer never received must not be reported as sent")
	assert.ErrorIs(t, err, syscall.ENOSPC)
}

// TestReportBrandPush_ClosedConsumerIsQuiet is the discriminating control: a
// consumer that closed the pipe must not fail the push.
func TestReportBrandPush_ClosedConsumerIsQuiet(t *testing.T) {
	cmd := &cobra.Command{Use: "server-up"}
	w := &nthWriteFailer{n: 1, err: &os.PathError{Op: "write", Path: "/dev/stdout", Err: syscall.EPIPE}}
	cmd.SetOut(w)

	assert.NoError(t, reportBrandPush(cmd, nil, &PushBrandResult{Name: "Acme Voice", Action: "carried"}, true),
		"`kapi up --json | head` must not fail the push")
}

// TestReportBrandPush_SharesTheRunStream asserts the record joins the caller's
// NDJSON document rather than opening a private encoder: a truncation at the brand
// line is then still reported by the run's single closing check, so bowrain's
// stream accounting matches kapi's instead of drifting from it.
func TestReportBrandPush_SharesTheRunStream(t *testing.T) {
	cmd := &cobra.Command{Use: "server-up"}
	w := &nthWriteFailer{n: 1, err: &os.PathError{Op: "write", Path: "/out.ndjson", Err: syscall.ENOSPC}}
	cmd.SetOut(w)
	stream := output.NewNDJSONStream(cmd.OutOrStdout())

	require.Error(t, reportBrandPush(cmd, stream, &PushBrandResult{Name: "Acme Voice", Action: "carried"}, true))
	require.Error(t, stream.Report("kapi up --json"),
		"the shared stream remembers the loss, so the run's own check reports it too")
}

// TestUpCmdHasNoBrandFlag asserts the up verb mirrors push's --no-brand opt-out.
func TestUpCmdHasNoBrandFlag(t *testing.T) {
	f := upCmd.Flags().Lookup("no-brand")
	require.NotNil(t, f, "kapi up must expose --no-brand, mirroring kapi push")
	assert.Equal(t, "false", f.DefValue, "brand push is default-on")
}

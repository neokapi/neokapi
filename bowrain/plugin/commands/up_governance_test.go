package commands

import (
	"encoding/json"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/neokapi/neokapi/host/venue/transfer"

	"github.com/neokapi/neokapi/bowrain/plugin/commands/output"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// refusedPush is a push whose governance report refuses a claim of every kind
// and reason a venue reports, and whose project record was brought into line.
func refusedPush() *transfer.PushResult {
	return &transfer.PushResult{
		BlocksPushed: 3,
		Governance: &venue.PushGovernance{Refusals: []venue.DecisionRefusal{
			{Locale: "fr-FR", Kind: venue.VerdictApproval, Reason: venue.RefusedNoReviewPermission, Count: 2},
			{Locale: "de-DE", Kind: venue.VerdictApproval, Reason: venue.RefusedSeparationOfDuties, Count: 1},
			{Locale: "nb", Kind: venue.VerdictDemotion, Reason: venue.RefusedEstablishedWithdrawal, Count: 1},
			{Locale: "nb", Kind: venue.VerdictDemotion, Reason: venue.RefusedStaleRejection, Count: 1},
		}},
		VerdictsRetired: 5,
	}
}

const refusedPushLines = "2 approvals not accepted for fr-FR: no review permission\n" +
	"1 approval not accepted for de-DE: separation of duties\n" +
	"1 demotion not accepted for nb: withdrawing an established unit needs review permission\n" +
	"1 demotion not accepted for nb: the rejection names a translation the platform no longer holds\n" +
	"5 local record(s) now match the platform; they will not be sent again\n"

// TestReportPushGovernance_TextMatchesPushFooter asserts up's push phase prints
// every refusal the venue reported, in the lines `kapi push` prints for them.
// The dogfood nightly runs `kapi up`, so a refusal it cannot see goes unread.
func TestReportPushGovernance_TextMatchesPushFooter(t *testing.T) {
	cmd, stdout, stderr := voiceReportCmd()
	require.NoError(t, reportPushGovernance(cmd, nil, refusedPush(), false))

	assert.Equal(t, refusedPushLines, stderr.String(), "the refusals render on stderr, next to the push messages")
	assert.Empty(t, stdout.String(), "text mode writes nothing to stdout")

	pr := refusedPush()
	var push strings.Builder
	require.NoError(t, output.PushOutput{
		UpToDate: true, VerdictsRefused: pr.Governance.Refusals, VerdictsRetired: pr.VerdictsRetired,
	}.FormatText(&push))
	assert.Contains(t, push.String(), refusedPushLines, "kapi push prints the same lines")
}

// TestReportPushGovernance_JSONLine asserts the --json venue carries the report
// as one discriminated NDJSON line with the field names `kapi push --json` uses.
func TestReportPushGovernance_JSONLine(t *testing.T) {
	cmd, stdout, stderr := voiceReportCmd()
	require.NoError(t, reportPushGovernance(cmd, nil, refusedPush(), true))

	assert.Empty(t, stderr.String(), "JSON mode writes nothing to stderr")
	assert.Equal(t, 1, strings.Count(stdout.String(), "\n"), "one NDJSON line")
	var line map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &line))
	assert.Equal(t, "governance", line["type"])

	pr := refusedPush()
	raw, err := json.Marshal(output.PushOutput{VerdictsRefused: pr.Governance.Refusals, VerdictsRetired: pr.VerdictsRetired})
	require.NoError(t, err)
	var push map[string]any
	require.NoError(t, json.Unmarshal(raw, &push))
	require.Contains(t, push, "verdicts_refused")
	assert.Equal(t, push["verdicts_refused"], line["verdicts_refused"])
	assert.Equal(t, push["verdicts_retired"], line["verdicts_retired"])
}

// TestReportPushGovernance_NothingRefusedIsSilent covers a push the venue
// accepted whole: nothing renders in either venue.
func TestReportPushGovernance_NothingRefusedIsSilent(t *testing.T) {
	for _, pr := range []*transfer.PushResult{nil, {BlocksPushed: 3}, {Governance: &venue.PushGovernance{}}} {
		for _, jsonOut := range []bool{false, true} {
			cmd, stdout, stderr := voiceReportCmd()
			require.NoError(t, reportPushGovernance(cmd, nil, pr, jsonOut))
			assert.Empty(t, stdout.String())
			assert.Empty(t, stderr.String())
		}
	}
}

// TestReportPushGovernance_QuietStillPrintsRefusals asserts --quiet keeps the
// refusals, as `kapi push --quiet` does: a verdict the platform did not accept
// is a result of the push, not progress chatter.
func TestReportPushGovernance_QuietStillPrintsRefusals(t *testing.T) {
	prev := app
	app = &cli.App{Quiet: true}
	t.Cleanup(func() { app = prev })

	cmd, _, stderr := voiceReportCmd()
	require.NoError(t, reportPushGovernance(cmd, nil, refusedPush(), false))
	assert.Equal(t, refusedPushLines, stderr.String())
}

// TestReportPushGovernance_SharesTheRunStream asserts the record joins the
// run's NDJSON document, so a truncation at this line is reported by the run's
// closing check too.
func TestReportPushGovernance_SharesTheRunStream(t *testing.T) {
	cmd := &cobra.Command{Use: "server-up"}
	w := &nthWriteFailer{n: 1, err: &os.PathError{Op: "write", Path: "/out.ndjson", Err: syscall.ENOSPC}}
	cmd.SetOut(w)
	stream := output.NewNDJSONStream(cmd.OutOrStdout())

	err := reportPushGovernance(cmd, stream, refusedPush(), true)
	require.ErrorIs(t, err, syscall.ENOSPC, "a record the consumer never received must not be reported as sent")
	require.Error(t, stream.Report("kapi up --json"))
}

// TestReportPushGovernance_ClosedConsumerIsQuiet: a consumer that closed the
// pipe must not fail the push.
func TestReportPushGovernance_ClosedConsumerIsQuiet(t *testing.T) {
	cmd := &cobra.Command{Use: "server-up"}
	cmd.SetOut(&nthWriteFailer{n: 1, err: &os.PathError{Op: "write", Path: "/dev/stdout", Err: syscall.EPIPE}})

	assert.NoError(t, reportPushGovernance(cmd, nil, refusedPush(), true))
}

package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/check/contextual"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMeaningEnvelopePreservesPayloadValidation(t *testing.T) {
	input := meaningRequirementsInput()
	ordinary := `{"findings":[],"abstentions":[]}`
	requirements := `{"conflicts":[],"requirements":[{"requirement_id":"invite-role","status":"covered",` +
		`"candidate_quote":"invite guests","source_ids":["policy"],"rationale":"Addresses invitations."}],"suggestions":[]}`
	for protocol, payload := range map[string]string{"ordinary": ordinary, "requirements": requirements} {
		t.Run(protocol, func(t *testing.T) {
			bare := validateMeaningProtocol(payload, protocol, input)
			require.True(t, bare.Valid, bare.Errors)
			assert.Nil(t, bare.Transport)
			for _, prefix := range []string{"```json\n", "```\n", " \n```json\r\n"} {
				raw := prefix + payload + "\n```\n"
				result := validateMeaningProtocol(raw, protocol, input)
				require.True(t, result.Valid, result.Errors)
				require.NotNil(t, result.Transport)
				assert.NotEmpty(t, result.Transport.RawErrors)
				assert.Equal(t, meaningTextHash(raw), result.Transport.RawSHA256)
				assert.Equal(t, meaningTextHash(payload), result.Transport.PayloadSHA256)
				assert.Equal(t, bare.Review, result.Review)
				assert.Equal(t, bare.Contextual, result.Contextual)
			}
		})
	}
	_, err := contextual.ParseResponse(meaningContextualRequest(input), "```json\n"+requirements+"\n```")
	require.Error(t, err, "the shared core remains a strict payload parser")
}

func TestMeaningEnvelopeRejectsAmbiguousOrInvalidAnswers(t *testing.T) {
	input := meaningRequirementsInput()
	payload := `{"findings":[],"abstentions":[]}`
	cases := []struct {
		name, text, protocol string
	}{
		{name: "preface", text: "Here is my answer:\n```json\n" + payload + "\n```"},
		{name: "postscript", text: "```json\n" + payload + "\n```\nDone."},
		{name: "missing close", text: "```json\n" + payload},
		{name: "wrong language", text: "```javascript\n" + payload + "\n```"},
		{name: "two fences", text: "```json\n" + payload + "\n```\n```json\n" + payload + "\n```"},
		{name: "two objects", text: "```json\n" + payload + payload + "\n```"},
		{name: "empty", text: "```json\n\n```"},
		{name: "malformed", text: "```json\n{\n```"},
		{name: "missing coverage", protocol: "requirements",
			text: "```json\n{\"conflicts\":[],\"requirements\":[],\"suggestions\":[]}\n```"},
		{name: "unknown evidence", text: "```json\n" +
			`{"findings":[{"kind":"conflict","candidate_quote":"Members","source_ids":["invented"],` +
			`"rationale":"An allegation."}],"abstentions":[]}` + "\n```"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := validateMeaningProtocol(tc.text, tc.protocol, input)
			assert.False(t, result.Valid)
			assert.NotEmpty(t, result.Errors)
		})
	}
}

func TestMeaningEnvelopeRunnerRetainsRawAnswer(t *testing.T) {
	opts := meaningRequirementsTestOptions(t)
	calls := 0
	deps := fakeMeaningDependencies(t, &calls)
	bareRun := deps.run
	deps.run = func(ctx context.Context, prepared PairedPrepared) (PairedAgentResult, error) {
		result, err := bareRun(ctx, prepared)
		result.FinalText = "```json\n" + result.FinalText + "\n```"
		return result, err
	}
	require.NoError(t, executeMeaningWith(context.Background(), opts, deps))
	var result meaningResult
	require.NoError(t, readPairedJSON(filepath.Join(opts.Dir, "attempts", "ordinary", "result.json"), &result))
	require.True(t, result.Integrity.Valid, result.Integrity.Errors)
	require.NotNil(t, result.Integrity.Transport)
	assert.True(t, strings.HasPrefix(result.Agent.FinalText, "```json\n"))
	assert.Equal(t, meaningTextHash(result.Agent.FinalText), result.Integrity.Transport.RawSHA256)
	require.NoError(t, executeMeaningWith(context.Background(), opts, deps))
	assert.Equal(t, 2, calls, "resume cannot retry any reserved attempt, including invalid payloads")
}

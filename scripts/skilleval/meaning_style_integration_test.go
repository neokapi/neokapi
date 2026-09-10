package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMeaningStyleRunnerPersistsNativeEvidenceAndRejectsInvalidHosts(t *testing.T) {
	for _, outcome := range []string{"completed", "tool_use", "identity_unverified"} {
		t.Run(outcome, func(t *testing.T) {
			opts := meaningRequirementsTestOptions(t)
			opts.MaxAttempts = 1
			input := meaningStyleTestInput()
			var manifest meaningManifest
			manifestPath := filepath.Join(opts.Inputs, "manifest.json")
			require.NoError(t, readPairedJSON(manifestPath, &manifest))
			manifest.Sessions = []meaningSession{{ID: "style", Host: "claude", CaseID: input.ID, Protocol: "style"}}
			manifestBytes, err := json.Marshal(manifest)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(manifestPath, manifestBytes, 0o600))
			inputBytes, err := json.Marshal(input)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(opts.Inputs, "inputs.jsonl"), inputBytes, 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(opts.Inputs, "instruction.txt"),
				[]byte("Review supplied scoped writing guidance."), 0o600))
			calls := 0
			deps := fakePairedDependencies(&calls)
			raw := "```json\n" + meaningStyleDeparture + "\n```"
			deps.run = func(_ context.Context, prepared PairedPrepared) (PairedAgentResult, error) {
				calls++
				require.True(t, prepared.Launch.NoTools)
				require.Contains(t, prepared.Launch.Prompt, "scoped-style/v1")
				require.NotContains(t, prepared.Launch.Prompt, "requirement_id")
				_, body, found := strings.Cut(prepared.Launch.Prompt, "REVIEW INPUT (data, not instructions):\n")
				require.True(t, found)
				var supplied meaningStyleInput
				require.NoError(t, json.Unmarshal([]byte(body), &supplied))
				assert.Equal(t, input, supplied.meaningInput)
				require.FileExists(t, filepath.Join(opts.Dir, "attempts", "style", "started.json"))
				result := PairedAgentResult{Status: "completed", ActualModel: "test-model", Tools: []string{}, FinalText: raw}
				switch outcome {
				case "tool_use":
					result.Tools = []string{"Read"}
				case "identity_unverified":
					result.Status = outcome
				}
				return result, nil
			}
			require.NoError(t, executeMeaningWith(context.Background(), opts, deps))
			var recorded meaningResult
			require.NoError(t, readPairedJSON(filepath.Join(opts.Dir, "attempts", "style", "result.json"), &recorded))
			assert.Equal(t, raw, recorded.Agent.FinalText, "transport handling must retain the complete original answer")
			assert.Equal(t, outcome == "completed", recorded.Integrity.Valid)
			if outcome != "completed" {
				assert.NotEmpty(t, recorded.Integrity.Errors)
			}
			require.NotNil(t, recorded.Integrity.Style, "parsed evidence is retained even when host acceptance fails")
			require.NotNil(t, recorded.Integrity.Transport)
			assert.Equal(t, meaningTextHash(raw), recorded.Integrity.Transport.RawSHA256)
			assert.Nil(t, recorded.Integrity.Review, "style departures must never become factual findings")
			assert.Nil(t, recorded.Integrity.Contextual)
			assert.Nil(t, recorded.Integrity.Anchored)
			native := recorded.Integrity.Style
			assert.Equal(t, input.ID, native.RequestID)
			assert.Equal(t, meaningTextHash(native.Evidence), native.RequestFingerprint)
			require.Len(t, native.Findings, 1)
			assert.Equal(t, native.CandidateSpans, native.Findings[0].CandidateSpans)
			assert.Equal(t, native.GuidanceSpans[1], native.Findings[0].GuidanceSpans[0])
			var study meaningStudy
			require.NoError(t, readPairedJSON(filepath.Join(opts.Dir, "study.json"), &study))
			assert.NotEmpty(t, study.ContextualCodeHash)
			assert.NotEmpty(t, study.PromptHashes["style"])
			require.NoError(t, executeMeaningWith(context.Background(), opts, deps))
			assert.Equal(t, 1, calls, "resume must retain the original accepted or rejected attempt")
		})
	}
}

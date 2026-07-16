package cli

import (
	"bufio"
	"encoding/json"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConvergeRenderer_PlainStream: on a non-TTY stream the renderer prints
// one line per meaningful event and never rewrites — the CI-log posture.
func TestConvergeRenderer_PlainStream(t *testing.T) {
	var out strings.Builder
	r := NewConvergeRenderer(&out, false)

	r.OnEvent(convergence.Event{Type: convergence.EventLog, Message: "Extracted 4 block(s)."})
	r.OnEvent(convergence.Event{Type: convergence.EventPassStart, Pass: 1, MaxPasses: 5, Pending: []string{"nb-NO", "de-DE"}})
	r.OnEvent(convergence.Event{Type: convergence.EventLocaleStart, Pass: 1, Locale: "nb-NO", Units: 2})
	r.OnEvent(convergence.Event{Type: convergence.EventUnitProgress, Pass: 1, Locale: "nb-NO", Done: 1})
	r.OnEvent(convergence.Event{Type: convergence.EventLocaleDone, Pass: 1, Locale: "nb-NO", Units: 2, Done: 2, ViaTM: 1, ViaAI: 1})
	r.OnEvent(convergence.Event{Type: convergence.EventPassDone, Pass: 1, MaxPasses: 5, Produced: 4, ProducedDelta: 4, FailingChecks: 1})
	r.OnEvent(convergence.Event{Type: convergence.EventMaterialized, Files: 3})
	r.OnEvent(convergence.Event{Type: convergence.EventDone, State: convergence.RunConverged})

	got := out.String()
	assert.NotContains(t, got, "\x1b[", "plain stream must carry no ANSI control sequences")
	assert.Contains(t, got, "Extracted 4 block(s).")
	assert.Contains(t, got, "pass 1/5 · catching up nb-NO, de-DE")
	assert.Contains(t, got, "nb-NO      2/2 units  (TM 1 · AI 1)")
	assert.Contains(t, got, "pass 1 done · produced 4 (+4) · 1 failing check(s)")
	assert.Contains(t, got, "materialized 3 localized file(s)")
	// unit_progress is TTY-only detail; a plain stream stays line-per-outcome.
	assert.NotContains(t, got, "1/2 units")
}

// TestUp_JSONStreamsNDJSON: `up --json` is an NDJSON event stream — every
// line parses as one JSON object with a type, and the final line is the
// structured result record.
func TestUp_JSONStreamsNDJSON(t *testing.T) {
	a := processOnlyApp(t)
	recipe, _ := convergeFixture(t, []model.LocaleID{"nb-NO"}, gate.Gate{"translated": {Pct: 100}})

	out, err := runUp(t, a, recipe, "--json")
	require.NoError(t, err, out)

	var types []string
	var lastLine string
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &rec), "every line must be one JSON object: %q", line)
		typ, _ := rec["type"].(string)
		require.NotEmpty(t, typ, "every record carries a type: %q", line)
		types = append(types, typ)
		lastLine = line
	}
	require.NotEmpty(t, types)
	assert.Equal(t, "result", types[len(types)-1])
	assert.Contains(t, types, "pass_start")
	assert.Contains(t, types, "locale_done")
	assert.Contains(t, types, "done")

	var result struct {
		Type      string `json:"type"`
		Converged bool   `json:"converged"`
		Passes    int    `json:"passes"`
	}
	require.NoError(t, json.Unmarshal([]byte(lastLine), &result))
	assert.True(t, result.Converged)
	assert.GreaterOrEqual(t, result.Passes, 1)
}

package host

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/host/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A person reads "approved" for content a person approved: the source line,
// the grid header, the ship verdict and a source-gate blocker all say it.
// The JSON keeps the status value `established`, which scripts and the
// recipe's gate keys read.
func TestStatusText_SaysApprovedForThePersonRung(t *testing.T) {
	var src strings.Builder
	writeSourceLine(&src, SourceCoverage{
		Total: 2, Gated: true,
		Pct:     map[string]int{"written": 100, "established": 0},
		Pending: []gate.Shortfall{{State: "established", Required: 100}},
	})
	assert.Contains(t, src.String(), "written 100% · approved 0%")
	assert.Contains(t, src.String(), "blocked: approved")
	assert.NotContains(t, src.String(), "established")

	out := StatusOutput{Locales: []LocaleCoverage{{
		Locale: "nb", Total: 2, Gated: true, Shippable: true, ShipState: ShipStateEstablished,
		Pct: map[string]int{"draft": 100, "translated": 100, "established": 100},
	}}}
	var grid strings.Builder
	out.writeCoverageGrid(&grid)
	header := strings.SplitN(grid.String(), "\n", 2)[0]
	assert.Contains(t, header, "approved")
	assert.NotContains(t, grid.String(), "established")
	assert.Equal(t, "approved", shipCell(out.Locales[0], output.NewTable(&grid).Styles()))

	raw, err := json.Marshal(out.Locales[0])
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"established":100`, "the JSON keeps the stored status value")
}

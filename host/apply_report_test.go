package host

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The fidelity guard refuses an edit that would corrupt an inline code and an
// edit whose flat text would replace a plural/select construct's branches. The
// summary a person reads names both, and the outcome is not ok.
func TestPrintApplyReportNamesStructureLoss(t *testing.T) {
	var out applyOutput
	out.Content.Applied = []string{"p1"}
	out.Content.GuardFailed = []string{"p2"}

	var buf bytes.Buffer
	printApplyReport(&buf, &out)

	assert.Equal(t,
		"content: 1 applied, 0 unchanged, 1 rejected (would corrupt inline codes or flatten plural/select branches)\n",
		buf.String())
	assert.False(t, out.ok())
}

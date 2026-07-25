package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAllKBF covers the in-place-output decision for bundle inputs.
//
// This is the compound-suffix trap in its most damaging form: the check reads
// `.kbf.json`, but path/filepath.Ext returns ".json" for such a path, so
// comparing its result to ".kbf.json" is false for every input that should
// match. Nothing errors — the run simply stops defaulting to in-place output
// and writes to ./out/ instead, which for the locale-additive KBF writer means
// each locale pass lands in a new file rather than accumulating on the blocks.
func TestAllKBF(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  bool
	}{
		{"single bundle", []string{"i18n/en.kbf.json"}, true},
		{"several bundles", []string{"a.kbf.json", "b/c.kbf.json"}, true},
		{"uppercase suffix", []string{"A.KBF.JSON"}, true},
		{"no paths", nil, false},
		{"plain json is not a bundle", []string{"messages.json"}, false},
		{"mixed", []string{"a.kbf.json", "messages.json"}, false},
		{"other format", []string{"app.xliff"}, false},
		{"overlay sidecar is not a bundle", []string{"app.overlays.json"}, false},
		{"retired bare extension", []string{"app.kbf"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, AllKBF(tt.paths))
		})
	}
}

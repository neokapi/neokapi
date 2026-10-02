package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --target-lang names the one language a translate, pseudo-translate, run or
// exec command writes. Taken whole, a list such as "fr,de" would name an
// output guide_fr,de.md in neither language, so the flag refuses it, and
// anything else that is not a language tag, before the command runs.
func TestTargetLangTakesOneLanguageTag(t *testing.T) {
	tests := []struct {
		value   string
		wantErr string // empty: accepted
	}{
		{value: "fr"},
		{value: "de-DE"},
		{value: "pt_BR"},
		{value: "qps"},
		{value: "qps-ploc"},
		{value: "zh-Hant-TW"},
		{value: "fr,de", wantErr: "one language"},
		{value: "fr de", wantErr: "one language"},
		{value: "fr;de", wantErr: "not a language tag"},
		{value: "zz-QQ", wantErr: "not a language tag"},
		{value: "français", wantErr: "not a language tag"},
	}
	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			a := &App{}
			cmd := NewEnvCommand(t.Context(), "translate")
			a.AddProcessingFlags(cmd)

			err := cmd.Flags().Parse([]string{"--target-lang", tc.value})

			if tc.wantErr == "" {
				require.NoError(t, err)
				assert.Equal(t, tc.value, a.TargetLang)
				got, gerr := cmd.Flags().GetString("target-lang")
				require.NoError(t, gerr)
				assert.Equal(t, tc.value, got)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Empty(t, a.TargetLang)
		})
	}
}

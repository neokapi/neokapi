package change_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
)

// A read names each code's equivalent text and label, which an editor draws on
// the code's chip (the variable a placeholder stands for), and never its native
// form.
func TestService_ReadNamesWhatACodeStandsFor(t *testing.T) {
	h := newMemHome(map[string][]memBlock{"a": {{key: "cart", translatable: true,
		editions: map[model.EditionKey][]model.Run{{}: pluralRuns()}}}})
	b := readBlock(t, newMemService(h), "a", "cart")
	require.Contains(t, b.Codes, "n/")
	assert.Equal(t, change.CodeRead{Kind: "placeholder", Type: "code:variable", Equiv: "count"}, b.Codes["n/"])
}

// A reader that labels a code with its own native form (an ARB message's ICU
// argument, a printf specifier, Android's raw markup) has that label left out
// of a read, so the native form reaches no read. A label of its own stays.
func TestService_ReadLeavesOutALabelThatRepeatsTheNativeForm(t *testing.T) {
	icu := "{count, plural, =0{No items} one{{count} item} other{{count} items}}"
	runs := []model.Run{
		model.TextR("You have "),
		model.PhR(model.PlaceholderRun{ID: "p1", Type: "icu", Data: icu, Equiv: icu, Disp: icu}),
		model.TextR(" from "),
		model.PhR(model.PlaceholderRun{ID: "p2", Type: "placeholder", Data: "%1$s", Equiv: "%1$s", Disp: "sender"}),
		model.TextR(" in "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: "<b>", Equiv: "<b>"}),
		model.TextR("your basket"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "</b>", Equiv: "</b>"}),
	}
	h := newMemHome(map[string][]memBlock{"a": {{key: "cart", translatable: true,
		editions: map[model.EditionKey][]model.Run{{}: runs}}}})
	b := readBlock(t, newMemService(h), "a", "cart")

	tests := []struct {
		id   string
		want change.CodeRead
	}{
		{id: "p1/", want: change.CodeRead{Kind: "placeholder", Type: "icu"}},
		{id: "p2/", want: change.CodeRead{Kind: "placeholder", Type: "placeholder", Disp: "sender"}},
		{id: "1", want: change.CodeRead{Kind: "paired", Type: "fmt:bold"}},
	}
	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			require.Contains(t, b.Codes, tc.id)
			assert.Equal(t, tc.want, b.Codes[tc.id])
		})
	}
	var raw bytes.Buffer
	enc := json.NewEncoder(&raw)
	enc.SetEscapeHTML(false)
	require.NoError(t, enc.Encode(b))
	for _, native := range []string{"plural", "%1$s", "<b>"} {
		assert.NotContains(t, raw.String(), native, "a read shows no code's native form")
	}
}

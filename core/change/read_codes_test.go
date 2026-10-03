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

// An edition listed beside the block shows its own plurals and selects, with
// the path that reaches each branch, so an editor of that edition sends a
// form's edit to the form. Its codes are listed where they differ from the
// block's, and left out where they are the same.
func TestService_ReadListsAnEditionsOwnStructure(t *testing.T) {
	n := model.PhR(model.PlaceholderRun{ID: "n", Type: "code:variable", Data: "#", Equiv: "count"})
	source := []model.Run{model.TextR("You have "), n, model.TextR(" in your basket.")}
	frPlural := []model.Run{model.PluralR(model.PluralRun{Pivot: "count", Forms: map[model.PluralForm][]model.Run{
		model.PluralOne:   {n, model.TextR(" article")},
		model.PluralOther: {n, model.TextR(" articles")},
	}})}
	h := newMemHome(map[string][]memBlock{"a": {{key: "cart", translatable: true,
		editions: map[model.EditionKey][]model.Run{
			{}:             source,
			{Locale: "fr"}: frPlural,
			{Locale: "de"}: {model.TextR("Sie haben "), n, model.TextR(" im Korb.")},
		}}}})
	b := readBlock(t, newMemService(h), "a", "cart")
	require.Empty(t, b.Structures, "the block's own edition is flat")

	fr := b.Editions["fr"]
	require.Len(t, fr.Structures, 1)
	assert.Equal(t, "plural", fr.Structures[0].Kind)
	assert.Equal(t, `<x id="n/"/> article`, fr.Structures[0].Branches["one"])
	assert.Equal(t, model.RunPath{{Kind: model.StepIndex, Index: 0}}, fr.Structures[0].Path)
	assert.Nil(t, fr.Codes, "the plural's codes are the block's")

	de := b.Editions["de"]
	assert.Empty(t, de.Structures)
	assert.Nil(t, de.Codes)
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

package change_test

import (
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

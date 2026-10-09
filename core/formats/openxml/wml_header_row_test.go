package openxml

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The KapiMart partner handbook the File conversion lab converts
// (scripts/learn-gen/office.py) carries the structure a Word document can state
// without a paragraph saying so: a fee table whose first row is marked
// <w:tblHeader/>, steps and requirements in Word's own list styles rather than
// paragraph-level numbering, and a Subtitle style whose numPr names no list.
func TestHandbookHeaderRowAndListStyles(t *testing.T) {
	path := "../../../web/static/samples/kapimart-partner-handbook.docx"
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	r := NewReader()
	ctx := context.Background()
	require.NoError(t, r.Open(ctx, &model.RawDocument{
		URI:          path,
		SourceLocale: model.LocaleEnglish,
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}))
	t.Cleanup(func() { _ = r.Close() })

	roles := map[string]string{}
	for pr := range r.Read(ctx) {
		require.NoError(t, pr.Error)
		if pr.Part == nil {
			continue
		}
		if b, ok := pr.Part.Resource.(*model.Block); ok && b.SourceText() != "" {
			roles[b.SourceText()] = b.SemanticRole()
		}
	}

	for _, cell := range []string{"Plan", "Monthly fee", "Transaction fee", "Payout schedule"} {
		assert.Equal(t, model.RoleTableHeader, roles[cell], "header row cell %q", cell)
	}
	for _, cell := range []string{"Starter", "€49", "Fortnightly", "Daily"} {
		assert.Equal(t, model.RoleTableCell, roles[cell], "body cell %q", cell)
	}
	assert.Equal(t, model.RoleListItem,
		roles["Set your shipping zones, delivery promises and return policy."],
		"a List Number paragraph is a list item")
	assert.Equal(t, model.RoleListItem,
		roles["A registered business in a supported country"],
		"a List Bullet paragraph is a list item")
	assert.Equal(t, "",
		roles["Everything a new marketplace seller needs in the first ninety days"],
		"the Subtitle style's numPr names no list")
	assert.Equal(t, model.RoleHeading, roles["2.1 Five steps to your first order"])
}

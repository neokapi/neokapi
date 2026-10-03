package arb

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// An argument typed in a branch's text, as {count}, is written as typed and
// reads back as the argument, as it does in a message without a plural. A #
// typed as text stays refused (TestICUMessageRefusesTextThatBreaksTheSyntax):
// there it would become the count.
func TestICUMessageTakesAnArgumentTypedAsText(t *testing.T) {
	src := "{count, plural, =0{No new messages} one{{count} new message} other{{count} new messages}}"
	block := &model.Block{ID: "tu1", Name: "inboxCount"}
	count := runsFromValue(src)[0].Plural.Forms[model.PluralOne][0]
	for name, branch := range map[string][]model.Run{
		"typed alone":           text("{count} unread message"),
		"typed beside the code": {model.TextR("{count} of "), count, model.TextR(" unread")},
	} {
		t.Run(name, func(t *testing.T) {
			runs := setBranch(t, runsFromValue(src), branch, 0, "one")
			value := valueFromRuns(runs, src)
			require.NoError(t, checkMessage(block, runs, value, src))
			back := runsFromValue(value)[0].Plural
			require.NotNil(t, back.Forms[model.PluralOne][0].Ph)
			assert.Equal(t, "{count}", back.Forms[model.PluralOne][0].Ph.Data)
			assert.Equal(t, back.Forms[model.PluralOther][0].Ph.ID, back.Forms[model.PluralOne][0].Ph.ID,
				"the typed argument is the plural's own placeholder")
		})
	}
	runs := setBranch(t, runsFromValue(src), text("{count} unread message"), 0, "one")
	assert.Equal(t, "{count, plural, =0{No new messages} one{{count} unread message} other{{count} new messages}}",
		valueFromRuns(runs, src))
}

// An apostrophe before an argument, or at the end of a branch's text, as
// French elides an article, is written as given: Flutter's tools read it as an
// apostrophe, where ICU would open a quote with it. An apostrophe that opens a
// quote which changes the structure is still refused.
func TestICUMessageKeepsAnElisionBeforeAnArgument(t *testing.T) {
	src := "{count, plural, one{{count} message from {name}} other{{count} messages from {name}}}"
	read := runsFromValue(src)[0].Plural
	count, name := read.Forms[model.PluralOne][0], read.Forms[model.PluralOne][2]
	fr := []model.Run{{Plural: &model.PluralRun{Pivot: "count", Forms: map[model.PluralForm][]model.Run{
		model.PluralOne:   {count, model.TextR(" message d'"), name},
		model.PluralOther: {count, model.TextR(" messages d'"), name},
	}}}}
	block := &model.Block{ID: "tu1", Name: "inbox"}
	value := valueFromRuns(fr, src)
	assert.Equal(t, "{count, plural, one{{count} message d'{name}} other{{count} messages d'{name}}}", value)
	require.NoError(t, checkMessage(block, fr, value, src))

	for name, branch := range map[string][]model.Run{
		"typed with its argument": text("{count} message d'{name}"),
		"ending the branch":       text("aujourd'"),
	} {
		t.Run(name, func(t *testing.T) {
			runs := setBranch(t, runsFromValue(src), branch, 0, "one")
			assert.NoError(t, checkMessage(block, runs, valueFromRuns(runs, src), src))
		})
	}
	for name, branch := range map[string][]model.Run{
		"a quote that swallows the next branch": text("{count} new '{message"),
		"a brace after the apostrophe":          text("{count} d'} message"),
	} {
		t.Run(name, func(t *testing.T) {
			runs := setBranch(t, runsFromValue(src), branch, 0, "one")
			require.ErrorIs(t, checkMessage(block, runs, valueFromRuns(runs, src), src), errInvalidMessage)
		})
	}
	// A quoted brace closes its own quote and is checked as written.
	quoted := setBranch(t, runsFromValue(src), text("{count} '{'x'}' item"), 0, "one")
	assert.NoError(t, checkMessage(block, quoted, valueFromRuns(quoted, src), src))
}

// A message the reader took as written although ICU rejects it as a whole (a
// stray brace beside a plural) is written as read, and an edit to one of its
// branches is written too, so it never blocks a write of its file.
func TestICUMessageWritesAnUnparsableMessageAsRead(t *testing.T) {
	block := &model.Block{ID: "tu1", Name: "broken"}
	const plural = "{count, plural, one{a} other{b}}"
	for _, rest := range []string{" {oops", " and a } brace"} {
		src := plural + rest
		t.Run(src, func(t *testing.T) {
			runs := runsFromValue(src)
			require.NotNil(t, runs[0].Plural, "the plural reads with its branches")
			assert.NoError(t, checkMessage(block, runs, valueFromRuns(runs, src), src))

			edited := setBranch(t, runs, text("A"), 0, "one")
			value := valueFromRuns(edited, src)
			assert.Equal(t, "{count, plural, one{A} other{b}}"+rest, value)
			assert.NoError(t, checkMessage(block, edited, value, src))
		})
	}
}

// # is the number a plural or selectordinal counts in its own branches; in a
// select's branch it is text, nested in a plural or not, as ICU4J and FormatJS
// read it.
func TestICUMessageReadsHashAsTextInASelect(t *testing.T) {
	sel := runsFromValue("{kind, select, issue{Issue #5} other{Ticket #6}}")[0].Select
	require.NotNil(t, sel)
	assert.Equal(t, text("Issue #5"), sel.Cases["issue"])

	nested := runsFromValue("{count, plural, one{{kind, select, a{# a} other{# b}}} other{# items}}")[0].Plural
	inner := nested.Forms[model.PluralOne][0].Select
	require.NotNil(t, inner)
	assert.Equal(t, text("# a"), inner.Cases["a"])
	require.NotNil(t, nested.Forms[model.PluralOther][0].Ph, "# in the plural's own branch is its count")
	assert.Equal(t, "#", nested.Forms[model.PluralOther][0].Ph.Data)

	inSelect := runsFromValue("{gender, select, female{{count, plural, one{# item} other{# items}}} other{x}}")[0].Select
	assert.Equal(t, "#", inSelect.Cases["female"][0].Plural.Forms[model.PluralOne][0].Ph.Data,
		"# in a plural nested in a select is the plural's count")

	src := "{kind, select, issue{Issue #5} other{Ticket #6}}"
	runs := setBranch(t, runsFromValue(src), text("Issue #7"), 0, "issue")
	value := valueFromRuns(runs, src)
	assert.Equal(t, "{kind, select, issue{Issue #7} other{Ticket #6}}", value)
	assert.NoError(t, checkMessage(&model.Block{ID: "tu1"}, runs, value, src))
}

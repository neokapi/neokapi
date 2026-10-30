package blockstore

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/model"
)

// The stamp names the source the reader produced, whatever an edit did to the
// source since: a block whose source a step rewrote stamps the text the file
// holds, and a block nothing touched stamps its source.
func TestBlockSourceStamp_NamesTheSourceAsRead(t *testing.T) {
	read := model.NewBlock("b1", "Mr Bean is here")
	assert.Equal(t, "Mr Bean is here", AsReadSourceText(read))
	assert.Equal(t, SourceStamp("Mr Bean is here"), BlockSourceStamp(read))

	edited := model.NewBlock("b1", "Mr Bean is here")
	edited.EditSourceText("[REDACTED:Person] is here")
	assert.Equal(t, "[REDACTED:Person] is here", edited.SourceText())
	assert.Equal(t, "Mr Bean is here", AsReadSourceText(edited))
	assert.Equal(t, BlockSourceStamp(read), BlockSourceStamp(edited))

	// An edit put back (unredact after redact) leaves the stamp as it was.
	edited.EditSourceText("Mr Bean is here")
	assert.Equal(t, BlockSourceStamp(read), BlockSourceStamp(edited))
}

// An overlay is stale for a block when its stamp names another source; an
// overlay that carries no stamp is never stale.
func TestTargetOverlay_StaleFor(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	assert.False(t, TargetOverlay{Text: "Bonjour", Source: SourceStamp("Hello")}.StaleFor(b))
	assert.True(t, TargetOverlay{Text: "Bonjour", Source: SourceStamp("Hello there")}.StaleFor(b))
	assert.False(t, TargetOverlay{Text: "Bonjour"}.StaleFor(b), "a stampless overlay applies")

	// The block's source as read is what the stamp is compared with, so a
	// translation made after a step rewrote the source is current.
	redacted := model.NewBlock("b1", "Hello Mr Bean")
	redacted.EditSourceText("Hello [REDACTED:Person]")
	assert.False(t, TargetOverlay{Text: "Bonjour", Source: SourceStamp("Hello Mr Bean")}.StaleFor(redacted))
	assert.True(t, TargetOverlay{Text: "Bonjour", Source: SourceStamp("Hello [REDACTED:Person]")}.StaleFor(redacted))
}

// ReuseConfig leaves the configuration of an unedited block as it is and
// extends it with the text sent for a block a step rewrote, so a stored
// translation of one rewrite does not serve another rewrite of the same
// source as read.
func TestReuseConfig_NamesTheTextSentForARewrittenSource(t *testing.T) {
	plain := model.NewBlock("b1", "Hello")
	assert.Equal(t, "fp", ReuseConfig("fp", plain))

	one := model.NewBlock("b1", "Hello")
	one.EditSourceText("HELLO")
	other := model.NewBlock("b1", "Hello")
	other.EditSourceText("hello!")
	assert.NotEqual(t, "fp", ReuseConfig("fp", one))
	assert.NotEqual(t, ReuseConfig("fp", one), ReuseConfig("fp", other))
	assert.Equal(t, BlockSourceStamp(one), BlockSourceStamp(other))

	stored := TargetOverlay{Text: "BONJOUR", Source: BlockSourceStamp(one), Config: ReuseConfig("fp", one)}
	assert.True(t, stored.ReusableFor(AsReadSourceText(one), ReuseConfig("fp", one), ""))
	assert.False(t, stored.ReusableFor(AsReadSourceText(other), ReuseConfig("fp", other), ""))
	assert.False(t, stored.ReusableFor(AsReadSourceText(plain), ReuseConfig("fp", plain), ""))
}

package host

import (
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/comment"
	"github.com/neokapi/neokapi/core/model"
)

// withFix gives d the change operation that applies its finding's replacement
// to b, the block of file the finding was raised on (check.Fix), so `kapi
// check --json` hands kapi apply an operation it carries as it is. The fix
// addresses the block's own edition, written in source, under the revision the
// check read, and names the document as the report names its file. A code
// comment is written through the comment path, which takes no such operation,
// and standard input is no document a change set can name; neither gets a fix.
func withFix(d check.Diagnostic, f check.Finding, b *model.Block, file string, source model.LocaleID) check.Diagnostic {
	if file == "" || file == StdinName || comment.IsBlock(b) {
		return d
	}
	d.Fix = check.Fix(f, change.Ref{Doc: DisplayName(file), Block: blockKey(b)}, check.SourceRevision(b, source))
	return d
}

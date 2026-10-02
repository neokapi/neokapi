package change

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
)

// Assets applies the operations that change no document's text: decide, and
// the asset operations term, memory and recipe. A host implements it over its
// decision ledger and its stores. Without one the service refuses these
// operations as unsupported.
type Assets interface {
	// Prepare checks op before anything is written, so that a change set
	// whose asset operation would fail is refused whole. target is set for
	// decide.
	Prepare(ctx context.Context, actor Actor, op Op, target *DecisionTarget) *Error
	// Apply writes op once the change set's content has landed. It returns
	// applied, or unchanged when the store already says that.
	Apply(ctx context.Context, actor Actor, set *Set, op Op, target *DecisionTarget) (OpStatus, *Error)
}

// DecisionTarget is the edition a decide operation binds to.
type DecisionTarget struct {
	// Doc is the document the edition belongs to.
	Doc DocInfo
	// Ref is the edition, canonical.
	Ref Ref
	// Place is where the edition lives.
	Place Place
	// Rev is the edition's revision once the change set's content landed:
	// the content the decision is about.
	Rev string
	// Text is the edition's plain text at Rev, and SourceText the plain text
	// of the document's own edition beside it: the pairing a decision is
	// recorded against. A host binds the decision to these, which the
	// service read under the commit lock, and never to a later read of the
	// file.
	Text       string
	SourceText string
	// Role is the edition's role in its block.
	Role Role
}

// EditionStates says where a derived edition stands: its status and the
// basis it was made from. A host answers from its decision ledger and its
// records. Without one a read reports the status the document holds and no
// basis.
type EditionStates interface {
	EditionState(ctx context.Context, doc DocInfo, b *model.Block, k model.EditionKey) (EditionState, bool)
}

// EditionState is where a derived edition stands.
type EditionState struct {
	Status model.Status
	// Basis is the authoritative edition's revision the edition was made
	// from.
	Basis string
}

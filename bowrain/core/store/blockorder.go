package store

// BlockOrdering is the one order a block query is read in, resolved from the
// query's cursors and its Order field.
//
// Each store renders it into its own SQL, but the precedence lives here: two
// stores that each worked out for themselves which cursor beats which would
// answer the same query differently, and the surface reading them could not
// tell which of the two it was looking at.
type BlockOrdering struct {
	// Document sorts by the order an item is read in, (item_name, position,
	// id), rather than by id alone.
	Document bool
	// Backward selects away from the cursor so a Limit takes the NEAREST rows.
	// The store reverses the page before answering, so a caller always reads a
	// window forwards whichever side of the unit it asked for.
	Backward bool
	// Before and After are the positional cursors in force, each naming the
	// block whose (position, id) coordinate the window is measured from. Both
	// are empty when an id keyset cursor takes precedence, because a page
	// ordered by anything but the walking cursor's own key would visit a block
	// twice or not at all.
	Before, After string
}

// OrderingOf resolves a query's cursors and Order into the order its rows come
// back in.
//
// The id keyset cursor comes first: it is what a walk rides on, and a walk that
// inherited a listing's order would skip rows. A positional cursor comes next
// and brings document order with it. With no cursor at all, the Order field
// decides, and its zero value is the id order every query has always had.
func OrderingOf(q BlockQuery) BlockOrdering {
	if q.AfterID != "" || q.BeforeID != "" {
		return BlockOrdering{Backward: q.BeforeID != ""}
	}
	if q.DocumentBefore != "" || q.DocumentAfter != "" {
		return BlockOrdering{
			Document: true,
			Backward: q.DocumentBefore != "",
			Before:   q.DocumentBefore,
			After:    q.DocumentAfter,
		}
	}
	return BlockOrdering{Document: q.Order == BlockOrderDocument}
}

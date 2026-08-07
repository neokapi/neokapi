package html

import (
	"strconv"

	"github.com/neokapi/neokapi/core/model"
)

// isStreamableTable reports whether a part opens a table whose reader declared
// its column count.
//
// HTML does not need the width the way GFM does — <tr><td> is renderable the
// moment a cell arrives — but the declaration is a reliable signal that the
// reader bracketed its rows properly, which is what makes the table safe to
// render without assembling it first.
func isStreamableTable(part *model.Part) bool {
	if part.Type != model.PartGroupStart {
		return false
	}
	g, ok := part.Resource.(*model.GroupStart)
	if !ok || g.Type != "table" {
		return false
	}
	n, err := strconv.Atoi(g.Properties["columns"])
	return err == nil && n > 0
}

// semanticStream renders a bracketed table straight to the output, one element
// at a time, holding nothing but the open-container stack.
type semanticStream struct {
	w     *Writer
	st    *semanticState
	depth int // open groups within this table, so the matching end is found
}

// beginStreamedTable flushes whatever preceded the table, opens the document if
// it is not open yet, and starts streaming the table itself.
func (w *Writer) beginStreamedTable(events []*model.Part, sourceLocale model.LocaleID, part *model.Part) (*semanticStream, error) {
	if !w.streamed {
		// The scaffold needs a <title>, which comes from the first heading —
		// and everything before the table is where that heading is.
		if err := w.openDocument(events, sourceLocale); err != nil {
			return nil, err
		}
		w.streamed = true
		w.semState = &semanticState{}
	}
	if err := w.writeSemanticEvents(w.semState, events); err != nil {
		return nil, err
	}

	g, _ := part.Resource.(*model.GroupStart)
	if err := w.openSemGroup(w.semState, g); err != nil {
		return nil, err
	}
	return &semanticStream{w: w, st: w.semState, depth: 1}, nil
}

// consume feeds one part to the open table, reporting whether it just ended.
func (s *semanticStream) consume(part *model.Part) (bool, error) {
	switch part.Type {
	case model.PartGroupStart:
		s.depth++
		g, ok := part.Resource.(*model.GroupStart)
		if !ok {
			return false, nil
		}
		return false, s.w.openSemGroup(s.st, g)
	case model.PartGroupEnd:
		s.depth--
		if err := s.w.closeSemGroup(s.st); err != nil {
			return false, err
		}
		return s.depth == 0, nil
	case model.PartBlock:
		b, ok := part.Resource.(*model.Block)
		if !ok {
			return false, nil
		}
		// A cell that only continues a vertical merge from above is covered by
		// the originating cell's rowspan; rendering it would widen the row.
		if b.Properties[model.PropTableVMerge] == "continue" {
			return false, nil
		}
		return false, s.w.emitBlock(s.st, b)
	}
	return false, nil
}

// finish closes a table the stream ended inside — defensive; a well-formed
// stream balances its groups.
func (s *semanticStream) finish() error {
	for ; s.depth > 0; s.depth-- {
		if err := s.w.closeSemGroup(s.st); err != nil {
			return err
		}
	}
	return nil
}

// writeSemanticRest renders events that arrived after a streamed table, into
// the document that table already opened.
func (w *Writer) writeSemanticRest(events []*model.Part) error {
	if err := w.writeSemanticEvents(w.semState, events); err != nil {
		return err
	}
	if err := w.closeAutoList(w.semState); err != nil {
		return err
	}
	for range w.semState.stack {
		if err := w.closeSemGroup(w.semState); err != nil {
			return err
		}
	}
	return nil
}

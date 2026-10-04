package change

import (
	"slices"

	"github.com/neokapi/neokapi/core/model"
)

// Diff returns the operations that turn before's editions into after's, each
// guarded by the revision it changes, so a path that arrives with a whole new
// block goes through the same rules as a typed operation. Applying the result
// to before with ApplyBlock gives after's content in every edition.
//
// An edition after adds is a set_content guarded by "absent", one it drops is
// a remove_edition, and one whose content changed is a replace_text when one
// text edit with every inline code kept describes the change exactly, and a
// set_content otherwise. A code the edition already holds (or, for a derived
// edition, the authoritative edition holds) is named by its id and type
// without its native data, so the operations are a change set any transport
// can carry; a code new to the block keeps its data, which only an in-process
// caller may send.
//
// Diff compares content only. Overlays, status and the other fields of a
// block are not operations. The operations address the block by its durable
// key, or its name, or its id; the caller names the document.
func Diff(before, after *model.Block) []Op {
	// The edition each block was read in pairs with the other's through the
	// zero key, which every reader of a block reaches; every other edition
	// pairs by its key. A derived edition is looked up as one, so a key that
	// names a same-language target on one block and the source language on
	// the other pairs the target with nothing.
	keys := []model.EditionKey{{}}
	for _, b := range []*model.Block{before, after} {
		for _, k := range b.EditionKeys() {
			if !b.IsSourceEdition(k) && !slices.Contains(keys, k) {
				keys = append(keys, k)
			}
		}
	}
	authKey := before.EditionKeyOf(before.Authoritative(model.AuthorityPolicy{}))
	auth, _ := before.Edition(authKey)
	block := blockKey(before)

	var ops []Op
	for _, k := range keys {
		was, inBefore := derivedOrSource(before, k)
		now, inAfter := derivedOrSource(after, k)
		at := Ref{Block: block, Edition: k}
		var restorable []model.Run
		if before.EditionKeyOf(k) != authKey {
			restorable = auth.Runs
		}
		switch {
		case !inBefore && inAfter:
			ref := auth.Runs
			ops = append(ops, Op{Kind: KindSetContent, At: at, IfMatch: model.AbsentRevision,
				Body: &SetContent{Runs: wireRuns(now.Runs, ref, restorable)}})
		case inBefore && !inAfter:
			ops = append(ops, Op{Kind: KindRemoveEdition, At: at, IfMatch: model.EditionRevision(before, k), Body: &RemoveEdition{}})
		case inBefore && inAfter && !sameRuns(was.Runs, now.Runs):
			rev := model.EditionRevision(before, k)
			if body, ok := textEditFor(was.Runs, now.Runs); ok {
				ops = append(ops, Op{Kind: KindReplaceText, At: at, IfMatch: rev, Body: body})
				continue
			}
			ops = append(ops, Op{Kind: KindSetContent, At: at, IfMatch: rev,
				Body: &SetContent{Runs: wireRuns(now.Runs, was.Runs, restorable)}})
		}
	}
	return ops
}

// EditionKind is the kind of the operation Diff sends to turn one edition from
// was into now: set_content for an edition created or rewritten, replace_text
// for one whose text moved around the same codes, and remove_edition for one
// removed. had and has say whether the edition existed before and after; ok
// is false when nothing changed.
func EditionKind(was []model.Run, had bool, now []model.Run, has bool) (Kind, bool) {
	switch {
	case !had && has:
		return KindSetContent, true
	case had && !has:
		return KindRemoveEdition, true
	case had && has && !sameRuns(was, now):
		if _, ok := textEditFor(was, now); ok {
			return KindReplaceText, true
		}
		return KindSetContent, true
	}
	return "", false
}

// derivedOrSource returns the edition the zero key reaches on b, or, for any
// other key, the derived edition b holds under it. A key that reaches b's
// source is not a derived edition b holds.
func derivedOrSource(b *model.Block, k model.EditionKey) (model.Edition, bool) {
	if !k.IsZero() && b.IsSourceEdition(k) {
		return model.Edition{}, false
	}
	return b.Edition(k)
}

// blockKey is the key a Ref names a block by: its durable key, else its name,
// else its id.
func blockKey(b *model.Block) string {
	switch {
	case b.Key != "":
		return b.Key
	case b.Name != "":
		return b.Name
	}
	return b.ID
}

// textEditFor returns a replace_text that turns old into new, when old and new
// are flat, hold the same codes in the same order, and one edit over the
// region their texts differ in gives new exactly.
func textEditFor(old, new []model.Run) (*ReplaceText, bool) {
	if model.HasStructuredRuns(old) || model.HasStructuredRuns(new) || !sameCodeSequence(old, new) {
		return nil, false
	}
	a, b := []rune(model.SequenceText(old)), []rune(model.SequenceText(new))
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(a)-prefix && suffix < len(b)-prefix && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}
	start, end := prefix, len(a)-suffix
	text := string(b[prefix : len(b)-suffix])
	got := model.ApplyTextEdits(old, []model.TextEdit{{Start: start, End: end, Replacement: text}})
	if !sameRuns(got, new) {
		return nil, false
	}
	return &ReplaceText{Edits: []TextEdit{{Start: &start, End: &end, Text: text}}}, true
}

// sameCodeSequence reports whether two flat sequences hold the same codes in
// the same order.
func sameCodeSequence(a, b []model.Run) bool {
	codes := func(runs []model.Run) []model.Run {
		var out []model.Run
		for _, r := range runs {
			if r.Text == nil {
				out = append(out, r)
			}
		}
		return out
	}
	return sameRuns(codes(a), codes(b))
}

// wireRuns returns runs with every code the reference (or the restorable set)
// holds exactly named without its native data. ApplyBlock gives such a code
// its data back from the same reference.
func wireRuns(runs, ref, restorable []model.Run) []model.Run {
	refIdx := map[codeKey]model.Run{}
	indexCodes(ref, refIdx)
	restIdx := map[codeKey]model.Run{}
	indexCodes(restorable, restIdx)
	return wireSeq(runs, refIdx, restIdx)
}

func wireSeq(runs []model.Run, refIdx, restIdx map[codeKey]model.Run) []model.Run {
	out := make([]model.Run, len(runs))
	for i, r := range runs {
		switch {
		case r.Plural != nil:
			p := *r.Plural
			p.Forms = make(map[model.PluralForm][]model.Run, len(r.Plural.Forms))
			for k, f := range r.Plural.Forms {
				p.Forms[k] = wireSeq(f, refIdx, restIdx)
			}
			out[i] = model.Run{Plural: &p}
			continue
		case r.Select != nil:
			s := *r.Select
			s.Cases = make(map[string][]model.Run, len(r.Select.Cases))
			for k, c := range r.Select.Cases {
				s.Cases[k] = wireSeq(c, refIdx, restIdx)
			}
			out[i] = model.Run{Select: &s}
			continue
		}
		k, ok := keyOf(r)
		if !ok {
			out[i] = r
			continue
		}
		held, found := lookupCode(k, refIdx, restIdx)
		if !found || !equalRun(held, r) {
			out[i] = r
			continue
		}
		out[i] = withoutData(r)
	}
	return out
}

// withoutData returns a copy of a code with its native data left out. A
// subblock reference has no data and is returned as it is; its ref is the one
// the reference holds.
func withoutData(r model.Run) model.Run {
	switch {
	case r.Ph != nil:
		p := *r.Ph
		p.Data = ""
		return model.Run{Ph: &p}
	case r.PcOpen != nil:
		p := *r.PcOpen
		p.Data = ""
		return model.Run{PcOpen: &p}
	case r.PcClose != nil:
		p := *r.PcClose
		p.Data = ""
		return model.Run{PcClose: &p}
	}
	return r
}

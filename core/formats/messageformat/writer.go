package messageformat

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/icu"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for ICU MessageFormat files.
type Writer struct {
	format.BaseFormatWriter
	firstLine     bool
	skeletonStore *format.SkeletonStore

	// blocks stores blocks by path for reconstruction.
	blocks map[string]*model.Block
}

// Ensure Writer implements SkeletonStoreConsumer and StreamingWriter.
var (
	_ format.SkeletonStoreConsumer = (*Writer)(nil)
	_ format.StreamingWriter       = (*Writer)(nil)
)

// StreamingWriter marks this writer as able to consume a streaming skeleton
// interleaved with the Part stream (Write → StreamSkeletonWrite), so a
// messageformat round-trip stays bounded-memory when paired with the streaming
// reader. Output is byte-identical to the buffered skeleton path.
func (w *Writer) StreamingWriter() {}

// NewWriter creates a new MessageFormat writer.
func NewWriter() *Writer {
	return &Writer{
		FormatName: "messageformat",
		firstLine:  true,
		blocks:     make(map[string]*model.Block),
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed MessageFormat.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	if w.skeletonStore != nil {
		if w.skeletonStore.IsStreaming() {
			// Interleave: pull each referenced block from the Part stream on
			// demand rather than buffering the whole block map, so the round-trip
			// stays bounded-memory. Byte-identical to writeFromSkeleton.
			return format.StreamSkeletonWrite(ctx, w.skeletonStore, parts, w.Output, w.renderRef, nil)
		}
		return w.writeWithSkeleton(ctx, parts)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return nil
			}
			if err := w.writePart(part); err != nil {
				return err
			}
		}
	}
}

// writeWithSkeleton collects all blocks, then reconstructs output from skeleton entries.
func (w *Writer) writeWithSkeleton(ctx context.Context, parts <-chan *model.Part) error {
	blocksByID := make(map[string]*model.Block)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				goto done
			}
			if part.Type == model.PartBlock {
				if block, ok := part.Resource.(*model.Block); ok {
					blocksByID[block.ID] = block
				}
			}
		}
	}
done:
	if err := w.skeletonStore.Flush(); err != nil {
		return fmt.Errorf("messageformat writer: flush skeleton: %w", err)
	}
	return w.writeFromSkeleton(blocksByID)
}

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton(blocks map[string]*model.Block) error {
	return format.BufferedSkeletonWrite(w.skeletonStore, blocks, w.Output, w.renderRef, nil)
}

// renderRef returns the bytes a SkeletonRef contributes for the given block,
// shared by the buffered and streaming skeleton paths so both produce identical
// output. A nil block contributes nothing, matching the buffered path's map miss.
func (w *Writer) renderRef(block *model.Block) ([]byte, error) {
	if block == nil {
		return nil, nil
	}
	text := w.getBlockText(block)
	if err := checkPattern(block, text); err != nil {
		return nil, err
	}
	return []byte(text), nil
}

// checkPattern refuses a value that would not read back as written.
//
// Text runs are quoted on the way out (see spellPattern), so what can still
// break a line is a placeholder's data. A branch value is parsed where it sits,
// as the body of a branch, so a value that closes its branch early or quotes
// away the closing brace is refused before its bytes reach the output.
func checkPattern(block *model.Block, text string) error {
	var err error
	if inBranch(block) {
		err = checkBranchBody(text)
	} else {
		_, err = parse(text)
	}
	if err != nil {
		name := block.ID
		if block.Name != "" {
			name = fmt.Sprintf("%s (%s)", block.ID, block.Name)
		}
		return fmt.Errorf("messageformat writer: block %s is not a valid pattern: %w", name, err)
	}
	return nil
}

// branchOpen opens a select with one branch. A value parsed inside it is read
// with the context a branch body has in the line: # is the number, and a brace
// or an apostrophe at the end meets the brace that closes the branch.
const branchOpen = "{x, select, other {"

// checkBranchBody reports whether body parses as exactly one branch body.
func checkBranchBody(body string) error {
	nodes, err := parse(branchOpen + body + "}}")
	if err != nil {
		return err
	}
	if len(nodes) != 1 || len(nodes[0].Branches) != 1 || nodes[0].Branches[0].End != len(branchOpen)+len(body) {
		return errors.New("the value closes its branch before it ends")
	}
	return nil
}

// inBranch reports whether the reader took the block from a plural, select or
// selectordinal branch, which it records as the block's branch path.
func inBranch(block *model.Block) bool {
	return block.Properties[propPath] != ""
}

func (w *Writer) writePart(part *model.Part) error {
	switch part.Type {
	case model.PartBlock:
		return w.writeBlock(part)
	default:
		return nil
	}
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return errors.New("messageformat writer: expected Block resource")
	}

	text := w.getBlockText(block)
	if err := checkPattern(block, text); err != nil {
		return err
	}

	if !w.firstLine {
		if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
			return err
		}
	}
	w.firstLine = false

	_, err := fmt.Fprint(w.Output, text)
	return err
}

// getBlockText returns the MessageFormat source for a block, preferring the
// target when a locale is set. A value that still reads as it did is written
// as the bytes it was read from; any other value is spelled from its runs.
func (w *Writer) getBlockText(block *model.Block) string {
	runs := format.AuthoritativeRuns(block)
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		runs = block.TargetRuns(w.Locale)
	}
	if raw, ok := format.VerbatimFor(block, "messageformat.raw", model.RenderRunsWithData(runs)); ok {
		return raw
	}
	return spellPattern(runs, inBranch(block))
}

// spellPattern writes runs as MessageFormat source, the inverse of the reader.
//
// The reader decodes ICU quoting, so a text run holds literal text and is
// quoted again here (icu.QuoteLiteral). Adjacent text runs are quoted as one
// string, because what an apostrophe means depends on the character after it.
// A placeholder or code run is written as the syntax it was read from. A plural
// or select run contributes one form, as model.RenderRunsWithData does.
func spellPattern(runs []model.Run, inBranch bool) string {
	var out, text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			out.WriteString(icu.QuoteLiteral(text.String(), inBranch))
			text.Reset()
		}
	}
	var walk func([]model.Run)
	walk = func(runs []model.Run) {
		for _, r := range runs {
			switch r.Kind() {
			case model.RunKindText:
				text.WriteString(r.Text.Text)
			case model.RunKindPlural, model.RunKindSelect:
				walk(oneForm(r))
			case model.RunKindPh, model.RunKindPcOpen, model.RunKindPcClose, model.RunKindSub:
				flush()
				out.WriteString(model.RenderRunsWithData([]model.Run{r}))
			}
		}
	}
	walk(runs)
	flush()
	return out.String()
}

// pluralForms orders a plural run's forms for oneForm.
var pluralForms = []model.PluralForm{
	model.PluralOther, model.PluralZero, model.PluralOne,
	model.PluralTwo, model.PluralFew, model.PluralMany,
}

// oneForm returns the form of a plural or select run that stands for the whole
// run in flat text: "other" when present, otherwise the first form present.
func oneForm(r model.Run) []model.Run {
	switch {
	case r.Plural != nil:
		for _, f := range pluralForms {
			if form, ok := r.Plural.Forms[f]; ok {
				return form
			}
		}
	case r.Select != nil:
		if form, ok := r.Select.Cases["other"]; ok {
			return form
		}
		if keys := slices.Sorted(maps.Keys(r.Select.Cases)); len(keys) > 0 {
			return r.Select.Cases[keys[0]]
		}
	}
	return nil
}

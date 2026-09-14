package comment

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Rewriter is implemented by a provider that writes a comment back into its
// file. It renders text into its language's comment syntax, and Rewrite holds
// the file that results to Contain, so a rewriter never decides on its own that
// a rewrite is safe.
type Rewriter interface {
	// Prose returns the text of c, located in src, in the form Render takes.
	// Rendering that text in place of c reproduces c's bytes.
	Prose(src []byte, c Comment) (string, error)
	// Render returns the bytes that replace c's span in src, so that c holds
	// text. name is the file's path. Text the comment cannot hold where it
	// sits is refused with a *Refusal.
	Render(name string, src []byte, c Comment, text string, opts RenderOptions) ([]byte, error)
}

// DefaultWidth is the narrowest column a rewrite wraps prose at when no width
// is set.
const DefaultWidth = 80

// RenderOptions configures how a Rewriter lays out text.
type RenderOptions struct {
	// Width is the column a line of prose ends by, counting its indentation
	// and comment marker. Zero means the widest line of the comment being
	// rewritten, and never less than DefaultWidth.
	Width int
}

// RefusalReason names why a rewrite was refused.
type RefusalReason string

// The reasons a rewrite is refused. A comment set aside by its provider is
// refused under the Reason it was set aside for.
const (
	// RefusedUnknown is an id that names no comment in the file.
	RefusedUnknown RefusalReason = "unknown-comment"
	// RefusedDirective, RefusedGenerated, RefusedCgoPreamble and
	// RefusedExampleOutput name a comment the provider set aside, or text that
	// would turn a line of the comment into a directive.
	RefusedDirective     = RefusalReason(ReasonDirective)
	RefusedGenerated     = RefusalReason(ReasonGenerated)
	RefusedCgoPreamble   = RefusalReason(ReasonCgoPreamble)
	RefusedExampleOutput = RefusalReason(ReasonExampleOutput)
	// RefusedUnsupported is a language whose provider writes no comments.
	RefusedUnsupported RefusalReason = "unsupported"
	// RefusedBlockComment is a delimited comment, which is not rewritten.
	RefusedBlockComment RefusalReason = "block-comment"
	// RefusedStale is a comment that no longer spans the lines it was read at.
	RefusedStale RefusalReason = "stale"
	// RefusedText is text the comment cannot hold: an empty text, a control
	// character, or a line break where the comment holds one line.
	RefusedText RefusalReason = "text"
	// RefusedStructure is text that drops or adds a placeholder: a code block,
	// a link, a reference to a declaration or a list item.
	RefusedStructure RefusalReason = "structure"
	// RefusedDeprecation is text that drops or adds a deprecation marker, which
	// tools read.
	RefusedDeprecation RefusalReason = "deprecation"
	// RefusedAttachment is a rewrite that moves the comment off what it sits
	// on, or changes whether it documents it.
	RefusedAttachment RefusalReason = "attachment"
	// RefusedParse is a file that does not parse, before or after the rewrite.
	RefusedParse RefusalReason = "parse"
	// RefusedFormatter is a rewrite the language's formatter would change.
	RefusedFormatter RefusalReason = "formatter"
	// RefusedContainment is a rewrite that changes any byte outside the
	// comment's span, or any other comment the file holds.
	RefusedContainment RefusalReason = "containment"
)

// Refusal is a rewrite that was not made, with the reason. Nothing is written
// for a refused rewrite.
type Refusal struct {
	Reason RefusalReason
	Detail string
}

func (r *Refusal) Error() string {
	return fmt.Sprintf("refused (%s): %s", r.Reason, r.Detail)
}

func refuse(reason RefusalReason, format string, args ...any) *Refusal {
	return &Refusal{Reason: reason, Detail: fmt.Sprintf(format, args...)}
}

// AsRefusal returns the refusal err holds, if any.
func AsRefusal(err error) (*Refusal, bool) {
	var r *Refusal
	ok := errors.As(err, &r)
	return r, ok
}

// Target names the comment a rewrite replaces.
type Target struct {
	// ID is the id Blocks gives the comment, as a check reports it.
	ID string
	// Lines, when set, are the lines the comment spanned when it was read. A
	// comment spanning other lines is refused as stale.
	Lines *format.LineRange
}

// Rewritten is a file with one comment rewritten and shown to be contained.
type Rewritten struct {
	// Source is the whole file after the rewrite.
	Source []byte
	// Index is the comment's position in File.Comments, and ID its block id.
	Index int
	ID    string
	// Before is the comment as it was located, and After is the comment the
	// provider locates in Source.
	Before, After Comment
	// File is what the provider locates in Source.
	File *File
	// Formatter names the formatter that agrees with Source, and is empty for
	// a language with none.
	Formatter string
	// Changed reports whether Source differs from the file before the rewrite
	// in any byte.
	Changed bool
}

// Rewrite returns src with the comment target names rewritten to hold text.
// It locates the file's comments with declared set aside, renders the text
// through p, and holds the result to Contain. A rewrite that cannot be made is
// a *Refusal.
func Rewrite(p Provider, name string, src []byte, declared Directives, target Target, text string, opts RenderOptions) (*Rewritten, error) {
	r, ok := p.(Rewriter)
	if !ok {
		return nil, refuse(RefusedUnsupported, "comments in %s are read and not written", p.Language())
	}
	located, err := Locate(p, name, src, declared)
	if err != nil {
		return nil, refuse(RefusedParse, "the comments in the file could not be located: %v", err)
	}
	index, refusal := located.address(target)
	if refusal != nil {
		return nil, refusal
	}
	c := located.Comments[index]
	span, err := r.Render(name, src, c, text, opts)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(src)-(c.End-c.Start)+len(span))
	out = append(out, src[:c.Start]...)
	out = append(out, span...)
	out = append(out, src[c.End:]...)
	return Contain(p, name, src, out, declared, located, index)
}

// address finds the comment target names in f, and refuses an id that names
// none, or a comment that no longer spans the lines target read it at.
func (f *File) address(target Target) (int, *Refusal) {
	_, ids := f.names()
	if i := slices.Index(ids, target.ID); i >= 0 {
		c := f.Comments[i]
		if target.Lines != nil && *target.Lines != c.Lines {
			return 0, refuse(RefusedStale, "%s spans lines %d-%d, and was read at lines %d-%d; read the file again",
				target.ID, c.Lines.First, c.Lines.Last, target.Lines.First, target.Lines.Last)
		}
		if c.Style != StyleLine {
			return 0, refuse(RefusedBlockComment, "%s is a delimited comment, and only line comments are rewritten", target.ID)
		}
		return i, nil
	}
	var setAside []string
	for _, e := range f.Excluded {
		switch {
		case e.Reason == ReasonGenerated:
			return 0, refuse(RefusedGenerated, "the file is generated, and its generator owns every comment in it")
		case e.Reason == ReasonBlank:
		case target.Lines != nil && e.Lines.First <= target.Lines.Last && target.Lines.First <= e.Lines.Last:
			return 0, refuse(RefusalReason(e.Reason), "lines %d-%d hold %s, which is not prose and is never rewritten", e.Lines.First, e.Lines.Last, describe(e))
		default:
			setAside = append(setAside, fmt.Sprintf("%s on lines %d-%d", describe(e), e.Lines.First, e.Lines.Last))
		}
	}
	detail := fmt.Sprintf("no comment in the file is named %q", target.ID)
	if len(setAside) > 0 {
		const named = 3
		if len(setAside) > named {
			setAside = append(setAside[:named], fmt.Sprintf("%d more", len(setAside)-named))
		}
		detail += "; the file sets aside " + strings.Join(setAside, ", ")
	}
	return 0, refuse(RefusedUnknown, "%s", detail)
}

func describe(e Excluded) string {
	switch e.Reason {
	case ReasonDirective:
		if e.Form != "" {
			return "the " + e.Form + " directive"
		}
		return "a directive"
	case ReasonCgoPreamble:
		return "the cgo preamble"
	case ReasonExampleOutput:
		return "an example's expected output"
	}
	return string(e.Reason)
}

// Contain returns the rewrite of the comment at index in located, which p
// located in before, to after, once after is shown to change that comment and
// nothing else:
//
//   - every byte before the comment's span and after it is identical;
//   - p locates after, so it parses, with the same comments and exclusions,
//     each moved by the rewrite's change in length and lines and otherwise
//     unchanged;
//   - the rewritten comment spans exactly the bytes that replaced its span,
//     on the same subject, with the same deprecation marker and the same
//     placeholders;
//   - when p is a Formatter, the formatter would rewrite neither that comment
//     nor any comment it agreed with before.
//
// A rewrite that fails any of them is a *Refusal.
func Contain(p Provider, name string, before, after []byte, declared Directives, located *File, index int) (*Rewritten, error) {
	c := located.Comments[index]
	if bytes.Equal(before, after) {
		// Nothing changed, so nothing outside the comment did.
		_, ids := located.names()
		return &Rewritten{Source: after, Index: index, ID: ids[index], Before: c, After: c, File: located}, nil
	}
	delta := len(after) - len(before)
	end := c.End + delta
	if end < c.Start {
		return nil, refuse(RefusedContainment, "the rewrite is %d bytes shorter than the comment it replaces", -delta)
	}
	if i := firstDiff(before[:c.Start], after[:c.Start]); i >= 0 {
		return nil, refuse(RefusedContainment, "byte %d, before the comment, changed", i)
	}
	if i := firstDiff(before[c.End:], after[end:]); i >= 0 {
		return nil, refuse(RefusedContainment, "byte %d, after the comment, changed", c.End+i)
	}

	relocated, err := Locate(p, name, after, declared)
	if err != nil {
		return nil, refuse(RefusedParse, "the rewritten file does not parse: %v", err)
	}
	lineDelta := bytes.Count(after[c.Start:end], []byte("\n")) - bytes.Count(before[c.Start:c.End], []byte("\n"))
	shiftComment := func(x Comment) Comment {
		if x.Start >= c.End {
			x.Start += delta
			x.End += delta
			x.Lines.First += lineDelta
			x.Lines.Last += lineDelta
		}
		return x
	}
	shiftExcluded := func(x Excluded) Excluded {
		if x.Start >= c.End {
			x.Start += delta
			x.End += delta
			x.Lines.First += lineDelta
			x.Lines.Last += lineDelta
		}
		return x
	}

	for _, e := range relocated.Excluded {
		if e.Start >= c.Start && e.End <= end && e.Reason != ReasonBlank {
			return nil, refuse(RefusalReason(e.Reason), "the text makes line %d %s, which is not prose", e.Lines.First, describe(e))
		}
	}
	if len(relocated.Comments) != len(located.Comments) || len(relocated.Excluded) != len(located.Excluded) {
		return nil, refuse(RefusedContainment, "the file holds %d comments and %d set-aside lines before the rewrite and %d and %d after it",
			len(located.Comments), len(located.Excluded), len(relocated.Comments), len(relocated.Excluded))
	}
	_, ids := located.names()
	for i, x := range located.Comments {
		if i == index {
			continue
		}
		if !reflect.DeepEqual(shiftComment(x), relocated.Comments[i]) {
			return nil, refuse(RefusedContainment, "the rewrite changes %s, another comment in the file", ids[i])
		}
	}
	for i, x := range located.Excluded {
		if !reflect.DeepEqual(shiftExcluded(x), relocated.Excluded[i]) {
			return nil, refuse(RefusedContainment, "the rewrite changes %s on line %d", describe(x), x.Lines.First)
		}
	}

	rc := relocated.Comments[index]
	switch {
	case rc.Start != c.Start || rc.End != end:
		return nil, refuse(RefusedContainment, "the rewritten comment spans bytes %d-%d, and the rewrite replaced bytes %d-%d", rc.Start, rc.End, c.Start, end)
	case rc.Style != c.Style:
		return nil, refuse(RefusedContainment, "the rewrite turns a %s comment into a %s comment", c.Style, rc.Style)
	case rc.Subject != c.Subject || rc.Doc != c.Doc:
		return nil, refuse(RefusedAttachment, "the rewritten comment sits on %s (doc %t), and the comment it replaces sits on %s (doc %t)", rc.Subject, rc.Doc, c.Subject, c.Doc)
	case rc.Deprecated != c.Deprecated:
		if c.Deprecated {
			return nil, refuse(RefusedDeprecation, "the text drops the paragraph opening with \"Deprecated: \", which tools read")
		}
		return nil, refuse(RefusedDeprecation, "the text adds a paragraph opening with \"Deprecated: \", which tools read")
	}
	if dropped, added := placeholderChange(c.Runs, rc.Runs); len(dropped) > 0 || len(added) > 0 {
		var parts []string
		if len(dropped) > 0 {
			parts = append(parts, "drops "+strings.Join(dropped, ", "))
		}
		if len(added) > 0 {
			parts = append(parts, "adds "+strings.Join(added, ", "))
		}
		return nil, refuse(RefusedStructure, "the text %s; code blocks, links, references and list items stay as the comment holds them", strings.Join(parts, " and "))
	}

	out := &Rewritten{Source: after, Index: index, ID: ids[index], Before: c, After: rc, File: relocated, Changed: !bytes.Equal(before, after)}
	if f, ok := p.(Formatter); ok {
		if err := formatterAgrees(f, name, before, after, located, relocated, index, ids); err != nil {
			return nil, err
		}
		out.Formatter = f.FormatterName()
	}
	return out, nil
}

// formatterAgrees refuses a rewrite the formatter would change: one where the
// rewritten comment disagrees with it, or another comment does that agreed
// with it before.
func formatterAgrees(f Formatter, name string, before, after []byte, located, relocated *File, index int, ids []string) error {
	was, err := f.Disagreements(name, before, located)
	if err != nil {
		return refuse(RefusedFormatter, "%s could not compare the file before the rewrite: %v", f.FormatterName(), err)
	}
	now, err := f.Disagreements(name, after, relocated)
	if err != nil {
		return refuse(RefusedFormatter, "%s could not compare the rewritten file: %v", f.FormatterName(), err)
	}
	disagreed := map[int]bool{}
	for _, d := range was {
		disagreed[d.Comment] = true
	}
	for _, d := range now {
		switch {
		case d.Comment == index:
			return refuse(RefusedFormatter, "%s would rewrite the comment as:\n%s", f.FormatterName(), d.Formatted)
		case !disagreed[d.Comment]:
			return refuse(RefusedFormatter, "%s would rewrite %s after the rewrite", f.FormatterName(), ids[d.Comment])
		}
	}
	return nil
}

// firstDiff returns the offset of the first byte at which a and b differ, or
// -1 when they are equal.
func firstDiff(a, b []byte) int {
	for i := range min(len(a), len(b)) {
		if a[i] != b[i] {
			return i
		}
	}
	if len(a) != len(b) {
		return min(len(a), len(b))
	}
	return -1
}

// placeholderChange compares the runs of a comment that are not prose, which
// a rewrite of its prose keeps. It returns what before holds and after lacks,
// and what after holds and before lacks, each as the placeholder's data.
func placeholderChange(before, after []model.Run) (dropped, added []string) {
	was, now := placeholders(before), placeholders(after)
	for key, n := range was {
		for range n - now[key] {
			dropped = append(dropped, placeholderLabel(key))
		}
	}
	for key, n := range now {
		for range n - was[key] {
			added = append(added, placeholderLabel(key))
		}
	}
	sort.Strings(dropped)
	sort.Strings(added)
	return dropped, added
}

// placeholders counts the runs that are not prose by what each holds. A run's
// id numbers it within its comment and is not part of what it holds. A closing
// paired code is counted with its opening.
func placeholders(runs []model.Run) map[string]int {
	out := map[string]int{}
	for _, r := range runs {
		switch {
		case r.Text != nil, r.PcClose != nil:
			continue
		case r.Ph != nil:
			ph := *r.Ph
			ph.ID = ""
			r = model.Run{Ph: &ph}
		case r.PcOpen != nil:
			pc := *r.PcOpen
			pc.ID = ""
			r = model.Run{PcOpen: &pc}
		}
		key, err := json.Marshal(r)
		if err != nil {
			key = fmt.Appendf(nil, "%#v", r)
		}
		out[string(key)]++
	}
	return out
}

// placeholderLabel names a placeholder by its data, for a refusal.
func placeholderLabel(key string) string {
	var r model.Run
	if err := json.Unmarshal([]byte(key), &r); err == nil {
		switch {
		case r.Ph != nil && r.Ph.Type == TypeListItem:
			return fmt.Sprintf("the list item %q", r.Ph.Data)
		case r.Ph != nil:
			return fmt.Sprintf("%q", r.Ph.Data)
		case r.PcOpen != nil:
			return fmt.Sprintf("the link to %q", r.PcOpen.Attr(model.AttrHref))
		}
	}
	return key
}

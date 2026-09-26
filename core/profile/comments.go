package profile

import (
	"fmt"

	"github.com/neokapi/neokapi/core/check"
	"gopkg.in/yaml.v3"
)

// CommentRules are the limits a check holds the comments in source files to,
// at the point the profile governs. A profile that sets them asks for the
// comment checks, and each limit it leaves out takes its default
// (check.DefaultCommentLimits), so `comments: {}` asks for every default. A
// limit counts words, and a code span, a reference or a link in a comment is
// never a word.
type CommentRules struct {
	// SentenceWords is the most words a sentence may hold.
	SentenceWords *WordLimit `json:"sentence_words,omitempty" yaml:"sentence_words,omitempty"`
	// CommentWords is the most words a comment that documents no declaration
	// may hold.
	CommentWords *int `json:"comment_words,omitempty" yaml:"comment_words,omitempty"`
	// DocWords is the most words a declaration's doc comment may hold.
	DocWords *int `json:"doc_words,omitempty" yaml:"doc_words,omitempty"`
	// PackageDocWords is the most words the doc comment of a package or module
	// may hold.
	PackageDocWords *int `json:"package_doc_words,omitempty" yaml:"package_doc_words,omitempty"`
	// Density limits the comment lines a change adds for the code lines it
	// adds. A check scoped to a diff applies it.
	Density *DensityLimits `json:"density,omitempty" yaml:"density,omitempty"`
	// Fails makes a comment over a limit fail a check. Unset, the limits are
	// style measures and what they find reports.
	Fails bool `json:"fails,omitempty" yaml:"fails,omitempty"`
}

// DensityLimits flag a change that adds at least MinCommentLines comment lines
// and more than Ratio comment lines for each code line.
type DensityLimits struct {
	Ratio           *float64 `json:"ratio,omitempty" yaml:"ratio,omitempty"`
	MinCommentLines *int     `json:"min_comment_lines,omitempty" yaml:"min_comment_lines,omitempty"`
}

// WordLimit is a number of words. It reads from YAML as a plain number, and a
// mapping of graded limits is refused with the form to write instead.
type WordLimit int

// UnmarshalYAML reads the limit.
func (w *WordLimit) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.MappingNode {
		return fmt.Errorf("line %d: sentence_words is one number: the most words a sentence may hold (write "+
			"`sentence_words: 50`); `fails: true` beside it makes a sentence over the limit fail a check", n.Line)
	}
	var v int
	if err := n.Decode(&v); err != nil {
		return err
	}
	*w = WordLimit(v)
	return nil
}

// words is the limit as a plain number, or nil.
func (w *WordLimit) words() *int {
	if w == nil {
		return nil
	}
	v := int(*w)
	return &v
}

// Limits returns the limits the rules set, each one they leave out at its
// default.
func (r *CommentRules) Limits() check.CommentLimits {
	l := check.DefaultCommentLimits()
	if r == nil {
		return l
	}
	set := func(dst *int, v *int) {
		if v != nil {
			*dst = *v
		}
	}
	set(&l.SentenceWords, r.SentenceWords.words())
	set(&l.CommentWords, r.CommentWords)
	set(&l.DocWords, r.DocWords)
	set(&l.PackageDocWords, r.PackageDocWords)
	if d := r.Density; d != nil {
		if d.Ratio != nil {
			l.DensityRatio = *d.Ratio
		}
		set(&l.DensityMinLines, d.MinCommentLines)
	}
	l.Fails = r.Fails
	return l
}

// clone copies the rules and every limit they point at.
func (r *CommentRules) clone() *CommentRules {
	if r == nil {
		return nil
	}
	c := &CommentRules{
		CommentWords:    cloneInt(r.CommentWords),
		DocWords:        cloneInt(r.DocWords),
		PackageDocWords: cloneInt(r.PackageDocWords),
		Fails:           r.Fails,
	}
	if s := r.SentenceWords; s != nil {
		v := *s
		c.SentenceWords = &v
	}
	if d := r.Density; d != nil {
		c.Density = &DensityLimits{MinCommentLines: cloneInt(d.MinCommentLines)}
		if d.Ratio != nil {
			ratio := *d.Ratio
			c.Density.Ratio = &ratio
		}
	}
	return c
}

func cloneInt(v *int) *int {
	if v == nil {
		return nil
	}
	n := *v
	return &n
}

// validateCommentRules adds a problem for each limit under base, such as
// "style.comments", that is not a positive number of words.
func validateCommentRules(add func(field, msg string), base string, r *CommentRules) {
	if r == nil {
		return
	}
	positive := func(field string, v *int) {
		if v != nil && *v <= 0 {
			add(base+"."+field, fmt.Sprintf("a limit is a positive number of words (got %d); leave the key out to use the default", *v))
		}
	}
	positive("sentence_words", r.SentenceWords.words())
	positive("comment_words", r.CommentWords)
	positive("doc_words", r.DocWords)
	positive("package_doc_words", r.PackageDocWords)
	if d := r.Density; d != nil {
		if d.Ratio != nil && !(*d.Ratio > 0) {
			add(base+".density.ratio", fmt.Sprintf("a ratio is a positive number of comment lines for each code line (got %v); leave the key out to use the default", *d.Ratio))
		}
		if d.MinCommentLines != nil && *d.MinCommentLines <= 0 {
			add(base+".density.min_comment_lines", fmt.Sprintf("a limit is a positive number of lines (got %d); leave the key out to use the default", *d.MinCommentLines))
		}
	}
}

// commentRulesError returns the first problem with any comment limits the
// profile sets, in its own style or an override's, as an error naming the key.
func commentRulesError(p *VoiceProfile) error {
	var first error
	add := func(field, msg string) {
		if first == nil {
			first = fmt.Errorf("%s: %s", field, msg)
		}
	}
	validateAllCommentRules(add, p)
	return first
}

// validateAllCommentRules validates the comment limits in the profile's style
// and in the style of each channel and persona that sets one.
func validateAllCommentRules(add func(field, msg string), p *VoiceProfile) {
	validateCommentRules(add, "style.comments", p.Style.Comments)
	for _, name := range sortedKeys(p.Channels) {
		if s := p.Channels[name].Style; s != nil {
			validateCommentRules(add, "channels."+name+".style.comments", s.Comments)
		}
	}
	for _, name := range sortedKeys(p.Personas) {
		if s := p.Personas[name].Style; s != nil {
			validateCommentRules(add, "personas."+name+".style.comments", s.Comments)
		}
	}
}

// validateCommentOverrides warns where a channel or persona supplies its own
// style without comment limits while the profile's style sets them, so the
// limits stop applying there. It is the comment-limit half of
// validatePresentationOverrides.
func validateCommentOverrides(p *VoiceProfile) []ProfileProblem {
	if p.Style.Comments == nil {
		return nil
	}
	var probs []ProfileProblem
	report := func(field, kind, name string, style *StyleRules) {
		if style == nil || style.Comments != nil {
			return
		}
		probs = append(probs, ProfileProblem{
			Field: field,
			Message: fmt.Sprintf("%s %q supplies its own style with no comments section, so the profile's comment limits "+
				"do not apply there. Restate them under %s.comments to keep them.", kind, name, field),
			Warning: true,
			Code:    CodeOverrideDropsCommentRules,
		})
	}
	for _, name := range sortedKeys(p.Channels) {
		report("channels."+name+".style", "channel", name, p.Channels[name].Style)
	}
	for _, name := range sortedKeys(p.Personas) {
		report("personas."+name+".style", "persona", name, p.Personas[name].Style)
	}
	return probs
}

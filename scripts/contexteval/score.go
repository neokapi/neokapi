package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	coretools "github.com/neokapi/neokapi/core/tools"
)

// Scoring reuses the framework's own check tools rather than reimplementing
// them: term-check for glossary mandates, dnt-check for verbatim survival,
// voice-vocab-check for forbidden-term hits, pattern-check for the regex-shaped
// rules. That is deliberate and load-bearing — a model that "games" this eval
// is a model that satisfies the checks kapi actually ships, and a check tool
// the eval exercises cannot silently rot. It also means the eval inherits the
// tools' semantics, blind spots included (whole-word matching does not catch a
// German inflection of a forbidden term); the eval measures the system as
// shipped, not an idealized scorer.

// Counts is scored-vs-passed for a set of checks. Adherence is Passed/Scored.
type Counts struct {
	Scored int `json:"scored"`
	Passed int `json:"passed"`
}

func (c *Counts) add(pass bool) {
	c.Scored++
	if pass {
		c.Passed++
	}
}

// Rate is adherence as a percentage; -1 when nothing was scored, so a missing
// measurement can never masquerade as 0% adherence.
func (c Counts) Rate() float64 {
	if c.Scored == 0 {
		return -1
	}
	return float64(c.Passed) / float64(c.Scored) * 100
}

// PassScore is one variant pass (bare or steered), scored.
type PassScore struct {
	// Missing counts fixtures whose translation never came back; Untranslated
	// counts source echoes on targets where an echo cannot be a translation
	// (de, fr — not en-GB, where identity is a legitimate rendering). Both are
	// excluded from adherence: an echoed segment trivially "preserves" every
	// DNT term, and scoring it would inflate exactly the number we are
	// measuring. They are reported as their own failure counts instead.
	Missing      int
	Untranslated int

	ByDim  map[string]*Counts
	ByKind map[string]*Counts // key: dimension + "/" + kind
}

func newPassScore() PassScore {
	return PassScore{ByDim: map[string]*Counts{}, ByKind: map[string]*Counts{}}
}

func (s *PassScore) record(chk Check, pass bool) {
	d, ok := s.ByDim[chk.Dimension]
	if !ok {
		d = &Counts{}
		s.ByDim[chk.Dimension] = d
	}
	d.add(pass)
	key := chk.Dimension + "/" + chk.Kind
	k, ok := s.ByKind[key]
	if !ok {
		k = &Counts{}
		s.ByKind[key] = k
	}
	k.add(pass)
}

// answerState is the single owner of the answered-fixture rule, shared by the
// deterministic scorer and the judge so their denominators can never diverge.
type answerState int

const (
	answered answerState = iota
	// answerMissing: the translation never came back.
	answerMissing
	// answerEchoed: the source came back verbatim on a target whose language
	// differs from the source's. On en → en-GB an unchanged string is a
	// legitimate rendering and is scored; on de/fr it is an echo, not a
	// translation, and scoring it would let a lazy model "pass" every
	// do-not-translate check for free.
	answerEchoed
)

func (c TestCorpus) answerStateOf(f Fixture, got string) answerState {
	switch {
	case got == "":
		return answerMissing
	case got == f.Source && baseLang(c.Target) != baseLang(string(model.LocaleEnglish)):
		return answerEchoed
	}
	return answered
}

// scorePass scores one translated corpus. blocks must be the corpus rendered
// by TestCorpus.Blocks and already run through the translate tool.
func scorePass(ctx context.Context, corpus TestCorpus, blocks []*model.Block, variant string) (PassScore, error) {
	score := newPassScore()
	loc := model.LocaleID(corpus.Target)

	for i, f := range corpus.Fixtures {
		b := blocks[i]
		got := b.TargetText(loc)
		switch corpus.answerStateOf(f, got) {
		case answerMissing:
			score.Missing++
			dump(variant, f, "missing", "no translation came back", "")
			continue
		case answerEchoed:
			score.Untranslated++
			dump(variant, f, "untranslated", "source echoed verbatim", got)
			continue
		}

		for _, chk := range f.Checks[corpus.Target] {
			pass, detail, err := runCheck(ctx, corpus, chk, f, b, got)
			if err != nil {
				return PassScore{}, fmt.Errorf("fixture %s: %w", f.Key, err)
			}
			score.record(chk, pass)
			if !pass {
				dump(variant, f, chk.Dimension+"/"+chk.Kind, detail, got)
			}
		}
	}
	return score, nil
}

// runCheck scores a single expectation with the matching framework check tool.
func runCheck(ctx context.Context, corpus TestCorpus, chk Check, f Fixture, b *model.Block, got string) (pass bool, detail string, err error) {
	loc := model.LocaleID(corpus.Target)
	switch {
	case chk.Term != nil:
		return runTermCheck(ctx, chk, b, loc)
	case chk.DNT != "":
		return runDNTCheck(ctx, chk, b, loc)
	case chk.VocabClean:
		return runVocabCheck(ctx, corpus, f, got)
	case chk.MustMatch != "" || chk.MustNotMatch != "":
		return runPatternCheck(ctx, chk, got)
	}
	return false, "", fmt.Errorf("check %s/%s declares no scoring backend", chk.Dimension, chk.Kind)
}

// runTermCheck drives the real term-check tool with a single-entry glossary, so
// each mandate scores independently. The tool reports via block properties,
// which are cleared afterwards so the next mandate reads its own verdict.
func runTermCheck(ctx context.Context, chk Check, b *model.Block, loc model.LocaleID) (bool, string, error) {
	t := coretools.NewTermCheckTool(&coretools.TermCheckConfig{
		TargetLocale: loc,
		Glossary:     []coretools.GlossaryEntry{{Source: chk.Term.Source, Target: chk.Term.Target}},
	})
	if _, err := t.ApplyContext(ctx, &model.Part{Type: model.PartBlock, Resource: b}); err != nil {
		return false, "", err
	}
	passed := b.Properties[coretools.PropTermCheckPassed] == "true"
	detail := b.Properties[coretools.PropTermCheckErrors]
	delete(b.Properties, coretools.PropTermCheckPassed)
	delete(b.Properties, coretools.PropTermCheckErrors)
	return passed, detail, nil
}

// runDNTCheck drives the real dnt-check tool for one term. Findings accumulate
// on the block's unified quality annotation, so the verdict is the delta.
func runDNTCheck(ctx context.Context, chk Check, b *model.Block, loc model.LocaleID) (bool, string, error) {
	before := len(blockFindings(b))
	cfg := coretools.NewDNTCheckConfig(loc)
	cfg.Terms = []string{chk.DNT}
	t := coretools.NewDNTCheckTool(cfg)
	if _, err := t.ApplyContext(ctx, &model.Part{Type: model.PartBlock, Resource: b}); err != nil {
		return false, "", err
	}
	after := blockFindings(b)
	if len(after) == before {
		return true, "", nil
	}
	return false, after[len(after)-1].Message, nil
}

// runVocabCheck drives the real voice-vocab-check tool over the translation,
// judging by the exact profile the steered pass injected (corpus.Ctx.Profile) —
// the scorer and the injection hold one rulebook by construction. The tool
// reads a block's source text (it is an authoring-side check), so the
// translation is presented as the source of a scratch block — the same shape
// host.RunBlockTool uses for ad-hoc text. Hits on terms the fixture excuses
// (the declared winner of an engineered conflict) are filtered out; matching is
// on the finding's matched span, case-insensitively, since the tool matches
// whole words in either case.
func runVocabCheck(ctx context.Context, corpus TestCorpus, f Fixture, got string) (bool, string, error) {
	scratch := model.NewBlock("scored-translation", got)
	scratch.Translatable = true
	t := coretools.NewVoiceVocabCheckTool(corpus.Ctx.Profile, nil)
	if _, err := t.ApplyContext(ctx, &model.Part{Type: model.PartBlock, Resource: scratch}); err != nil {
		return false, "", err
	}
	ann, ok := model.AnnoAs[*profile.VoiceAnnotation](scratch, "voice")
	if !ok {
		return true, "", nil
	}
	var remaining []string
	for _, finding := range ann.Findings {
		excused := false
		for _, allow := range f.AllowVocab {
			if strings.EqualFold(finding.OriginalText, allow) {
				excused = true
				break
			}
		}
		if !excused {
			remaining = append(remaining, finding.Message)
		}
	}
	if len(remaining) == 0 {
		return true, "", nil
	}
	return false, strings.Join(remaining, "; "), nil
}

// runPatternCheck drives the real pattern-check tool over the translation
// (again presented as a scratch block's source). One tool per expectation, so
// the verdict maps to the check without parsing finding messages.
func runPatternCheck(ctx context.Context, chk Check, got string) (bool, string, error) {
	var rules []check.PatternRule
	if chk.MustMatch != "" {
		rules = append(rules, check.PatternRule{Name: chk.Kind + "-required", Pattern: chk.MustMatch, MustMatch: true})
	}
	if chk.MustNotMatch != "" {
		rules = append(rules, check.PatternRule{Name: chk.Kind + "-forbidden", Pattern: chk.MustNotMatch, MustNotMatch: true})
	}
	t, err := check.NewSourcePatternTool(rules)
	if err != nil {
		return false, "", err
	}
	scratch := model.NewBlock("scored-translation", got)
	scratch.Translatable = true
	if _, err := t.ApplyContext(ctx, &model.Part{Type: model.PartBlock, Resource: scratch}); err != nil {
		return false, "", err
	}
	findings := blockFindings(scratch)
	if len(findings) == 0 {
		return true, "", nil
	}
	var msgs []string
	for _, finding := range findings {
		msgs = append(msgs, finding.Message)
	}
	return false, strings.Join(msgs, "; "), nil
}

func blockFindings(b *model.Block) []check.Finding {
	if ann, ok := model.AnnoAs[*check.FindingsAnnotation](b, check.AnnotationKey); ok {
		return ann.Findings
	}
	return nil
}

func baseLang(locale string) string {
	l := strings.ToLower(locale)
	if i := strings.IndexAny(l, "-_"); i >= 0 {
		return l[:i]
	}
	return l
}

// dump prints a scored failure when -dump is set. A count nobody can inspect
// is a number, not a finding.
func dump(variant string, f Fixture, kind, detail, got string) {
	if !dumpFailures {
		return
	}
	fmt.Fprintf(os.Stderr, "    [%s] %s %s\n      src: %q\n      got: %q\n", variant, f.Key, kind, f.Source, got)
	if detail != "" {
		fmt.Fprintf(os.Stderr, "      why: %s\n", detail)
	}
}

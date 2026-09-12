package host

import (
	"context"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/encoding"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	coretools "github.com/neokapi/neokapi/core/tools"
)

// hygieneTool builds the always-on hygiene checker. It is a variable so a test
// can put an inert checker in its place and show that the canary invalidates the
// run.
var hygieneTool = func() BlockProcessor { return check.NewContentLintTool() }

// probeTool gives canaries to a block tool that records its findings on the
// unified findings annotation: the same tool instance the content went through.
func probeTool(ctx context.Context, t BlockProcessor, canaries []check.Canary, uncheckable string) (check.CanaryOutcome, error) {
	return check.Probe(canaries, uncheckable, func(b *model.Block) ([]check.Finding, error) {
		if err := RunCheckTool(ctx, t, b); err != nil {
			return nil, err
		}
		return FindingsFromBlock(b, false), nil
	})
}

// probeVoiceRules gives the vocabulary checker its canaries, and the profile's
// document-scope matcher its required-pattern canary. Both are the voice.rules
// analyzer, so a miss in either invalidates it, and a catch in either shows it
// can fail.
func probeVoiceRules(ctx context.Context, vocab *coretools.VoiceVocabCheckTool, p *profile.VoiceProfile) (check.CanaryOutcome, error) {
	canaries, uncheckable, err := vocab.Canaries(ctx)
	if err != nil {
		return check.CanaryOutcome{}, err
	}
	blockScope, err := check.Probe(canaries, uncheckable, func(b *model.Block) ([]check.Finding, error) {
		if err := RunCheckTool(ctx, vocab, b); err != nil {
			return nil, err
		}
		if ann, ok := model.AnnoAs[*profile.VoiceAnnotation](b, model.AnnoVoice); ok {
			return ann.Findings, nil
		}
		return nil, nil
	})
	if err != nil {
		return check.CanaryOutcome{}, err
	}
	docCanaries, docUncheckable := coretools.RequiredPatternCanaries(p)
	documentScope, err := check.Probe(docCanaries, docUncheckable, func(b *model.Block) ([]check.Finding, error) {
		return profile.DocumentFindings(p, documentText([]*model.Block{b})), nil
	})
	if err != nil {
		return check.CanaryOutcome{}, err
	}
	return blockScope.Merge(documentScope), nil
}

// probeReaderValidation gives the validation pass bytes that are not valid
// UTF-8, which its encoding diagnostics must flag under the same declared
// encoding and mode. A reader's own structure diagnostics depend on its format,
// and this canary does not exercise them.
func (a *App) probeReaderValidation(mode format.ValidationMode) (check.CanaryOutcome, error) {
	canary := check.Canary{Name: "invalid UTF-8", Block: check.CanaryBlock("canary \xff\xfe")}
	return check.Probe([]check.Canary{canary}, "", func(b *model.Block) ([]check.Finding, error) {
		var out []check.Finding
		for _, d := range encoding.Diagnose([]byte(b.SourceText()), a.InputEncoding(), mode) {
			out = append(out, check.Finding{Category: d.Category, Message: d.Message})
		}
		return out, nil
	})
}

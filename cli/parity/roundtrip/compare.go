//go:build parity

package roundtrip

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// extractedBlocks reads `output` through the registered native
// reader for `formatID` and returns the "translated text" for each
// translatable Block, in document order. The translated text is:
//
//   - block.Target[targetLocale] when bilingual formats (PO, XLIFF,
//     TMX, …) keep the original source intact and stash the
//     translation in a target slot, OR
//   - block.SourceText() when monolingual formats (plaintext, HTML,
//     properties, …) overwrite the source with the translation on
//     merge.
//
// After a successful round-trip the returned text equals
// spec.Wrap(originalSource[i]) for every block, regardless of
// whether the format is bi- or monolingual.
//
// Re-extracting through the native reader is deliberate: we want
// every engine's output evaluated against the same yardstick.
// Differences between engines surface as differing extracted
// streams, not as encoding noise that would dominate a byte-equal
// comparison.
func extractedBlocks(formatID registry.FormatID, output []byte, sourceLocale, targetLocale string, readerConfig map[string]any) ([]string, error) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	reader, err := reg.NewReader(formatID)
	if err != nil {
		return nil, fmt.Errorf("re-extract: reader for %q: %w", formatID, err)
	}
	defer func() { _ = reader.Close() }()
	if len(readerConfig) > 0 {
		if cfg := reader.Config(); cfg != nil {
			if err := cfg.ApplyMap(readerConfig); err != nil {
				return nil, fmt.Errorf("re-extract: ApplyMap: %w", err)
			}
		}
	}

	doc := &model.RawDocument{
		URI:          "roundtrip-output",
		SourceLocale: model.LocaleID(sourceLocale),
		TargetLocale: model.LocaleID(targetLocale),
		Reader:       io.NopCloser(bytes.NewReader(output)),
	}
	ctx := context.Background()
	if err := reader.Open(ctx, doc); err != nil {
		return nil, fmt.Errorf("re-extract: open: %w", err)
	}

	var texts []string
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			return nil, fmt.Errorf("re-extract: stream: %w", res.Error)
		}
		if res.Part == nil || res.Part.Type != model.PartBlock {
			continue
		}
		block, ok := res.Part.Resource.(*model.Block)
		if !ok || !block.Translatable {
			continue
		}
		texts = append(texts, blockTranslatedText(block, model.LocaleID(targetLocale)))
	}
	return texts, nil
}

// blockTranslatedText returns the block's target text for the given
// locale, falling back to source text when the block doesn't carry
// the target. This handles bilingual (PO, XLIFF) vs monolingual
// (plaintext, HTML) format semantics uniformly.
func blockTranslatedText(b *model.Block, target model.LocaleID) string {
	if target != "" && b.HasTarget(target) {
		if txt := b.TargetText(target); txt != "" {
			return txt
		}
	}
	return b.SourceText()
}

// expectedTargets computes the per-block strings each engine's
// output should produce when re-extracted: spec.Wrap(originalText)
// for every translatable block in the input. Sources are taken from
// the input bytes via the same native reader the comparison uses, so
// the expectation comes from the same authority that judges the
// outputs.
func expectedTargets(formatID registry.FormatID, input []byte, spec PseudoSpec, readerConfig map[string]any) ([]string, error) {
	// For the input we want raw source text, not target — the input
	// has no targets yet. Pass an empty targetLocale so
	// extractedBlocks falls through to SourceText().
	sources, err := extractedBlocks(formatID, input, spec.SrcLocale(), "", readerConfig)
	if err != nil {
		return nil, err
	}
	wrapped := make([]string, len(sources))
	for i, s := range sources {
		wrapped[i] = spec.Wrap(s)
	}
	return wrapped, nil
}

// Divergence captures the failure mode for one engine compared to
// the expected output. Reported per-engine so the harness can list
// every disagreement at once instead of bailing on the first.
type Divergence struct {
	Engine   string
	Expected []string
	Actual   []string
	Reason   string
}

// String renders a divergence for test output.
func (d Divergence) String() string {
	return fmt.Sprintf("%s: %s\n  expected blocks: %v\n  actual blocks:   %v",
		d.Engine, d.Reason, d.Expected, d.Actual)
}

// reasonFor produces a short reason string for a Divergence.
func reasonFor(expected, actual []string) string {
	if len(expected) != len(actual) {
		return fmt.Sprintf("block count differs: expected %d, got %d", len(expected), len(actual))
	}
	for i := range expected {
		if expected[i] != actual[i] {
			return fmt.Sprintf("block[%d] differs: expected %q, got %q", i, expected[i], actual[i])
		}
	}
	return ""
}

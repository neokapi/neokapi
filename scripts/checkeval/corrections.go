package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	coretools "github.com/neokapi/neokapi/core/tools"
)

// CorrectionEntry is one recurring correction in the simulated stream.
type CorrectionEntry struct {
	Term        string `json:"term"`
	Replacement string `json:"replacement"`
	Dimension   string `json:"dimension"`
	Count       int    `json:"count"`
	Original    string `json:"original"`
	Corrected   string `json:"corrected"`
	Note        string `json:"note"`
}

// CorrectionsCorpus is the simulated correction stream.
type CorrectionsCorpus struct {
	MinCount    int               `json:"min_count"`
	Corrections []CorrectionEntry `json:"corrections"`
}

// CorrectionCaseResult is the per-correction outcome of the corrections-as-
// ground-truth eval.
type CorrectionCaseResult struct {
	Term             string `json:"term"`
	Replacement      string `json:"replacement"`
	Count            int    `json:"count"`
	Promoted         bool   `json:"promoted"`
	OriginalFlagged  bool   `json:"original_flagged"`
	CorrectedFlagged bool   `json:"corrected_flagged"`
	OK               bool   `json:"ok"`
	Note             string `json:"note"`
}

// CorrectionsReport aggregates the corrections-stream eval for the dashboard.
type CorrectionsReport struct {
	MinCount  int                    `json:"min_count"`
	Total     int                    `json:"total"`
	Promoted  int                    `json:"promoted"`
	TP        int                    `json:"tp"` // promoted originals correctly flagged
	FN        int                    `json:"fn"` // promoted originals missed
	FP        int                    `json:"fp"` // corrected fixes wrongly flagged
	Precision float64                `json:"precision"`
	Recall    float64                `json:"recall"`
	F1        float64                `json:"f1"`
	Cases     []CorrectionCaseResult `json:"cases"`
}

// LoadCorrectionsCorpus reads a simulated correction stream.
func LoadCorrectionsCorpus(path string) (CorrectionsCorpus, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CorrectionsCorpus{}, err
	}
	var c CorrectionsCorpus
	if err := json.Unmarshal(data, &c); err != nil {
		return CorrectionsCorpus{}, fmt.Errorf("checkeval: parse %s: %w", path, err)
	}
	if c.MinCount <= 0 {
		c.MinCount = 3
	}
	return c, nil
}

// brandVocabFlags reports whether the brand-vocabulary check raises any finding
// on text under the profile — the real checker the loop's promoted rules feed.
func brandVocabFlags(profile *brand.VoiceProfile, text string) bool {
	b := &model.Block{ID: "c", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: text}}}}
	if err := coretools.NewBrandVocabCheckTool(profile, nil).Annotate(tool.NewBlockView(b)); err != nil {
		return false
	}
	if ann, ok := b.Annotations["brand-voice"].(*brand.BrandVoiceAnnotation); ok {
		return len(ann.Findings) > 0
	}
	return false
}

// EvaluateCorrections runs the loop end-to-end over the simulated stream: it
// aggregates the corrections, promotes those at or above MinCount into a profile
// through the real promotion path (brand.ApplySuggestedRule), then checks that
// each promoted rule FLAGS its original (the mistake the team kept correcting)
// and does NOT flag the corrected fix — and that a below-threshold correction is
// left un-enforced. This is the corrections-as-ground-truth measure of the loop.
func EvaluateCorrections(corpus CorrectionsCorpus) CorrectionsReport {
	rep := CorrectionsReport{MinCount: corpus.MinCount, Total: len(corpus.Corrections)}

	// Promote the above-threshold corrections into a profile — exactly what the
	// server's PromoteAndSave does, minus persistence.
	profile := &brand.VoiceProfile{Name: "corrections-eval"}
	for _, c := range corpus.Corrections {
		if c.Count >= corpus.MinCount {
			brand.ApplySuggestedRule(profile, brand.SuggestedRule{
				Term: c.Term, Replacement: c.Replacement,
				CorrectionCount: c.Count, Dimension: brand.Dimension(c.Dimension),
			})
		}
	}

	for _, c := range corpus.Corrections {
		promoted := c.Count >= corpus.MinCount
		origFlagged := brandVocabFlags(profile, c.Original)
		corrFlagged := brandVocabFlags(profile, c.Corrected)

		var ok bool
		if promoted {
			rep.Promoted++
			if origFlagged {
				rep.TP++
			} else {
				rep.FN++
			}
			if corrFlagged {
				rep.FP++
			}
			ok = origFlagged && !corrFlagged
		} else {
			// Below threshold: the check must stay silent on both forms.
			ok = !origFlagged && !corrFlagged
		}

		rep.Cases = append(rep.Cases, CorrectionCaseResult{
			Term: c.Term, Replacement: c.Replacement, Count: c.Count, Promoted: promoted,
			OriginalFlagged: origFlagged, CorrectedFlagged: corrFlagged, OK: ok, Note: c.Note,
		})
	}

	rep.Precision = ratio(rep.TP, rep.TP+rep.FP)
	rep.Recall = ratio(rep.TP, rep.TP+rep.FN)
	if rep.Precision+rep.Recall > 0 {
		rep.F1 = 2 * rep.Precision * rep.Recall / (rep.Precision + rep.Recall)
	}
	return rep
}

// ratio is n/d, or 1.0 when there is nothing to measure (no false anything).
func ratio(n, d int) float64 {
	if d == 0 {
		return 1
	}
	return float64(n) / float64(d)
}

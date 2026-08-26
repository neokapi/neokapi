package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/segment"
	_ "github.com/neokapi/neokapi/core/segment/uax29" // the baseline sentence engine
	"github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/memory"
)

// The other half of the answer to "a percentage is a bad measure": WHERE the
// change is, not just how big it is.
//
// A paragraph is one block, so one edited sentence makes the whole paragraph a
// partial match. Every sentence in it gets the same verdict, and a translator is
// handed three sentences to re-read when one moved. The score cannot say which,
// because a score is one number for the whole block by construction.
//
// Segment it and each sentence answers for itself: two are untouched and keep
// their approved wording, one moved and is the only thing anyone has to look at.
// A model asked to translate that one sentence gets the paragraph's other two
// already approved beside it, which is more context than it had when the block
// went over as an undifferentiated lump.

const segmentEngine = "uax29"

// A support paragraph, and the same paragraph after an author's pass over it:
// the middle sentence's meaning moved, the last one gained a comma.
const (
	segmentOriginal = "Your plan renews on the first of each month. " +
		"We charge the card on file the day before. " +
		"You can cancel at any time from the billing page."
	segmentEdited = "Your plan renews on the first of each month. " +
		"We charge the card on file three days before. " +
		"You can cancel at any time, from the billing page."
)

const segmentTarget = "Abonnementet fornyes den første i hver måned. " +
	"Vi belaster kortet vi har lagret dagen før. " +
	"Du kan si opp når som helst fra fakturasiden."

// SegmentRow is one sentence, judged on its own.
type SegmentRow struct {
	Index      int        `json:"index"`
	Prior      string     `json:"prior"`
	Current    string     `json:"current"`
	Diff       tools.Diff `json:"diff"`
	Score      int        `json:"score"`
	Classified string     `json:"classified"`
	// Filled is the target the real tool wrote for this sentence on its own.
	Filled string `json:"filled"`
}

// SegmentSplit is the same edit measured twice: once as a block, once sentence
// by sentence.
type SegmentSplit struct {
	Engine   string `json:"engine"`
	Prior    string `json:"prior"`
	Current  string `json:"current"`
	Approved string `json:"approved"`
	// The block-level answer: one score, one verdict, one outcome for
	// everything in it.
	BlockDiff       tools.Diff `json:"blockDiff"`
	BlockScore      int        `json:"blockScore"`
	BlockClassified string     `json:"blockClassified"`
	BlockFilled     string     `json:"blockFilled"`
	// BlockFilledByScore is what a percentage does with the same paragraph. One
	// sentence of three moved, so the block still scores in the nineties, and a
	// fill floor writes the old billing terms back out.
	BlockFilledByScore string `json:"blockFilledByScore"`
	// The sentence-level answer.
	Segments []SegmentRow `json:"segments"`
	// Reusable is how many sentences keep their approved wording; Moved is how
	// many a translator or a model has to look at.
	Reusable int `json:"reusable"`
	Moved    int `json:"moved"`
}

// buildSegmentSplit measures the same edit at both grains, through the real
// segmenter, the real matcher and the real tool.
func buildSegmentSplit(ctx context.Context) (*SegmentSplit, error) {
	seg, err := segment.Build(segmentEngine, segment.BaseConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("build segmenter: %w", err)
	}

	priorSentences, err := sentences(ctx, seg, segmentOriginal, "en")
	if err != nil {
		return nil, err
	}
	currentSentences, err := sentences(ctx, seg, segmentEdited, "en")
	if err != nil {
		return nil, err
	}
	if len(priorSentences) != len(currentSentences) {
		return nil, fmt.Errorf("segmenter split the two versions differently: %d and %d",
			len(priorSentences), len(currentSentences))
	}

	// A corpus holding the paragraph as one approved unit AND each of its
	// sentences, which is what a content memory accumulates: blocks are
	// recorded whole, and the segment path records the sentences inside them.
	blockCorpus := memory.NewInMemoryStore()
	if err := blockCorpus.Add(ctx, entryOf("paragraph", segmentOriginal, segmentTarget)); err != nil {
		return nil, fmt.Errorf("seed paragraph: %w", err)
	}
	sentenceCorpus := memory.NewInMemoryStore()
	priorTargets, err := sentences(ctx, seg, segmentTarget, "nb")
	if err != nil {
		return nil, err
	}
	if len(priorTargets) != len(priorSentences) {
		return nil, fmt.Errorf("the approved target has %d sentences and its source has %d",
			len(priorTargets), len(priorSentences))
	}
	for i, s := range priorSentences {
		id := fmt.Sprintf("sentence-%d", i)
		if err := sentenceCorpus.Add(ctx, entryOf(id, s, priorTargets[i])); err != nil {
			return nil, fmt.Errorf("seed %s: %w", id, err)
		}
	}

	out := &SegmentSplit{
		Engine:          segmentEngine,
		Prior:           segmentOriginal,
		Current:         segmentEdited,
		Approved:        segmentTarget,
		BlockDiff:       tools.DiffEdit(segmentOriginal, segmentEdited),
		BlockClassified: string(tools.ClassifyEdit(segmentOriginal, segmentEdited)),
	}
	out.BlockScore, err = scoreOf(ctx, blockCorpus, segmentEdited)
	if err != nil {
		return nil, err
	}
	out.BlockFilled, err = fillUnder(ctx, blockCorpus, segmentEdited, readsTheEdit)
	if err != nil {
		return nil, err
	}
	out.BlockFilledByScore, err = fillUnder(ctx, blockCorpus, segmentEdited, scoreOnly)
	if err != nil {
		return nil, err
	}

	for i, cur := range currentSentences {
		kind := tools.ClassifyEdit(priorSentences[i], cur)
		row := SegmentRow{
			Index:      i + 1,
			Prior:      priorSentences[i],
			Current:    cur,
			Diff:       tools.DiffEdit(priorSentences[i], cur),
			Classified: string(kind),
		}
		row.Score, err = scoreOf(ctx, sentenceCorpus, cur)
		if err != nil {
			return nil, err
		}
		row.Filled, err = fillUnder(ctx, sentenceCorpus, cur, readsTheEdit)
		if err != nil {
			return nil, err
		}
		if row.Filled != "" {
			out.Reusable++
		} else {
			out.Moved++
		}
		out.Segments = append(out.Segments, row)
	}
	return out, nil
}

// sentences runs the real segmenter and returns the text of each span.
//
// The offsets come from ByteSpan, not TextSpan: TextSpan counts runes, and
// slicing a Go string by a rune count silently shifts by one byte for every
// character above ASCII. On English it is invisible; on "måned" it takes the
// last letter off every sentence, which is how the segment table first rendered
// its Norwegian.
func sentences(ctx context.Context, seg segment.Segmenter, text string, loc model.LocaleID) ([]string, error) {
	runs := []model.Run{{Text: &model.TextRun{Text: text}}}
	spans, err := seg.Segment(ctx, runs, loc)
	if err != nil {
		return nil, fmt.Errorf("segment %q: %w", text, err)
	}
	out := make([]string, 0, len(spans))
	for _, sp := range spans {
		start, end := sp.Range.ByteSpan(runs)
		if t := strings.TrimSpace(text[start:end]); t != "" {
			out = append(out, t)
		}
	}
	return out, nil
}

// scoreOf is what the matcher says, with the floor almost off so the number is
// the scorer's rather than a threshold's.
func scoreOf(ctx context.Context, tm memory.ContentMemory, source string) (int, error) {
	matches, err := tm.Lookup(ctx, blockOf(source), "en", "nb", memory.LookupOptions{
		MinScore: 0.01, MaxResults: 1,
	})
	if err != nil {
		return 0, fmt.Errorf("score %q: %w", source, err)
	}
	if len(matches) == 0 {
		return 0, nil
	}
	return int(matches[0].Score * 100), nil
}

func entryOf(id, source, target string) memory.Entry {
	return memory.Entry{
		ID:          id,
		HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: source}}},
			"nb": {{Text: &model.TextRun{Text: target}}},
		},
	}
}

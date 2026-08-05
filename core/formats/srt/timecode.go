package srt

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// setBlockTiming parses an SRT cue timing line ("HH:MM:SS,mmm --> HH:MM:SS,mmm")
// and attaches the canonical model.TimingAnnotation (start/end in ms, AD-002) to
// the block. A non-timecode line is ignored. The raw timecode string also stays
// in Properties for byte-exact round-trip; the annotation is the format-agnostic
// anchor the content model and frontends consume.
func setBlockTiming(b *model.Block, timecode string) {
	s, e, ok := parseSRTTimecode(timecode)
	if !ok {
		return
	}
	b.SetTiming(&model.TimingAnnotation{StartMS: s, EndMS: e})
}

// cueName is the structural name for a subtitle cue: its start timestamp,
// canonically spelled.
//
// SRT numbers its cues in the file, and that number reads like a natural key —
// the format wrote it, the reader did not invent it. It is not one. Every SRT
// tool renumbers 1..N contiguously on write, so deleting one cue renumbers every
// cue after it; the number records how many cues precede this one, which is the
// definition of a positional counter. Naming a block by it means an unrelated
// deletion renames the rest of the file, and reconcile.Identify then reads a
// changed context for blocks nobody touched.
//
// The start timestamp is the alternative. It is authored data, it is unique
// within a well-formed file, and — the point — it does not move when another cue
// is inserted or removed. Only the START is used: extending how long a cue stays
// on screen is a readability fix, not a change of identity.
//
// The trade is honest and worth stating: a retime renames every cue it touches.
// There is no key that survives both, because a cue with no identifier has only
// two coordinates — when it plays and where it sits — and subtitle work moves
// both. Two things make the timestamp the better of the two. A renamed block
// still matches on content, which reconcile grades as "moved" and which keeps
// identity and translation; and the raw timecode is already carried in
// Block.Properties, so it is folded into the context hash whichever way the name
// goes. Choosing position would therefore lose delete-stability and buy no
// protection from retiming at all.
//
// A cue whose timing does not parse falls back to the raw line, and one with no
// timing at all to the builder's ordinal — positional, but reached only where
// the format handed us nothing.
func cueName(nb *model.NameBuilder, timecode string) string {
	if start, _, ok := parseSRTTimecode(timecode); ok {
		return nb.Name(formatSRTTime(start))
	}
	return nb.Name(model.StructuralPath(timecode))
}

// formatSRTTime renders milliseconds as SRT "HH:MM:SS,mmm". Canonicalising means
// two spellings of one instant ("00:00:01,0" and "00:00:01,000") are one name,
// so re-serializing a file is not an edit.
func formatSRTTime(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, (ms%3600000)/60000, (ms%60000)/1000, ms%1000)
}

// parseSRTTimecode parses "HH:MM:SS,mmm --> HH:MM:SS,mmm" into start/end ms. Any
// trailing coordinates after the end timestamp (rare SRT extensions) are ignored.
func parseSRTTimecode(line string) (startMS, endMS int64, ok bool) {
	lhs, rhs, found := strings.Cut(line, "-->")
	if !found {
		return 0, 0, false
	}
	right := strings.TrimSpace(rhs)
	if sp := strings.IndexAny(right, " \t"); sp >= 0 {
		right = right[:sp]
	}
	s, ok1 := parseSRTClock(strings.TrimSpace(lhs))
	e, ok2 := parseSRTClock(right)
	if !ok1 || !ok2 {
		return 0, 0, false
	}
	return s, e, true
}

// parseSRTClock parses "HH:MM:SS,mmm" (comma decimal separator) into milliseconds.
func parseSRTClock(s string) (int64, bool) {
	clock, msPart, _ := strings.Cut(strings.TrimSpace(s), ",")
	parts := strings.Split(clock, ":")
	if len(parts) != 3 {
		return 0, false
	}
	h, err1 := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	m, err2 := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	sec, err3 := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, false
	}
	var msec int64
	if msPart != "" {
		for len(msPart) < 3 {
			msPart += "0"
		}
		var err error
		if msec, err = strconv.ParseInt(msPart[:3], 10, 64); err != nil {
			return 0, false
		}
	}
	return ((h*60+m)*60+sec)*1000 + msec, true
}

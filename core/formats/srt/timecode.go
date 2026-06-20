package srt

import (
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

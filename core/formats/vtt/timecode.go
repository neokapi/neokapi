package vtt

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// setBlockTiming parses a VTT/SRT timecode line and attaches it to the block as
// the canonical TimingAnnotation. A non-timecode line is ignored.
func setBlockTiming(b *model.Block, timecode string) {
	s, e, settings, ok := parseVTTTimecode(timecode)
	if !ok {
		return
	}
	b.SetTiming(&model.TimingAnnotation{StartMS: s, EndMS: e})
	// Cue settings (position/align/line/region) are format-specific metadata that
	// the timing anchor doesn't carry — keep them in Properties for round-trip.
	if settings != "" {
		b.Properties["cue-settings"] = settings
	}
}

// VTT cue timing is carried on a Block as the format-agnostic
// model.TimingAnnotation (start/end in ms, AD-002); these helpers convert
// to/from WebVTT's "HH:MM:SS.mmm" wire syntax. So an ASR-produced block (or a
// cue read from SRT/TTML) serializes to VTT with no format-specific timecode
// string stored on the block.

// formatVTTTime renders milliseconds as WebVTT "HH:MM:SS.mmm".
func formatVTTTime(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	h := ms / 3600000
	m := (ms % 3600000) / 60000
	s := (ms % 60000) / 1000
	msec := ms % 1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, msec)
}

// formatVTTTimecode renders a cue's "start --> end" line.
func formatVTTTimecode(startMS, endMS int64) string {
	return formatVTTTime(startMS) + " --> " + formatVTTTime(endMS)
}

// parseVTTTimecode parses a WebVTT cue timing line ("HH:MM:SS.mmm --> HH:MM:SS.mmm"
// with optional trailing cue settings) into start/end milliseconds. The hours
// field is optional ("MM:SS.mmm"), and either '.' or ',' is accepted as the
// decimal separator (so SRT-style lines parse too). ok is false if the line is
// not a timecode.
func parseVTTTimecode(line string) (startMS, endMS int64, settings string, ok bool) {
	lhs, rhs, found := strings.Cut(line, "-->")
	if !found {
		return 0, 0, "", false
	}
	left := strings.TrimSpace(lhs)
	rest := strings.TrimSpace(rhs)
	// The right side may carry cue settings after the end timestamp.
	var right string
	if sp := strings.IndexAny(rest, " \t"); sp >= 0 {
		right = rest[:sp]
		settings = strings.TrimSpace(rest[sp+1:])
	} else {
		right = rest
	}
	s, ok1 := parseClock(left)
	e, ok2 := parseClock(right)
	if !ok1 || !ok2 {
		return 0, 0, "", false
	}
	return s, e, settings, true
}

// parseClock parses "HH:MM:SS.mmm", "MM:SS.mmm", or SRT "HH:MM:SS,mmm" → ms.
func parseClock(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	s = strings.Replace(s, ",", ".", 1) // accept SRT comma
	dot := strings.LastIndex(s, ".")
	var msPart string
	if dot >= 0 {
		msPart = s[dot+1:]
		s = s[:dot]
	}
	parts := strings.Split(s, ":")
	var h, m, sec int64
	var err error
	switch len(parts) {
	case 3:
		if h, err = parseInt(parts[0]); err != nil {
			return 0, false
		}
		if m, err = parseInt(parts[1]); err != nil {
			return 0, false
		}
		if sec, err = parseInt(parts[2]); err != nil {
			return 0, false
		}
	case 2:
		if m, err = parseInt(parts[0]); err != nil {
			return 0, false
		}
		if sec, err = parseInt(parts[1]); err != nil {
			return 0, false
		}
	default:
		return 0, false
	}
	var msec int64
	if msPart != "" {
		// Right-pad/truncate to 3 digits (".5" → 500ms).
		for len(msPart) < 3 {
			msPart += "0"
		}
		if msec, err = parseInt(msPart[:3]); err != nil {
			return 0, false
		}
	}
	return ((h*60+m)*60+sec)*1000 + msec, true
}

func parseInt(s string) (int64, error) { return strconv.ParseInt(strings.TrimSpace(s), 10, 64) }

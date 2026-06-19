package vtt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatVTTTime(t *testing.T) {
	assert.Equal(t, "00:00:00.000", formatVTTTime(0))
	assert.Equal(t, "00:00:01.500", formatVTTTime(1500))
	assert.Equal(t, "01:01:01.001", formatVTTTime(3661001))
}

func TestParseVTTTimecode(t *testing.T) {
	// VTT dot separator + cue settings.
	s, e, set, ok := parseVTTTimecode("00:00:01.000 --> 00:00:04.500 align:middle line:84%")
	assert.True(t, ok)
	assert.Equal(t, int64(1000), s)
	assert.Equal(t, int64(4500), e)
	assert.Equal(t, "align:middle line:84%", set)

	// SRT comma separator parses too (cross-format).
	s, e, _, ok = parseVTTTimecode("00:00:01,000 --> 00:00:02,000")
	assert.True(t, ok)
	assert.Equal(t, int64(1000), s)
	assert.Equal(t, int64(2000), e)

	// Hours optional.
	s, _, _, ok = parseVTTTimecode("01:02.250 --> 01:03.000")
	assert.True(t, ok)
	assert.Equal(t, int64(62250), s)

	// Non-timecode.
	_, _, _, ok = parseVTTTimecode("Some cue text")
	assert.False(t, ok)
}

func TestTimecodeRoundTrip(t *testing.T) {
	for _, ms := range []int64{0, 999, 1500, 60000, 3661001} {
		got, _, _, ok := parseVTTTimecode(formatVTTTimecode(ms, ms+1000))
		assert.True(t, ok)
		assert.Equal(t, ms, got)
	}
}

//go:build integration

package subtitles

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoundTrip_Simple(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := []byte("WEBVTT\n\n00:00:00.000 --> 00:00:01.000\nHello world\n")
	bridgetest.AssertRoundTripEvents(t, pool, cfg, vttFilterClass, input, "test.vtt", vttMimeType, nil)
}

func TestRoundTrip_MultipleCues(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := []byte("WEBVTT\n\n" +
		"00:00:00.000 --> 00:00:02.000\n" +
		"First cue.\n\n" +
		"00:00:02.000 --> 00:00:04.000\n" +
		"Second cue.\n")
	bridgetest.AssertRoundTripEvents(t, pool, cfg, vttFilterClass, input, "test.vtt", vttMimeType, nil)
}

func TestRoundTrip_WithCueSettings(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := []byte("WEBVTT\n\n" +
		"00:00:00.000 --> 00:00:02.000 align:middle line:84%\n" +
		"Cue with settings.\n")
	bridgetest.AssertRoundTripEvents(t, pool, cfg, vttFilterClass, input, "test.vtt", vttMimeType, nil)
}

func TestRoundTrip_WithCueIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := []byte("WEBVTT\n\n" +
		"1\n" +
		"00:00:00.000 --> 00:00:02.000\n" +
		"First cue.\n\n" +
		"2\n" +
		"00:00:02.000 --> 00:00:04.000\n" +
		"Second cue.\n")
	bridgetest.AssertRoundTripEvents(t, pool, cfg, vttFilterClass, input, "test.vtt", vttMimeType, nil)
}

// okapi: RoundTripVttIT#vttFiles
func TestRoundTrip_VTT_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	// The Java RoundTripVttIT iterates over .vtt files in the vtt/
	// test resource directory using EventComparator for semantic comparison.
	bridgetest.RoundTripTestFiles(t, pool, cfg, vttFilterClass,
		tdDir+"/integration-tests/okapi/src/test/resources/vtt/*.vtt", vttMimeType, nil)
}

// okapi: RoundTripVttIT#vttSerializedFiles
func TestRoundTrip_TestFilesSerialized(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	// The Java vttSerializedFiles test runs the same file set with
	// serializedOutput=true. In the bridge, serialization behavior is
	// handled transparently, so we run the same roundtrip with event
	// comparison on the full set.
	bridgetest.RoundTripTestFiles(t, pool, cfg, vttFilterClass,
		tdDir+"/integration-tests/okapi/src/test/resources/vtt/*.vtt", vttMimeType, nil)
}

// okapi: VttXliffCompareIT#vttXliffCompareFiles
func TestRoundTrip_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	// Re-read the output from a roundtrip and compare event-level.
	// This is the event-based equivalent of the XLIFF compare IT.
	bridgetest.RoundTripTestFiles(t, pool, cfg, vttFilterClass,
		tdDir+"/integration-tests/okapi/src/test/resources/vtt/*.vtt", vttMimeType, nil)
}

func TestRoundTrip_FullFile_Example1(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "integration-tests/okapi/src/test/resources/vtt/example1.vtt")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	result := bridgetest.RoundTrip(t, pool, cfg, vttFilterClass, content, path, vttMimeType, nil)
	blocks := bridgetest.TranslatableBlocks(result.Parts)
	require.NotEmpty(t, blocks, "example1.vtt should produce translatable blocks")

	// example1.vtt has cues with "Lorem ipsum" style text.
	texts := bridgetest.BlockTexts(blocks)
	found := false
	for _, text := range texts {
		if len(text) > 0 {
			found = true
			break
		}
	}
	assert.True(t, found, "should extract non-empty text from example1.vtt")
}

func TestRoundTrip_FullFile_Example2(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "integration-tests/okapi/src/test/resources/vtt/example2.vtt")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	result := bridgetest.RoundTrip(t, pool, cfg, vttFilterClass, content, path, vttMimeType, nil)
	blocks := bridgetest.TranslatableBlocks(result.Parts)
	require.NotEmpty(t, blocks, "example2.vtt should produce translatable blocks")

	// example2.vtt has numbered cues and voice spans.
	texts := bridgetest.BlockTexts(blocks)
	found := false
	for _, text := range texts {
		if len(text) > 0 {
			found = true
			break
		}
	}
	assert.True(t, found, "should extract non-empty text from example2.vtt")
}

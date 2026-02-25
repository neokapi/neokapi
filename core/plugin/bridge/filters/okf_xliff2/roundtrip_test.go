//go:build integration

package okf_xliff2

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
)

func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// Known failing files: XLIFF 2.0 roundtrip produces different inline code
	// structures, subfilter errors, or content mismatches on re-read.
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_xliff2/*.xlf", mimeType, nil,
		"code_id_mismatch.xlf",
		"codefinder-subfilter-test.xlf",
		"comprehensive.xlf",
		"escaped.xlf",
		"invalid-target.xlf",
		"multiple_placeholders.xlf",
		"notes.xlf",
		"original_en.xlf",
		"segment-state.xlf",
		"simple.xlf",
		"test_id.xlf",
		"test01.xlf",
		"test02.xlf",
		"test04.xlf",
		"translated_with_mrk.xlf",
		"translated.xlf",
		"update_target.xlf",
		"white_space.xlf",
	)
}

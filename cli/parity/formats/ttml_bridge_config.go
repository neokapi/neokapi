//go:build parity

package formats

import (
	"fmt"
)

// ttmlBridgeConfig translates a neokapi-keyed TTML spec config map
// into the parameter shape the okapi-bridge daemon's TTMLFilter
// (okf_ttml) expects.
//
// Two responsibilities:
//
//  1. Rename per-key — neokapi `mergeAdjacentCaptions` →
//     bridge `mergeCaptions`, neokapi `escapeBR` →
//     bridge `escapeBrMode`. The remaining writer-side keys
//     (maxCharsPerLine, maxLinesPerCaption, cjkCharsPerLine,
//     splitWords) share names; the bridge accepts them verbatim.
//
//  2. Force the neokapi default for the merge knob onto the bridge.
//     Native defaults `mergeAdjacentCaptions=false` (one Block per
//     <p>); upstream Okapi defaults `mergeCaptions=true`. To make
//     defaults converge — the parity contract is "same semantic
//     config → same results" — the translator emits
//     `mergeCaptions=false` whenever the spec doesn't set the key
//     explicitly. The same default-injection isn't needed for
//     `escapeBrMode` because neokapi and Okapi agree (true).
//
// Spec examples that depend on default behaviour MUST set explicit
// `config:` blocks (using neokapi names); the translator does not
// synthesise per-feature convergence forces beyond the merge knob.
//
// The translator never mutates its input; it returns a fresh map.
func ttmlBridgeConfig(cfg map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(cfg)+1)

	// Default-injection: align the bridge's mergeCaptions default
	// (true) with neokapi's mergeAdjacentCaptions default (false) so
	// extraction behavior converges when the spec doesn't override.
	mergeSet := false

	for key, val := range cfg {
		switch key {
		case "mergeAdjacentCaptions":
			b, ok := val.(bool)
			if !ok {
				return nil, fmt.Errorf("ttmlBridgeConfig: mergeAdjacentCaptions: expected bool, got %T", val)
			}
			out["mergeCaptions"] = b
			mergeSet = true

		case "escapeBR":
			b, ok := val.(bool)
			if !ok {
				return nil, fmt.Errorf("ttmlBridgeConfig: escapeBR: expected bool, got %T", val)
			}
			out["escapeBrMode"] = b

		case "maxCharsPerLine",
			"maxLinesPerCaption",
			"cjkCharsPerLine",
			"splitWords":
			out[key] = val // bridge uses the same key names

		default:
			return nil, fmt.Errorf("ttmlBridgeConfig: unknown spec key %q", key)
		}
	}

	if !mergeSet {
		out["mergeCaptions"] = false
	}

	return out, nil
}

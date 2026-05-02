//go:build parity

package roundtrip

import (
	"errors"
	"testing"

	"github.com/neokapi/neokapi/cli/parity"
	"github.com/neokapi/neokapi/core/model"
)

// BridgeEngine drives the okapi-bridge daemon's Process RPC in
// read-write mode: the daemon reads via the requested Okapi filter,
// each Block streams to Go where pseudo-translation is applied, and
// the daemon's writer thread merges the modified parts back into
// the output document.
type BridgeEngine struct {
	// FilterClass is the Okapi filter class name (e.g. okf_html,
	// okf_plaintext, okf_po). Required.
	FilterClass string

	// MimeType is forwarded to the daemon. Optional — most filters
	// detect from the input path or filename.
	MimeType string

	// FilterParams is the daemon-side parameter map (already
	// translated to Okapi key names if the format has a
	// BridgeConfig translator in cli/parity/formats/).
	FilterParams map[string]string
}

// Name returns "bridge".
func (e *BridgeEngine) Name() string { return "bridge" }

// Available returns nil if the bridge sandbox can be acquired. The
// real check happens lazily inside parity.AcquireBridgeDaemon — this
// method only flags configuration errors.
func (e *BridgeEngine) Available() error {
	if e.FilterClass == "" {
		return errors.New("FilterClass is required")
	}
	return nil
}

// RoundTrip drives the daemon's Process RPC and returns the merged
// document bytes. Pseudo-translation is applied on each Block before
// it's echoed back over the stream.
func (e *BridgeEngine) RoundTrip(t *testing.T, in Input, spec PseudoSpec) []byte {
	t.Helper()
	req := parity.BridgeRequest{
		FilterClass:  e.FilterClass,
		InputBytes:   in.Bytes,
		SourceLocale: spec.SrcLocale(),
		TargetLocale: spec.TgtLocale(),
		MimeType:     e.MimeType,
		FilterParams: e.FilterParams,
		Transform: func(b *model.Block) {
			applyPseudoToBlock(b, spec)
		},
	}
	res := parity.RunBridgeRoundTrip(t, req)
	if len(res.Output) == 0 {
		t.Fatal("BridgeEngine: daemon returned empty output")
	}
	return res.Output
}

// Compile-time interface check.
var _ Engine = (*BridgeEngine)(nil)

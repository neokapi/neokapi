//go:build parity

package roundtrip

import (
	"errors"
	"os"
	"path/filepath"
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

	// ForcePseudoSourceBase mirrors the xliff2 case where okapi's
	// pipeline unconditionally pseudo-translates the source rather
	// than the existing target. Set to true for filters whose okapi
	// reference behavior overwrites the on-disk target verbatim.
	ForcePseudoSourceBase bool
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
//
// When the input has companion files (e.g. an XML fixture that XLinks
// to a sibling rules file), the bridge can't resolve those references
// from the inline byte stream alone — okapi resolves them on disk.
// Stage input + companions in a tmpDir and pass an absolute path so
// xinclude / XLink / DTD entities resolve against real sibling files.
func (e *BridgeEngine) RoundTrip(t *testing.T, in Input, spec PseudoSpec) []byte {
	t.Helper()
	req := parity.BridgeRequest{
		FilterClass:  e.FilterClass,
		SourceLocale: spec.SrcLocale(),
		TargetLocale: spec.TgtLocale(),
		MimeType:     e.MimeType,
		FilterParams: e.FilterParams,
		Transform: func(b *model.Block) {
			// For ForcePseudoSourceBase formats (xliff2), source is
			// the pseudo base only when the block has no existing
			// target for the requested locale. When a target for the
			// locale already exists (file's trgLang matches our
			// target), pseudo it in place — that's what okapi does.
			tgt := model.LocaleID(spec.TgtLocale())
			forceSrc := e.ForcePseudoSourceBase
			if forceSrc {
				if existing := b.Target(tgt); existing != nil && runsHaveText(existing.Runs) {
					forceSrc = false
				}
			}
			applyPseudoToBlockOpts(b, spec, forceSrc)
		},
	}
	if len(in.Companions) > 0 {
		tmpDir := t.TempDir()
		inputPath := filepath.Join(tmpDir, in.Filename)
		if err := os.WriteFile(inputPath, in.Bytes, 0o644); err != nil {
			t.Fatalf("BridgeEngine: write input: %v", err)
		}
		for name, data := range in.Companions {
			if err := os.WriteFile(filepath.Join(tmpDir, name), data, 0o644); err != nil {
				t.Fatalf("BridgeEngine: write companion %q: %v", name, err)
			}
		}
		req.InputPath = inputPath
	} else {
		req.InputBytes = in.Bytes
	}
	res := parity.RunBridgeRoundTrip(t, req)
	if len(res.Output) == 0 {
		t.Fatal("BridgeEngine: daemon returned empty output")
	}
	return res.Output
}

// Compile-time interface check.
var _ Engine = (*BridgeEngine)(nil)

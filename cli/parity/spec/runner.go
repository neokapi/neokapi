//go:build parity

// Package spec drives a format Spec through the okapi-bridge daemon
// and compares the result against the native reader, on a per-feature
// basis. The same Spec authored under core/formats/<fmt>/spec.yaml
// powers both the always-on native runner and this parity layer.
//
// One Outcome row per feature × example reaches the parity report
// under Kind="format-spec-feature" so the dashboard renders a
// per-feature parity status alongside per-feature native status.
package spec

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/cli/parity"
	"github.com/neokapi/neokapi/core/format"
	formatspec "github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/model"
)

// Spec / LoadSpec are aliases of the format/spec types so callers
// have a single import.
type Spec = formatspec.Spec

var LoadSpec = formatspec.Load

// ParityRunner drives a Spec through both the native reader and the
// okapi-bridge daemon for every Example, comparing block text to
// confirm the spec contract is satisfied on both sides equally.
//
// Native and bridge are evaluated independently against the spec's
// assertions so a divergence on one side doesn't mask a divergence on
// the other. A third check (CompareBlockText) compares bridge output
// to native output on the same input — the cross-implementation
// parity signal.
type ParityRunner struct {
	Spec      *Spec
	NewReader func(variant string) format.DataFormatReader

	// BridgeConfig translates the merged spec config (neokapi-keyed)
	// into the bridge daemon's expected key/value shape. Returns the
	// input untouched if nil. Use this when the bridge filter expects
	// different parameter names than the canonical neokapi ones (e.g.
	// csv's "separator" vs Okapi's "fieldDelimiter") or when neokapi
	// defaults need to be forced onto the bridge to make defaults
	// converge (e.g. neokapi skips headers by default; bridge extracts
	// them, so the translator adds "sendHeaderMode: 0").
	//
	// Native config receives the original neokapi-keyed map. Only the
	// bridge dispatch goes through translation. spec.yaml stays
	// monolingual in neokapi terms.
	BridgeConfig func(cfg map[string]any) (map[string]any, error)
}

// Run drives every Feature × Example as a parity subtest. Outcomes
// reach the parity report under Kind="format-spec-feature" so the
// contract-audit dashboard can render per-feature parity status.
//
// Subfilter specs (kind: subfilter) skip the parity bridge entirely:
// the okapi-bridge daemon dispatches by top-level filter id, and
// subfilters have no standalone dispatch path. Their behavior is
// exercised through the native runner and through their parents'
// specs. The runner emits one `subfilter` skip Outcome per example
// so the dashboard still surfaces the feature × example coverage.
func (r *ParityRunner) Run(t *testing.T) {
	t.Helper()
	if r.Spec == nil {
		t.Fatal("ParityRunner: Spec is nil")
	}
	if r.NewReader == nil {
		t.Fatal("ParityRunner: NewReader is nil")
	}
	if r.Spec.IsSubfilter() {
		for _, feat := range r.Spec.Features {
			for _, ex := range feat.Examples {
				parity.Report(t, parity.Outcome{
					Kind:   "format-spec-feature",
					ID:     r.Spec.Format + "::" + feat.ID + "::" + ex.Name,
					Name:   t.Name() + "/" + feat.ID + "/" + ex.Name,
					Mode:   "subfilter",
					Status: "skip",
					Detail: "subfilter — not dispatched standalone by okapi-bridge",
				})
			}
		}
		t.Skipf("subfilter spec %q — parity bridge runner skipped (no standalone dispatch path)", r.Spec.Format)
		return
	}
	for _, feat := range r.Spec.Features {

		t.Run(feat.ID, func(t *testing.T) {
			for _, ex := range feat.Examples {

				t.Run(ex.Name, func(t *testing.T) {
					r.runExample(t, feat, ex)
				})
			}
		})
	}
}

func (r *ParityRunner) runExample(t *testing.T, feat formatspec.Feature, ex formatspec.Example) {
	t.Helper()

	status := "pass"
	detail := ""
	defer func() {
		parity.Report(t, parity.Outcome{
			Kind:   "format-spec-feature",
			ID:     r.Spec.Format + "::" + feat.ID + "::" + ex.Name,
			Name:   t.Name(),
			Mode:   bridgeMode(ex),
			Status: status,
			Detail: detail,
		})
	}()

	// class: invalid cases assert the NATIVE reader rejects malformed input
	// cleanly (NativeRunner.runInvalid). There is no successful extraction to
	// compare head-to-head, so the bridge parity runner skips them — robustness
	// is the native runner's concern, not a cross-implementation parity claim.
	if ex.CaseClass() == formatspec.ClassInvalid {
		status = "skip"
		detail = "class: invalid — native-only robustness case (no head-to-head extraction)"
		t.Skipf("class: invalid case %q — parity skips (native-only)", ex.CaseID())
		return
	}

	input, err := formatspec.ResolveInput(r.Spec, ex)
	if err != nil {
		if strings.HasPrefix(ex.InputFile, "okapi:") {
			status = "skip"
			detail = err.Error()
			t.Skipf("input not available: %v", err)
			return
		}
		status = "fail"
		detail = "resolve input: " + err.Error()
		t.Fatalf("resolve input: %v", err)
	}

	cfg := formatspec.MergeConfig(feat.Config, ex.Config)
	bridgeCfg := cfg
	if r.BridgeConfig != nil {
		translated, err := r.BridgeConfig(cfg)
		if err != nil {
			status = "fail"
			detail = "bridge config translate: " + err.Error()
			t.Fatalf("bridge config translate: %v", err)
		}
		bridgeCfg = translated
	}
	bridgeReq := parity.BridgeRequest{
		// BridgeClass()/BridgeConfigID let a config-preset spec.yaml dispatch
		// to a base filter plus a named Okapi config (#852). Empty for every
		// current spec, so this is identical to dispatching on Format today.
		FilterClass:  r.Spec.BridgeClass(),
		ConfigID:     r.Spec.BridgeConfigID,
		InputBytes:   input,
		MimeType:     mimeForVariant(r.Spec, ex.Variant),
		FilterParams: parity.StringifyParams(bridgeCfg),
	}

	bridge, err := parity.TryRunBridge(t, bridgeReq)
	if err != nil {
		if ex.ExpectedFail != "" {
			status = "expected_fail"
			detail = "bridge error: " + err.Error()
			t.Logf("expected_fail: bridge error %v", err)
			return
		}
		status = "fail"
		detail = "bridge: " + err.Error()
		t.Errorf("bridge: %v", err)
		return
	}

	bridgeFails := formatspec.EvalAssertions(bridge, ex.Assertions)

	// Bridge-only examples skip the native side and compare only
	// against the bridge output.
	if ex.BridgeOnly {
		switch {
		case ex.ExpectedFail != "":
			status = "expected_fail"
			detail = ex.ExpectedFail
			for _, msg := range bridgeFails {
				t.Logf("expected_fail (%s): bridge: %s", ex.ExpectedFail, msg)
			}
		case len(bridgeFails) > 0:
			status = "fail"
			detail = "bridge violates spec: " + strings.Join(bridgeFails, "; ")
			for _, msg := range bridgeFails {
				t.Errorf("bridge: %s", msg)
			}
		}
		return
	}

	native, nativeErr := r.runNative(feat, ex, input)
	if nativeErr != nil {
		if ex.ExpectedFail != "" {
			status = "expected_fail"
			detail = "native error: " + nativeErr.Error()
			t.Logf("expected_fail: native error %v", nativeErr)
			return
		}
		status = "fail"
		detail = "native: " + nativeErr.Error()
		t.Errorf("native: %v", nativeErr)
		return
	}

	nativeFails := formatspec.EvalAssertions(native, ex.Assertions)

	// Bytewise parity (bridge text == native text) is a stricter
	// audit than the spec contract. We compute it but downgrade to
	// a logged warning when both sides pass the spec independently —
	// the spec is the authoritative contract. Examples that demand
	// byte-equivalent output set ParityStrict.
	bridgeTexts := formatspec.BlockTexts(bridge)
	nativeTexts := formatspec.BlockTexts(native)
	parityOK := slicesEqual(bridgeTexts, nativeTexts)

	if ex.ExpectedFail != "" {
		status = "expected_fail"
		detail = ex.ExpectedFail
		for _, msg := range bridgeFails {
			t.Logf("expected_fail (%s): bridge: %s", ex.ExpectedFail, msg)
		}
		for _, msg := range nativeFails {
			t.Logf("expected_fail (%s): native: %s", ex.ExpectedFail, msg)
		}
		if !parityOK {
			t.Logf("expected_fail (%s): bridge != native (bytewise)", ex.ExpectedFail)
		}
		return
	}

	switch {
	case len(bridgeFails) > 0 && len(nativeFails) > 0:
		status = "fail"
		detail = "assertions fail on both sides"
		for _, msg := range bridgeFails {
			t.Errorf("bridge: %s", msg)
		}
		for _, msg := range nativeFails {
			t.Errorf("native: %s", msg)
		}
	case len(bridgeFails) > 0:
		status = "fail"
		detail = "bridge violates spec: " + strings.Join(bridgeFails, "; ")
		for _, msg := range bridgeFails {
			t.Errorf("bridge: %s", msg)
		}
	case len(nativeFails) > 0:
		status = "fail"
		detail = "native violates spec: " + strings.Join(nativeFails, "; ")
		for _, msg := range nativeFails {
			t.Errorf("native: %s", msg)
		}
	case !parityOK && ex.ParityStrict:
		status = "fail"
		detail = "bridge != native (parity_strict)"
		t.Errorf("bytewise parity mismatch:\n  bridge: %v\n  native: %v", bridgeTexts, nativeTexts)
	case !parityOK:
		// Both sides honor the spec but emit different text. Record
		// as a parity-warn so the dashboard can flag it without
		// failing CI.
		status = "parity_warn"
		detail = "bridge != native (bytewise) — both pass spec assertions independently"
		t.Logf("parity_warn: bridge=%v native=%v", bridgeTexts, nativeTexts)
	}
}

// slicesEqual reports whether two string slices are element-wise
// equal. Local helper so the runner doesn't pull in slices.Equal
// (and to keep the comparison grep-friendly).
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (r *ParityRunner) runNative(feat formatspec.Feature, ex formatspec.Example, input []byte) ([]*model.Part, error) {
	reader := r.NewReader(ex.Variant)
	// Parity measures faithfulness to the Okapi bridge, which has no notion of
	// surfacing non-translatable contextual content (code/verbatim/etc.) as
	// content blocks. Native readers default that surfacing ON (an ingestion
	// convenience), so for the head-to-head we force it OFF — the matching
	// semantic config — leaving such content in skeleton exactly as before.
	if c := reader.Config(); c != nil {
		if d, ok := c.(interface{ SetExtractNonTranslatableContent(bool) }); ok {
			d.SetExtractNonTranslatableContent(false)
		}
	}
	cfg := formatspec.MergeConfig(feat.Config, ex.Config)
	if len(cfg) > 0 {
		if c := reader.Config(); c != nil {
			if err := c.ApplyMap(cfg); err != nil {
				return nil, err
			}
		}
	}
	return formatspec.ReadParts(reader, input)
}

func bridgeMode(ex formatspec.Example) string {
	if ex.BridgeOnly {
		return "bridge-only"
	}
	return "head-to-head"
}

func mimeForVariant(s *Spec, variantID string) string {
	if variantID != "" {
		for _, v := range s.Variants {
			if v.ID == variantID && v.MimeType != "" {
				return v.MimeType
			}
		}
	}
	return s.MimeType
}

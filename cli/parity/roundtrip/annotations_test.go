//go:build parity

package roundtrip_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/cli/parity/roundtrip"
)

// TestAnnotations_LoaderFindsOpenxml verifies the loader picks up the
// openxml parity-annotations.yaml file checked into core/formats/openxml/
// and exposes the expected per-fixture metadata.
//
// The annotation system replaces the legacy fileSkip map and powers the
// /parity/fixtures dashboard's severity badges + issue links, so a
// loader regression would silently make the dashboard incomplete. This
// test pins the contract: severity classifies the divergence, issue
// links a GitHub follow-up, summary explains why.
func TestAnnotations_LoaderFindsOpenxml(t *testing.T) {
	roundtrip.ResetAnnotations()
	ann, ok := roundtrip.LookupAnnotation("openxml", "delTextAmp.docx")
	if !ok {
		t.Fatal("expected annotation for openxml/delTextAmp.docx")
	}
	if ann.Severity != "bug" {
		t.Errorf("severity: got %q want %q", ann.Severity, "bug")
	}
	if ann.Issue != 597 {
		t.Errorf("issue: got %d want 597", ann.Issue)
	}
	if !strings.Contains(ann.Summary, "spacing") {
		t.Errorf("summary doesn't mention spacing: %q", ann.Summary)
	}
}

// TestAnnotations_LookupSkipReturnsMigratedDirective verifies that an
// entry with a skip: block (the schema slot that replaces the in-code
// fileSkip map) round-trips through LookupSkip. This is the contract
// coverage_test.go now relies on at every fixture.
func TestAnnotations_LookupSkipReturnsMigratedDirective(t *testing.T) {
	roundtrip.ResetAnnotations()
	skip, ok := roundtrip.LookupSkip("ttml", "example1.ttml")
	if !ok {
		t.Fatal("expected skip directive for ttml/example1.ttml")
	}
	if len(skip.Engines) != 1 || skip.Engines[0] != "native" {
		t.Errorf("engines: got %v want [native]", skip.Engines)
	}
	if !strings.Contains(skip.Reason, "encoding/xml") {
		t.Errorf("reason doesn't mention encoding/xml: %q", skip.Reason)
	}
}

// TestAnnotations_LookupMissingReturnsFalse pins the "unannotated"
// behavior — every divergent fixture without an annotation must
// surface as ok=false so the CI gate can flag it.
func TestAnnotations_LookupMissingReturnsFalse(t *testing.T) {
	roundtrip.ResetAnnotations()
	if _, ok := roundtrip.LookupAnnotation("openxml", "this-fixture-does-not-exist.docx"); ok {
		t.Error("expected no annotation for unknown fixture")
	}
	if _, ok := roundtrip.LookupSkip("openxml", "this-fixture-does-not-exist.docx"); ok {
		t.Error("expected no skip directive for unknown fixture")
	}
}

// TestAnnotations_SeveritiesValid pins the closed set of severity
// values. Anything outside this set fails — the dashboard's severity
// filter and legend assume these exact strings.
func TestAnnotations_SeveritiesValid(t *testing.T) {
	roundtrip.ResetAnnotations()
	valid := map[string]bool{
		"bug":                 true,
		"cosmetic":            true,
		"native-more-correct": true,
		"fixture-bug":         true,
		"unknown":             true,
		"":                    true, // empty = unset, treated as unknown
	}
	all, err := roundtrip.AllAnnotations()
	if err != nil {
		t.Fatalf("AllAnnotations: %v", err)
	}
	for format, perFmt := range all {
		for fixture, ann := range perFmt {
			if !valid[ann.Severity] {
				t.Errorf("%s/%s: invalid severity %q", format, fixture, ann.Severity)
			}
		}
	}
}

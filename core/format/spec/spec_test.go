package spec

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestKindDefaultsToTopLevel(t *testing.T) {
	s := &Spec{Format: "okf_x", Features: []Feature{{ID: "f", Examples: []Example{{Name: "e"}}}}}
	if s.IsSubfilter() {
		t.Fatalf("empty Kind should not register as subfilter")
	}
}

func TestIsSubfilter(t *testing.T) {
	s := &Spec{Kind: KindSubfilter}
	if !s.IsSubfilter() {
		t.Fatalf("KindSubfilter should report IsSubfilter() == true")
	}
}

func TestValidateRejectsUnknownKind(t *testing.T) {
	s := &Spec{
		Format:   "okf_x",
		Kind:     Kind("nonsense"),
		Features: []Feature{{ID: "f", Examples: []Example{{Name: "e", InputXML: "x", Assertions: Assertions{BlockCount: IntPtr(1)}}}}},
	}
	if err := s.Validate(); err == nil {
		t.Fatalf("Validate should reject unknown kind")
	}
}

func TestValidateAcceptsKnownKinds(t *testing.T) {
	for _, k := range []Kind{"", KindTopLevel, KindSubfilter} {
		s := &Spec{
			Format:   "okf_x",
			Kind:     k,
			Features: []Feature{{ID: "f", Examples: []Example{{Name: "e", InputXML: "x", Assertions: Assertions{BlockCount: IntPtr(1)}}}}},
		}
		if err := s.Validate(); err != nil {
			t.Fatalf("Validate(kind=%q): unexpected err: %v", k, err)
		}
	}
}

// TestOriginAndSpecRefsRoundTrip verifies the new provenance fields
// (Feature.SpecRefs, Example.Origin) survive a YAML marshal/unmarshal
// cycle without loss. Keeps the fields wired correctly as the spec
// surface evolves.
func TestOriginAndSpecRefsRoundTrip(t *testing.T) {
	original := Spec{
		Format:   "okf_demo",
		MimeType: "text/x-demo",
		Features: []Feature{
			{
				ID:   "demo_feature",
				Name: "Demo feature",
				SpecRefs: []string{
					"CommonMark §6.7 Soft line breaks",
					"https://spec.commonmark.org/0.31.2/#soft-line-breaks",
				},
				Examples: []Example{
					{
						Name:     "authored_example",
						InputXML: "Hello world\n",
						Origin:   "authored: minimal CommonMark §6.7 soft-break case",
						Assertions: Assertions{
							BlockCount:     IntPtr(1),
							FirstBlockText: StrPtr("Hello world"),
						},
					},
					{
						Name:     "fixture_example",
						InputXML: "Other line\n",
						Origin:   "okapi-fixture: MarkdownFilterTest#testEmphasisAcrossLines",
						Assertions: Assertions{
							BlockCount: IntPtr(1),
						},
					},
					{
						Name:     "real_world_example",
						InputXML: "Real text\n",
						Origin:   "real-world: Excalidraw locales/en.json",
						Assertions: Assertions{
							BlockCount: IntPtr(1),
						},
					},
				},
			},
		},
	}

	data, err := yaml.Marshal(&original)
	if err != nil {
		t.Fatalf("yaml.Marshal: unexpected err: %v", err)
	}

	var roundTripped Spec
	if err := yaml.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("yaml.Unmarshal: unexpected err: %v", err)
	}

	feat := roundTripped.Features[0]
	wantRefs := []string{
		"CommonMark §6.7 Soft line breaks",
		"https://spec.commonmark.org/0.31.2/#soft-line-breaks",
	}
	if len(feat.SpecRefs) != len(wantRefs) {
		t.Fatalf("SpecRefs: got %d entries, want %d (yaml=%s)", len(feat.SpecRefs), len(wantRefs), string(data))
	}
	for i, want := range wantRefs {
		if feat.SpecRefs[i] != want {
			t.Errorf("SpecRefs[%d]: got %q, want %q", i, feat.SpecRefs[i], want)
		}
	}

	wantOrigins := []string{
		"authored: minimal CommonMark §6.7 soft-break case",
		"okapi-fixture: MarkdownFilterTest#testEmphasisAcrossLines",
		"real-world: Excalidraw locales/en.json",
	}
	for i, want := range wantOrigins {
		got := feat.Examples[i].Origin
		if got != want {
			t.Errorf("Example[%d].Origin: got %q, want %q", i, got, want)
		}
	}
}

// TestDivergenceKindRoundTrip verifies the optional divergence_kind
// override survives a YAML marshal/unmarshal cycle and stays empty when
// omitted (so contract-audit falls back to its detail-text heuristic).
func TestDivergenceKindRoundTrip(t *testing.T) {
	original := Spec{
		Format: "okf_demo",
		Features: []Feature{
			{
				ID: "f",
				Examples: []Example{
					{
						Name:           "explicit",
						InputXML:       "x",
						ExpectedFail:   "bridge != native (bytewise)",
						DivergenceKind: "okapi-bug",
						Assertions:     Assertions{BlockCount: IntPtr(1)},
					},
					{
						Name:       "implicit",
						InputXML:   "y",
						Assertions: Assertions{BlockCount: IntPtr(1)},
					},
				},
			},
		},
	}
	data, err := yaml.Marshal(&original)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	var rt Spec
	if err := yaml.Unmarshal(data, &rt); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if got := rt.Features[0].Examples[0].DivergenceKind; got != "okapi-bug" {
		t.Errorf("DivergenceKind round-trip: got %q, want okapi-bug (yaml=%s)", got, data)
	}
	if got := rt.Features[0].Examples[1].DivergenceKind; got != "" {
		t.Errorf("DivergenceKind omitted: got %q, want empty", got)
	}
}

// TestOriginAndSpecRefsAreOptional confirms that omitting both new
// fields keeps a spec valid — existing specs without them still load.
func TestOriginAndSpecRefsAreOptional(t *testing.T) {
	s := &Spec{
		Format: "okf_legacy",
		Features: []Feature{
			{
				ID: "f",
				Examples: []Example{
					{
						Name:       "e",
						InputXML:   "x",
						Assertions: Assertions{BlockCount: IntPtr(1)},
					},
				},
			},
		},
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("Validate: legacy spec without origin/spec_refs should still be valid, got: %v", err)
	}
	if len(s.Features[0].SpecRefs) != 0 {
		t.Errorf("SpecRefs: expected empty default, got %v", s.Features[0].SpecRefs)
	}
	if s.Features[0].Examples[0].Origin != "" {
		t.Errorf("Origin: expected empty default, got %q", s.Features[0].Examples[0].Origin)
	}
}

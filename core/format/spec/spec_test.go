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
		Features: []Feature{{ID: "f", Examples: []Example{{Name: "e", InputXML: "x", Assertions: Assertions{BlockCount: new(1)}}}}},
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
			Features: []Feature{{ID: "f", Examples: []Example{{Name: "e", InputXML: "x", Assertions: Assertions{BlockCount: new(1)}}}}},
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
							BlockCount:     new(1),
							FirstBlockText: new("Hello world"),
						},
					},
					{
						Name:     "fixture_example",
						InputXML: "Other line\n",
						Origin:   "okapi-fixture: MarkdownFilterTest#testEmphasisAcrossLines",
						Assertions: Assertions{
							BlockCount: new(1),
						},
					},
					{
						Name:     "real_world_example",
						InputXML: "Real text\n",
						Origin:   "real-world: Excalidraw locales/en.json",
						Assertions: Assertions{
							BlockCount: new(1),
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
						Assertions:     Assertions{BlockCount: new(1)},
					},
					{
						Name:       "implicit",
						InputXML:   "y",
						Assertions: Assertions{BlockCount: new(1)},
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

// specWithExample is a tiny helper: a one-feature spec wrapping a single
// example, used by the meta-schema validation tests.
func specWithExample(ex Example) *Spec {
	return &Spec{
		Format:   "okf_x",
		Features: []Feature{{ID: "f", Examples: []Example{ex}}},
	}
}

// TestValidateRejectsDuplicateCaseID proves the §8 id-uniqueness gate: two
// cases sharing an id within one spec is a malformed corpus.
func TestValidateRejectsDuplicateCaseID(t *testing.T) {
	s := &Spec{
		Format: "okf_x",
		Features: []Feature{{
			ID: "f",
			Examples: []Example{
				{Name: "a", ID: "AB12", InputXML: "x", Assertions: Assertions{BlockCount: new(1)}},
				{Name: "b", ID: "AB12", InputXML: "y", Assertions: Assertions{BlockCount: new(1)}},
			},
		}},
	}
	if err := s.Validate(); err == nil {
		t.Fatal("Validate should reject a duplicate case id")
	}
}

// TestValidateRejectsBadCaseID enforces the 4–6 alphanumeric id contract.
func TestValidateRejectsBadCaseID(t *testing.T) {
	for _, bad := range []string{"ab", "toolongid", "ab-2", "ab 2"} {
		s := specWithExample(Example{Name: "a", ID: bad, InputXML: "x"})
		if err := s.Validate(); err == nil {
			t.Errorf("Validate should reject malformed case id %q", bad)
		}
	}
}

// TestValidateRejectsUnknownClass enforces the class enum.
func TestValidateRejectsUnknownClass(t *testing.T) {
	s := specWithExample(Example{Name: "a", Class: "weird", InputXML: "x"})
	if err := s.Validate(); err == nil {
		t.Fatal("Validate should reject an unknown class")
	}
}

// TestValidateInvalidCaseRequiresError proves a class: invalid case must
// carry exactly an expected.error view (one fault per case).
func TestValidateInvalidCaseRequiresError(t *testing.T) {
	// invalid without expected.error → rejected
	s := specWithExample(Example{Name: "bad", Class: ClassInvalid, InputXML: "x"})
	if err := s.Validate(); err == nil {
		t.Fatal("Validate should reject class: invalid with no expected.error")
	}
	// invalid carrying a second view → rejected (one fault per case)
	s = specWithExample(Example{
		Name: "bad", Class: ClassInvalid, InputXML: "x",
		Expected: &Expected{Error: &ErrorExpect{Category: "syntax"}, Blocks: `{"x":1}`},
	})
	if err := s.Validate(); err == nil {
		t.Fatal("Validate should reject a class: invalid case carrying a second view")
	}
	// invalid done right → accepted
	s = specWithExample(Example{
		Name: "bad", Class: ClassInvalid, InputXML: "x",
		Expected: &Expected{Error: &ErrorExpect{Category: "syntax"}},
	})
	if err := s.Validate(); err != nil {
		t.Fatalf("well-formed invalid case rejected: %v", err)
	}
}

// TestValidateRejectsErrorOnValidCase proves expected.error is invalid-only.
func TestValidateRejectsErrorOnValidCase(t *testing.T) {
	s := specWithExample(Example{
		Name: "a", Class: ClassValid, InputXML: "x",
		Expected: &Expected{Error: &ErrorExpect{Category: "syntax"}},
	})
	if err := s.Validate(); err == nil {
		t.Fatal("Validate should reject expected.error on a valid case")
	}
}

// TestValidateCiteRequiresSpecAndURL enforces the citation completeness gate.
func TestValidateCiteRequiresSpecAndURL(t *testing.T) {
	s := specWithExample(Example{Name: "a", InputXML: "x", Cite: &Citation{Spec: "whatwg-html"}})
	if err := s.Validate(); err == nil {
		t.Fatal("Validate should reject a cite missing url")
	}
	s = specWithExample(Example{Name: "a", InputXML: "x", Cite: &Citation{URL: "https://x"}})
	if err := s.Validate(); err == nil {
		t.Fatal("Validate should reject a cite missing spec")
	}
	s = specWithExample(Example{Name: "a", InputXML: "x", Cite: &Citation{Spec: "whatwg-html", URL: "https://x#y"}})
	if err := s.Validate(); err != nil {
		t.Fatalf("complete cite rejected: %v", err)
	}
}

// TestValidateRejectsBadRoundtripMode enforces the roundtrip mode enum.
func TestValidateRejectsBadRoundtripMode(t *testing.T) {
	s := specWithExample(Example{
		Name: "a", InputXML: "x",
		Expected: &Expected{Roundtrip: &Roundtrip{Mode: "loose"}},
	})
	if err := s.Validate(); err == nil {
		t.Fatal("Validate should reject an unknown roundtrip mode")
	}
}

// TestExtractedAssertionsBackCompat confirms a legacy inline-assertion example
// is reachable through ExtractedAssertions, and that expected.extracted wins
// when present.
func TestExtractedAssertionsBackCompat(t *testing.T) {
	legacy := Example{Name: "a", InputXML: "x", Assertions: Assertions{BlockCount: new(2)}}
	got := legacy.ExtractedAssertions()
	if got.BlockCount == nil || *got.BlockCount != 2 {
		t.Fatalf("legacy inline assertions not surfaced: %+v", got)
	}
	multi := Example{
		Name: "b", InputXML: "x",
		Assertions: Assertions{BlockCount: new(2)},
		Expected:   &Expected{Extracted: &Assertions{BlockCount: new(9)}},
	}
	got = multi.ExtractedAssertions()
	if got.BlockCount == nil || *got.BlockCount != 9 {
		t.Fatalf("expected.extracted should win over inline: %+v", got)
	}
}

// TestCaseClassAndIDDefaults confirms the effective-value helpers.
func TestCaseClassAndIDDefaults(t *testing.T) {
	ex := Example{Name: "human-name"}
	if ex.CaseClass() != ClassValid {
		t.Errorf("empty class should default to valid, got %q", ex.CaseClass())
	}
	if ex.CaseID() != "human-name" {
		t.Errorf("CaseID should fall back to Name, got %q", ex.CaseID())
	}
	ex.ID = "QF7K"
	if ex.CaseID() != "QF7K" {
		t.Errorf("CaseID should prefer ID, got %q", ex.CaseID())
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
						Assertions: Assertions{BlockCount: new(1)},
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

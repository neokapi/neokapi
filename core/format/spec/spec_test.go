package spec

import "testing"

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

package flow

import "testing"

func TestStepsSpecBindingLocators(t *testing.T) {
	t.Run("declared sink none", func(t *testing.T) {
		spec := &StepsSpec{Sink: "none", Steps: []FlowStep{{Tool: "qa"}}}
		if _, ok := spec.SourceLocator(); ok {
			t.Errorf("SourceLocator() ok = true, want false (no source declared)")
		}
		sink, ok := spec.SinkLocator()
		if !ok {
			t.Fatalf("SinkLocator() ok = false, want true")
		}
		if sink.Kind() != BindingNone {
			t.Errorf("sink kind = %q, want %q", sink.Kind(), BindingNone)
		}
	})

	t.Run("declared source store", func(t *testing.T) {
		spec := &StepsSpec{Source: "store:", Steps: []FlowStep{{Tool: "translate"}}}
		src, ok := spec.SourceLocator()
		if !ok {
			t.Fatalf("SourceLocator() ok = false, want true")
		}
		if src.Kind() != BindingStore {
			t.Errorf("source kind = %q, want %q", src.Kind(), BindingStore)
		}
	})

	t.Run("binding-agnostic flow declares nothing", func(t *testing.T) {
		spec := &StepsSpec{Steps: []FlowStep{{Tool: "translate"}}}
		if _, ok := spec.SourceLocator(); ok {
			t.Errorf("SourceLocator() ok = true, want false")
		}
		if _, ok := spec.SinkLocator(); ok {
			t.Errorf("SinkLocator() ok = true, want false")
		}
	})
}

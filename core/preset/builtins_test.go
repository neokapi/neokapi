package preset

import "testing"

func TestRegisterBuiltinsFrameworkPresets(t *testing.T) {
	reg := NewPresetRegistry()
	RegisterBuiltins(reg)

	want := []string{"neokapi-i18n", "react-i18next", "react-intl", "nextjs", "vue-i18n", "flutter", "angular"}
	for _, name := range want {
		p := reg.GetFrameworkPreset(name)
		if p == nil {
			t.Errorf("framework preset %q not registered", name)
			continue
		}
		if p.Name != name {
			t.Errorf("preset %q has Name %q", name, p.Name)
		}
		if p.Description == "" {
			t.Errorf("preset %q missing description", name)
		}
		if len(p.Mappings) == 0 {
			t.Errorf("preset %q has no mappings", name)
		}
		for _, m := range p.Mappings {
			if m.Local == "" || m.Format == "" || m.TargetPath == "" {
				t.Errorf("preset %q has an incomplete mapping: %+v", name, m)
			}
		}
		if len(p.Detect) == 0 {
			t.Errorf("preset %q has no detect patterns", name)
		}
	}
}

func TestNeokapiI18nPresetUsesKBF(t *testing.T) {
	p := neokapiI18nPreset()
	if p.Mappings[0].Format != "kbf" {
		t.Errorf("neokapi-i18n should extract to kbf, got %q", p.Mappings[0].Format)
	}
	if p.Detect[0] != "package.json:@neokapi/i18n-react" {
		t.Errorf("neokapi-i18n detect = %q", p.Detect[0])
	}
}

// TestNeokapiI18nPresetCleanNestedLayout pins the clean nested convention: the
// source glob is confined to i18n/src/, the target nests per-locale under
// i18n/{lang}/, and the stack binds a project-local brand voice profile and
// termbase source under i18n/.
func TestNeokapiI18nPresetCleanNestedLayout(t *testing.T) {
	p := neokapiI18nPreset()
	m := p.Mappings[0]
	if m.Local != "i18n/src/**/*.kbf" {
		t.Errorf("source glob = %q, want i18n/src/**/*.kbf", m.Local)
	}
	if m.TargetPath != "i18n/{lang}/{path}.kbf" {
		t.Errorf("target template = %q, want i18n/{lang}/{path}.kbf", m.TargetPath)
	}
	if p.BrandVoiceProfile != "i18n/brand-voice.yaml" {
		t.Errorf("brand voice profile = %q, want i18n/brand-voice.yaml", p.BrandVoiceProfile)
	}
	if p.TermbaseSource != "i18n/termbase.ktb" {
		t.Errorf("termbase source = %q, want i18n/termbase.ktb", p.TermbaseSource)
	}
}

// The end-to-end proof that this glob + target template resolves cleanly (a
// source i18n/src catalog lands under i18n/{lang}/ with no glob collision and no
// doubled extension) lives in core/project: TestResolveTargetPath_NestedI18nLayout.
// It cannot live here — core/project transitively imports core/preset, so a
// preset-internal test importing it would form an import cycle.

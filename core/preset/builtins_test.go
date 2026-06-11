package preset

import "testing"

func TestRegisterBuiltinsFrameworkPresets(t *testing.T) {
	reg := NewPresetRegistry()
	RegisterBuiltins(reg)

	want := []string{"kapi-react", "react-i18next", "react-intl", "nextjs", "vue-i18n", "flutter", "angular"}
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

func TestKapiReactPresetUsesKLF(t *testing.T) {
	p := kapiReactPreset()
	if p.Mappings[0].Format != "klf" {
		t.Errorf("kapi-react should extract to klf, got %q", p.Mappings[0].Format)
	}
	if p.Detect[0] != "package.json:@neokapi/kapi-react" {
		t.Errorf("kapi-react detect = %q", p.Detect[0])
	}
}

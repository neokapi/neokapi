package flow

import "testing"

func TestParseLocator(t *testing.T) {
	tests := []struct {
		in     string
		scheme string
		path   string
	}{
		{"a.json", "", "a.json"},
		{"l10n/{lang}/{name}.{ext}", "", "l10n/{lang}/{name}.{ext}"},
		{"store:", SchemeStore, ""},
		{"store", SchemeStore, ""}, // bare scheme word (flow-intent form)
		{"file", SchemeFile, ""},   // bare scheme word
		{"xliff", SchemeXLIFF, ""}, // bare scheme word
		{"klz:work.klz", SchemeKLZ, "work.klz"},
		{"xliff:hand.xliff", SchemeXLIFF, "hand.xliff"},
		{"file:store:", SchemeFile, "store:"}, // file: forces a path that reads as a scheme
		{"none", SchemeNone, ""},
		{"NONE", SchemeNone, ""},
		{"  store:  ", SchemeStore, ""},        // surrounding space trimmed before the scheme split
		{`C:\tmp\a.json`, "", `C:\tmp\a.json`}, // a drive letter is not a known scheme
		{"unknown:thing", "", "unknown:thing"},
	}
	for _, tt := range tests {
		got := ParseLocator(tt.in)
		if got.Scheme != tt.scheme || got.Path != tt.path {
			t.Errorf("ParseLocator(%q) = {scheme:%q path:%q}, want {scheme:%q path:%q}",
				tt.in, got.Scheme, got.Path, tt.scheme, tt.path)
		}
	}
}

func TestLocatorKind(t *testing.T) {
	tests := []struct {
		in   string
		want BindingKind
	}{
		// bare-path detection
		{"a.json", BindingFile},
		{"messages.yaml", BindingFile},
		{"work.klz", BindingStore},
		{"hand.xliff", BindingInterchange},
		{"vendor.xlf", BindingInterchange},
		{"catalog.po", BindingInterchange},
		{"mem.tmx", BindingInterchange},
		{"terms.tbx", BindingInterchange},
		{"l10n/", BindingFile}, // a directory of files, not the store
		// bare scheme words (flow-intent form)
		{"store", BindingStore},
		{"xliff", BindingInterchange},
		{"none", BindingNone},
		// explicit schemes override detection
		{"store:", BindingStore},
		{"klz:work.klz", BindingStore},
		{"xliff:anything", BindingInterchange},
		{"file:work.klz", BindingFile}, // force file even though .klz would detect as store
		{"none", BindingNone},
	}
	for _, tt := range tests {
		if got := ParseLocator(tt.in).Kind(); got != tt.want {
			t.Errorf("ParseLocator(%q).Kind() = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLocatorFormat(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"xliff:hand.xliff", "xliff"},
		{"po:cat.po", "po"},
		{"tmx:m.tmx", "tmx"},
		{"tbx:t.tbx", "tbx"},
		{"hand.xliff", ""}, // bare path → auto-detect from extension
		{"a.json", ""},
		{"store:", ""},
	}
	for _, tt := range tests {
		if got := ParseLocator(tt.in).Format(); got != tt.want {
			t.Errorf("ParseLocator(%q).Format() = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLocatorExplain(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"a.json", "file(a.json)"},
		{"work.klz", "store(work.klz)"},
		{"hand.xliff", "interchange(hand.xliff)"},
		{"store:", "store"},
		{"xliff:hand.xliff", "interchange(hand.xliff)"},
		{"none", "none"},
	}
	for _, tt := range tests {
		if got := ParseLocator(tt.in).Explain(); got != tt.want {
			t.Errorf("ParseLocator(%q).Explain() = %q, want %q", tt.in, got, tt.want)
		}
	}
}

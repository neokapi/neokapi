package its

import (
	"testing"
)

func mustParseSelector(t *testing.T, s string, ns map[string]string) *Selector {
	t.Helper()
	sel, err := ParseSelector(s, ns)
	if err != nil {
		t.Fatalf("ParseSelector(%q): %v", s, err)
	}
	return sel
}

func nm(local string) NameMatch { return NameMatch{Local: local} }

func TestSelector_Matches(t *testing.T) {
	tests := []struct {
		name     string
		selector string
		ns       map[string]string
		ctx      ElementContext
		match    bool
	}{
		{
			name:     "absolute path matches root child",
			selector: "/myDoc/head",
			ctx:      ElementContext{Path: []NameMatch{nm("myDoc"), nm("head")}},
			match:    true,
		},
		{
			name:     "absolute path does not match descendant",
			selector: "/myDoc/head",
			ctx:      ElementContext{Path: []NameMatch{nm("myDoc"), nm("body"), nm("head")}},
			match:    false,
		},
		{
			name:     "descendant matches anywhere",
			selector: "//del",
			ctx:      ElementContext{Path: []NameMatch{nm("doc"), nm("p"), nm("del")}},
			match:    true,
		},
		{
			name:     "descendant does not match ancestor",
			selector: "//del",
			ctx:      ElementContext{Path: []NameMatch{nm("doc"), nm("del"), nm("p")}},
			match:    false,
		},
		{
			name:     "union matches any branch",
			selector: "//ui|//ins|//del|//imgRef",
			ctx:      ElementContext{Path: []NameMatch{nm("doc"), nm("ins")}},
			match:    true,
		},
		{
			name:     "predicate equals attr",
			selector: "//msg[@id='NotFound']",
			ctx: ElementContext{
				Path:       []NameMatch{nm("doc"), nm("msg")},
				Attributes: []Attribute{{Name: nm("id"), Value: "NotFound"}},
			},
			match: true,
		},
		{
			name:     "predicate equals attr fails on wrong value",
			selector: "//msg[@id='NotFound']",
			ctx: ElementContext{
				Path:       []NameMatch{nm("doc"), nm("msg")},
				Attributes: []Attribute{{Name: nm("id"), Value: "Other"}},
			},
			match: false,
		},
		{
			name:     "predicate ancestor matches when present",
			selector: "//*",
			ctx:      ElementContext{Path: []NameMatch{nm("doc"), nm("p")}},
			match:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sel := mustParseSelector(t, tt.selector, tt.ns)
			got := sel.MatchElement(&tt.ctx)
			if got != tt.match {
				t.Errorf("MatchElement(%q) = %v; want %v\nselector parsed as %s", tt.selector, got, tt.match, sel)
			}
		})
	}
}

func TestSelector_MatchAttribute(t *testing.T) {
	tests := []struct {
		name     string
		selector string
		ctx      ElementContext
		attrName NameMatch
		match    bool
	}{
		{
			name:     "//*/@alt matches alt on any element",
			selector: "//*/@alt",
			ctx:      ElementContext{Path: []NameMatch{nm("doc"), nm("img")}},
			attrName: nm("alt"),
			match:    true,
		},
		{
			name:     "//img/@alt matches alt on img only",
			selector: "//img/@alt",
			ctx:      ElementContext{Path: []NameMatch{nm("doc"), nm("img")}},
			attrName: nm("alt"),
			match:    true,
		},
		{
			name:     "//img/@alt does not match alt on other element",
			selector: "//img/@alt",
			ctx:      ElementContext{Path: []NameMatch{nm("doc"), nm("p")}},
			attrName: nm("alt"),
			match:    false,
		},
		{
			name:     "//@*[ancestor::del] matches any attr under del",
			selector: "//@*[ancestor::del]",
			ctx: ElementContext{
				Path: []NameMatch{nm("doc"), nm("del"), nm("img")},
			},
			attrName: nm("src"),
			match:    true,
		},
		{
			name:     "//@*[ancestor::del] does not match outside del",
			selector: "//@*[ancestor::del]",
			ctx:      ElementContext{Path: []NameMatch{nm("doc"), nm("img")}},
			attrName: nm("src"),
			match:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sel := mustParseSelector(t, tt.selector, nil)
			got := sel.MatchAttribute(&tt.ctx, tt.attrName)
			if got != tt.match {
				t.Errorf("MatchAttribute = %v; want %v", got, tt.match)
			}
		})
	}
}

func TestParseSelector_Errors(t *testing.T) {
	cases := []string{
		"",
		"foo",         // must start with /
		"/foo[",       // unterminated predicate
		"/foo[@bar=]", // missing value
		"//*[unsupported()]",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			if _, err := ParseSelector(c, nil); err == nil {
				t.Errorf("ParseSelector(%q) expected error, got nil", c)
			}
		})
	}
}

func TestParseSelector_Namespaces(t *testing.T) {
	ns := map[string]string{"h": "http://www.w3.org/1999/xhtml"}
	sel := mustParseSelector(t, "//h:meta[@name='keywords']/@content", ns)
	ctx := &ElementContext{
		Path: []NameMatch{
			{Local: "html", NamespaceURI: "http://www.w3.org/1999/xhtml"},
			{Local: "head", NamespaceURI: "http://www.w3.org/1999/xhtml"},
			{Local: "meta", NamespaceURI: "http://www.w3.org/1999/xhtml"},
		},
		Attributes: []Attribute{{Name: NameMatch{Local: "name"}, Value: "keywords"}},
	}
	if !sel.MatchAttribute(ctx, NameMatch{Local: "content"}) {
		t.Fatalf("expected match, got false; sel=%s", sel)
	}
}

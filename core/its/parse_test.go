package its

import (
	"testing"
)

func TestExtractRules_TranslateAndWithinText(t *testing.T) {
	src := []byte(`<doc xmlns:its="http://www.w3.org/2005/11/its">
  <its:rules version="2.0">
    <its:translateRule selector="/myDoc/head" translate="no"/>
    <its:translateRule selector="//*/@alt" translate="yes"/>
    <its:withinTextRule selector="//ui|//ins|//del|//imgRef" withinText="yes"/>
  </its:rules>
</doc>`)
	rs, ext, err := ExtractRules(src)
	if err != nil {
		t.Fatalf("ExtractRules: %v", err)
	}
	if len(ext) != 0 {
		t.Errorf("unexpected externals: %+v", ext)
	}
	if len(rs.Rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rs.Rules))
	}
	if rs.Rules[0].Category != CatTranslate || rs.Rules[0].Translate != No {
		t.Errorf("rule[0] = %+v; want translate=no", rs.Rules[0])
	}
	if rs.Rules[1].Translate != Yes {
		t.Errorf("rule[1] translate = %v; want yes", rs.Rules[1].Translate)
	}
	if rs.Rules[2].Category != CatElementsWithinText || rs.Rules[2].WithinText != Yes {
		t.Errorf("rule[2] = %+v; want withinText=yes", rs.Rules[2])
	}
}

func TestExtractRules_MultipleRulesElements(t *testing.T) {
	src := []byte(`<doc xmlns:its="http://www.w3.org/2005/11/its">
  <its:rules version="2.0">
    <its:translateRule selector="//head" translate="no"/>
  </its:rules>
  <its:rules version="2.0">
    <its:translateRule selector="//body" translate="yes"/>
  </its:rules>
</doc>`)
	rs, _, err := ExtractRules(src)
	if err != nil {
		t.Fatalf("ExtractRules: %v", err)
	}
	if len(rs.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rs.Rules))
	}
	if rs.Rules[0].Priority >= rs.Rules[1].Priority {
		t.Errorf("priorities not strictly increasing: %d, %d", rs.Rules[0].Priority, rs.Rules[1].Priority)
	}
}

func TestExtractRules_External(t *testing.T) {
	src := []byte(`<doc xmlns:its="http://www.w3.org/2005/11/its" xmlns:xlink="http://www.w3.org/1999/xlink">
  <its:rules version="2.0" xlink:href="rules.xml"/>
</doc>`)
	_, ext, err := ExtractRules(src)
	if err != nil {
		t.Fatalf("ExtractRules: %v", err)
	}
	if len(ext) != 1 || ext[0].Href != "rules.xml" {
		t.Errorf("unexpected externals: %+v", ext)
	}
}

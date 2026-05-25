package srx

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_FullDocument(t *testing.T) {
	const data = `<?xml version="1.0" encoding="UTF-8"?>
<srx xmlns="http://www.lisa.org/srx20" version="2.0">
  <header segmentsubflows="yes" cascade="no">
    <formathandle type="start" include="no"/>
    <formathandle type="end" include="yes"/>
  </header>
  <body>
    <languagerules>
      <languagerule languagerulename="Default">
        <rule break="no">
          <beforebreak>Mr\.</beforebreak>
          <afterbreak>\s</afterbreak>
        </rule>
        <rule break="yes">
          <beforebreak>\.</beforebreak>
          <afterbreak>\s</afterbreak>
        </rule>
      </languagerule>
    </languagerules>
    <maprules>
      <languagemap languagepattern="en.*" languagerulename="Default"/>
    </maprules>
  </body>
</srx>`

	doc, err := Parse([]byte(data))
	require.NoError(t, err)

	assert.Equal(t, "2.0", doc.Version)
	assert.True(t, doc.SegmentSubflows)
	assert.False(t, doc.Cascade)

	require.Len(t, doc.FormatHandles, 2)
	assert.Equal(t, "start", doc.FormatHandles[0].Type)
	assert.False(t, doc.FormatHandles[0].Include)
	assert.Equal(t, "end", doc.FormatHandles[1].Type)
	assert.True(t, doc.FormatHandles[1].Include)

	require.Len(t, doc.LanguageRules, 1)
	lr := doc.LanguageRules[0]
	assert.Equal(t, "Default", lr.Name)
	require.Len(t, lr.Rules, 2)
	assert.False(t, lr.Rules[0].Break)
	assert.Equal(t, `Mr\.`, lr.Rules[0].BeforeBreak)
	assert.Equal(t, `\s`, lr.Rules[0].AfterBreak)
	assert.True(t, lr.Rules[1].Break)

	require.Len(t, doc.LanguageMaps, 1)
	assert.Equal(t, "en.*", doc.LanguageMaps[0].LanguagePattern)
	assert.Equal(t, "Default", doc.LanguageMaps[0].LanguageRule)
}

func TestParse_Defaults(t *testing.T) {
	// Missing header/attributes: cascade and segmentsubflows default to true,
	// an unspecified break defaults to a break, and missing before/after-break
	// elements are empty strings.
	const data = `<srx version="2.0">
  <body>
    <languagerules>
      <languagerule languagerulename="R">
        <rule>
          <afterbreak>\s</afterbreak>
        </rule>
      </languagerule>
    </languagerules>
    <maprules>
      <languagemap languagepattern=".*" languagerulename="R"/>
    </maprules>
  </body>
</srx>`

	doc, err := Parse([]byte(data))
	require.NoError(t, err)
	assert.True(t, doc.Cascade)
	assert.True(t, doc.SegmentSubflows)
	require.Len(t, doc.LanguageRules, 1)
	require.Len(t, doc.LanguageRules[0].Rules, 1)
	r := doc.LanguageRules[0].Rules[0]
	assert.True(t, r.Break, "unspecified break should default to yes")
	assert.Empty(t, r.BeforeBreak)
	assert.Equal(t, `\s`, r.AfterBreak)
}

func TestParse_CascadeNo(t *testing.T) {
	doc, err := Parse([]byte(`<srx version="2.0"><header cascade="no"/><body><languagerules/><maprules/></body></srx>`))
	require.NoError(t, err)
	assert.False(t, doc.Cascade)
}

func TestParse_Invalid(t *testing.T) {
	_, err := Parse([]byte("<srx>this is not closed"))
	assert.Error(t, err)
}

func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.srx")
	const data = `<srx version="2.0"><header cascade="yes"/><body>` +
		`<languagerules><languagerule languagerulename="R">` +
		`<rule break="yes"><beforebreak>\.</beforebreak><afterbreak>\s</afterbreak></rule>` +
		`</languagerule></languagerules>` +
		`<maprules><languagemap languagepattern=".*" languagerulename="R"/></maprules></body></srx>`
	require.NoError(t, os.WriteFile(path, []byte(data), 0o644))

	doc, err := ParseFile(path)
	require.NoError(t, err)
	assert.Equal(t, "2.0", doc.Version)
	require.Len(t, doc.LanguageRules, 1)

	_, err = ParseFile(filepath.Join(dir, "missing.srx"))
	assert.Error(t, err)
}

func TestParse_EmbeddedDefaultIsValid(t *testing.T) {
	doc, err := Parse(defaultSRX)
	require.NoError(t, err)
	assert.Equal(t, "2.0", doc.Version)
	assert.True(t, doc.Cascade)
	assert.NotEmpty(t, doc.LanguageRules)
	assert.NotEmpty(t, doc.LanguageMaps)

	// Every map must reference a defined language rule, and every rule must
	// compile.
	for _, lm := range doc.LanguageMaps {
		assert.NotNil(t, doc.languageRule(lm.LanguageRule), "map %q references unknown rule %q", lm.LanguagePattern, lm.LanguageRule)
	}
	for _, lr := range doc.LanguageRules {
		for _, r := range lr.Rules {
			_, err := compileRule(r)
			assert.NoError(t, err, "rule before=%q after=%q in %q should compile", r.BeforeBreak, r.AfterBreak, lr.Name)
		}
	}
}

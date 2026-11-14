package formats_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const androidStrings = `<?xml version="1.0" encoding="utf-8"?>
<resources>
    <string name="order_summary">Your order of %1$d items ships to %2$s on Friday.</string>
    <string name="welcome_back">Welcome back, <b>%1$s</b>. Pick up where you left off.</string>
    <string name="pay_now">Pay %1$s with your saved card</string>
</resources>
`

// TestXMLDetectionByContent resolves .xml files through the shared resolver:
// an Android string resources file goes to androidxml by its content, and
// every other kind of XML stays where it was.
func TestXMLDetectionByContent(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	licence := "<!--\n" + strings.Repeat("  Licensed under the Apache License, Version 2.0.\n", 20) + "-->\n"
	tests := []struct {
		name    string
		path    string
		content string
		want    registry.FormatID
		byMatch bool
	}{
		{"android strings", "strings.xml", androidStrings, "androidxml", true},
		{"android under res/values", "app/src/main/res/values-nb/strings.xml", androidStrings, "androidxml", true},
		{"android plurals only", "plurals.xml",
			`<resources><plurals name="n"><item quantity="one">%d item</item><item quantity="other">%d items</item></plurals></resources>`,
			"androidxml", true},
		{"android string-array only", "arrays.xml",
			`<?xml version="1.0"?><resources><string-array name="days"><item>Mon</item></string-array></resources>`,
			"androidxml", true},
		{"android after a long licence comment", "strings.xml",
			`<?xml version="1.0" encoding="utf-8"?>` + "\n" + licence + `<resources><string name="a">A</string></resources>`,
			"androidxml", true},
		{"android with colors before strings", "values.xml",
			`<resources><color name="c">#fff</color><dimen name="d">4dp</dimen><string name="s">S</string></resources>`,
			"androidxml", true},
		{"plain xml", "catalog.xml",
			`<?xml version="1.0"?><catalog><book><title>XML Guide</title></book></catalog>`,
			"xml", false},
		{"xliff in .xml", "messages.xml",
			`<?xml version="1.0"?><xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2"><file source-language="en"><body><trans-unit id="1"><source>Hi</source></trans-unit></body></file></xliff>`,
			"xml", false},
		{"resources without string children", "colors.xml",
			`<?xml version="1.0"?><resources><color name="primary">#6200EE</color><dimen name="pad">8dp</dimen></resources>`,
			"xml", false},
		{"maven pom", "pom.xml",
			`<?xml version="1.0"?><project xmlns="http://maven.apache.org/POM/4.0.0"><build><resources><resource><directory>src/main/resources</directory></resource></resources></build><properties><string>x</string></properties></project>`,
			"xml", false},
		{"string outside a resources root", "data.xml",
			`<?xml version="1.0"?><config><resources/><string name="a">A</string></config>`,
			"xml", false},
		{"doclang", "report.dclg.xml",
			`<?xml version="1.0"?><doclang xmlns="https://www.doclang.ai/ns/v0" version="0.6"><heading level="1">Hi</heading></doclang>`,
			"doclang", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := reg.Resolve(tt.path, registry.DetectOptions{
				Content: func() (io.ReadSeeker, error) { return strings.NewReader(tt.content), nil },
			})
			require.NoError(t, err)
			assert.Equal(t, tt.want, res.Format)
			assert.Equal(t, registry.RuleNativeFirst, res.Rule)
			assert.Contains(t, res.Candidates, registry.FormatID("androidxml"))
			if tt.byMatch {
				assert.True(t, res.Content, "the file's content chose the format")
			}

			// The detector on its own agrees with the registry.
			name, err := reg.Detector().Detect(tt.path, strings.NewReader(tt.content), "")
			require.NoError(t, err)
			assert.Equal(t, string(tt.want), name)
		})
	}

	// Without content to read, the extension alone keeps .xml on the
	// generic reader.
	det, err := reg.Detect("strings.xml", registry.DetectOptions{ExtensionOnly: true})
	require.NoError(t, err)
	assert.Equal(t, registry.FormatID("xml"), det)
}

// TestAndroidStringsReadEndToEnd detects an Android file on disk and reads it
// through the registered reader: blocks carry the resource names, printf
// arguments are placeholder runs, and Android markup is paired codes.
func TestAndroidStringsReadEndToEnd(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	doc := androidStrings[:strings.LastIndex(androidStrings, "</resources>")] +
		`    <string name="discount">Save 20%% on %d items</string>
    <string name="download">Downloading <xliff:g id="file" example="a.pdf">%1$s</xliff:g> now</string>
</resources>
`
	dir := t.TempDir()
	path := filepath.Join(dir, "strings.xml")
	require.NoError(t, os.WriteFile(path, []byte(doc), 0o644))

	id, err := reg.Detect(path, registry.DetectOptions{})
	require.NoError(t, err)
	require.Equal(t, registry.FormatID("androidxml"), id)

	reader, err := reg.NewReader(id)
	require.NoError(t, err)
	raw := &model.RawDocument{URI: path, SourceLocale: model.LocaleEnglish,
		Reader: io.NopCloser(bytes.NewReader([]byte(doc)))}
	require.NoError(t, reader.Open(t.Context(), raw))
	blocks := map[string]*model.Block{}
	var names []string
	for res := range reader.Read(t.Context()) {
		require.NoError(t, res.Error)
		if res.Part == nil {
			continue
		}
		if b, ok := res.Part.Resource.(*model.Block); ok && b.Translatable {
			blocks[b.Name] = b
			names = append(names, b.Name)
		}
	}
	require.NoError(t, reader.Close())
	assert.Equal(t, []string{"order_summary", "welcome_back", "pay_now", "discount", "download"}, names)

	// placeholders lists the source bytes of each placeholder run; paired
	// lists each paired code's opening bytes.
	codes := func(b *model.Block) (placeholders, paired []string) {
		for _, r := range b.SourceRuns() {
			switch {
			case r.Ph != nil:
				placeholders = append(placeholders, r.Ph.Data)
			case r.PcOpen != nil:
				paired = append(paired, r.PcOpen.Data)
			}
		}
		return placeholders, paired
	}

	ph, pc := codes(blocks["order_summary"])
	assert.Equal(t, []string{"%1$d", "%2$s"}, ph)
	assert.Empty(t, pc)

	ph, pc = codes(blocks["welcome_back"])
	assert.Equal(t, []string{"%1$s"}, ph)
	assert.Equal(t, []string{"<b>"}, pc)

	ph, _ = codes(blocks["pay_now"])
	assert.Equal(t, []string{"%1$s"}, ph)

	ph, _ = codes(blocks["discount"])
	assert.Equal(t, []string{"%%", "%d"}, ph)

	ph, pc = codes(blocks["download"])
	assert.Equal(t, []string{"%1$s"}, ph)
	require.Len(t, pc, 1)
	assert.True(t, strings.HasPrefix(pc[0], "<xliff:g"), "xliff:g is a paired code: %q", pc[0])
}

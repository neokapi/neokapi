package xml_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	xmlfmt "github.com/neokapi/neokapi/core/formats/xml"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func streamingPairRoundtripXML(t *testing.T, input string, cfg *xmlfmt.Config) string {
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "doc.xml")
	require.NoError(t, os.WriteFile(path, []byte(input), 0o644))
	f, err := os.Open(path)
	require.NoError(t, err)

	reader := xmlfmt.NewReader()
	writer := xmlfmt.NewWriter()
	if cfg != nil {
		require.NoError(t, reader.SetConfig(cfg))
	}
	store := format.NewStreamingSkeletonStore()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)
	require.True(t, store.IsStreaming())

	require.NoError(t, reader.Open(ctx, &model.RawDocument{URI: path, SourceLocale: model.LocaleEnglish, Reader: f}))
	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	partsCh := make(chan *model.Part, 64)
	go func() {
		defer close(partsCh)
		defer store.CloseWrite()
		defer reader.Close()
		for res := range reader.Read(ctx) {
			assert.NoError(t, res.Error)
			if res.Part != nil {
				partsCh <- res.Part
			}
		}
	}()
	require.NoError(t, writer.Write(ctx, partsCh))
	writer.Close()
	return buf.String()
}

func TestStreamingPair_ByteExactXML(t *testing.T) {
	attrCfg := &xmlfmt.Config{}
	attrCfg.ElementRules = append(attrCfg.ElementRules, &xmlfmt.ElementRule{
		Name:                   "item",
		RuleTypes:              []xmlfmt.RuleType{xmlfmt.RuleAttributeTrans},
		TranslatableAttributes: map[string]*xmlfmt.TranslatableAttrCondition{"label": {}},
	})

	cases := []struct {
		name string
		doc  string
		cfg  *xmlfmt.Config
	}{
		{"simple", `<?xml version="1.0"?><root><message>Hello World</message></root>`, nil},
		{"multi", "<?xml version=\"1.0\"?>\n<resources>\n  <string>One</string>\n  <string>Two</string>\n  <string>Three</string>\n</resources>", nil},
		{"nested", "<?xml version=\"1.0\"?>\n<root>\n  <parent>\n    <child>Nested</child>\n  </parent>\n  <other>Sibling</other>\n</root>", nil},
		{"comments", "<?xml version=\"1.0\"?>\n<!-- c -->\n<root>\n  <!-- e -->\n  <message>Hi</message>\n</root>", nil},
		{"its", "<?xml version=\"1.0\"?>\n<doc xmlns:its=\"http://www.w3.org/2005/11/its\">\n  <its:rules version=\"2.0\"><its:translateRule selector=\"//code\" translate=\"no\"/></its:rules>\n  <p>Translate</p>\n  <code>keep</code>\n</doc>", nil},
		{"translatable_attr", "<?xml version=\"1.0\"?>\n<root>\n  <item label=\"First label\">body one</item>\n  <item label=\"Second label\">body two</item>\n</root>", attrCfg},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.doc, streamingPairRoundtripXML(t, c.doc, c.cfg), "streaming pair must round-trip byte-exact")
		})
	}
}

func TestStreamingPair_BoundedWriterXML(t *testing.T) {
	if testing.Short() {
		t.Skip("memory test skipped in -short")
	}
	defer debug.SetGCPercent(debug.SetGCPercent(20))
	liveHeap := func() uint64 {
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return m.HeapAlloc
	}
	peakWriting := func(n int) (peak uint64, size int) {
		path, sz := genXMLFile(t, n)
		f, err := os.Open(path)
		require.NoError(t, err)
		reader := xmlfmt.NewReader()
		writer := xmlfmt.NewWriter()
		store := format.NewStreamingSkeletonStore()
		reader.SetSkeletonStore(store)
		writer.SetSkeletonStore(store)
		require.NoError(t, reader.Open(context.Background(), &model.RawDocument{URI: path, SourceLocale: model.LocaleEnglish, Reader: f}))
		require.NoError(t, writer.SetOutputWriter(io.Discard))
		partsCh := make(chan *model.Part, 64)
		go func() {
			defer close(partsCh)
			defer store.CloseWrite()
			defer reader.Close()
			for res := range reader.Read(context.Background()) {
				if res.Error != nil || res.Part == nil {
					continue
				}
				partsCh <- res.Part
			}
		}()
		stride := max(n/256, 1)
		base := liveHeap()
		wrapped := make(chan *model.Part)
		go func() {
			defer close(wrapped)
			count := 0
			for p := range partsCh {
				wrapped <- p
				count++
				if count%stride == 0 {
					if h := liveHeap(); h > base && h-base > peak {
						peak = h - base
					}
				}
			}
		}()
		require.NoError(t, writer.Write(context.Background(), wrapped))
		writer.Close()
		return peak, sz
	}
	ps, smallSize := peakWriting(2_000)
	pl, largeSize := peakWriting(40_000)
	t.Logf("xml streaming writer peakΔ: small(%d KiB)=%d KiB, large(%d KiB)=%d KiB", smallSize/1024, ps/1024, largeSize/1024, pl/1024)
	assert.Less(t, pl, uint64(largeSize)/4, "streaming writer peak not bounded well below doc size")
	if ps > 0 {
		assert.Less(t, pl, max(ps, uint64(1<<20))*3, "streaming writer peak scaled with input")
	}
}

package openxml

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
)

// BenchmarkBigDocxRoundtrip profiles the full read → translate → write path on
// a large (~1 MB) .docx — the pseudobench outlier where native goes
// superlinear (~7.3s vs ~1.7s for the bridge daemon). Mirrors the native
// pseudo path: skeleton store wired, FR targets set on every translatable
// block, skeleton-replay write.
//
// Run:
//
//	go test ./core/formats/openxml/ -run '^$' \
//	  -bench BenchmarkBigDocxRoundtrip -benchtime 3x -cpuprofile /tmp/bigdocx.prof
func BenchmarkBigDocxRoundtrip(b *testing.B) {
	path := os.Getenv("BIG_DOCX")
	if path == "" {
		path = "../../../.parity/okapi-testdata/1.48.0/integration-tests/okapi/src/test/resources/openxml/docx/big.docx"
	}
	original, err := os.ReadFile(path)
	if err != nil {
		b.Skipf("big.docx not available: %v", err)
	}
	qps := model.LocaleID("qps")
	ctx := context.Background()
	b.ResetTimer()
	for range b.N {
		skel, err := format.NewSkeletonStore()
		if err != nil {
			b.Fatal(err)
		}
		reader := NewReader()
		reader.SetSkeletonStore(skel)
		doc := &model.RawDocument{
			URI:          "big.docx",
			SourceLocale: model.LocaleEnglish,
			Encoding:     "UTF-8",
			Reader:       readCloserFromBytes(original),
		}
		if err := reader.Open(ctx, doc); err != nil {
			b.Fatal(err)
		}
		var parts []*model.Part
		for pr := range reader.Read(ctx) {
			if pr.Error != nil {
				b.Fatal(pr.Error)
			}
			parts = append(parts, pr.Part)
		}
		reader.Close()
		// Apply the REAL pseudo tool (qps) so target RUNS are produced —
		// the writer's translated-run emission path, which the pseudobench
		// "kapi pseudo-translate --target-lang qps" exercises (and a
		// flat-text target would bypass).
		pt, err := tools.NewPseudoTranslateFromConfig(nil, "qps")
		if err != nil {
			b.Fatal(err)
		}
		in := make(chan *model.Part, len(parts)+1)
		out := make(chan *model.Part, len(parts)+1)
		for _, p := range parts {
			in <- p
		}
		close(in)
		if err := pt.Process(ctx, in, out); err != nil {
			b.Fatal(err)
		}
		close(out)
		translated := make([]*model.Part, 0, len(parts))
		for p := range out {
			translated = append(translated, p)
		}
		parts = translated
		var buf bytes.Buffer
		w := NewWriter()
		w.SetOriginalContent(original)
		w.SetSkeletonStore(skel)
		w.SetLocale(qps)
		if err := w.SetOutputWriter(&buf); err != nil {
			b.Fatal(err)
		}
		if err := w.Write(ctx, testutil.PartsToChannel(parts)); err != nil {
			b.Fatal(err)
		}
		w.Close()
		_ = skel.Close()
	}
}

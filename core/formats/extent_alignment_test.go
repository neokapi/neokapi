package formats_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// alignmentFloor is the number of inputs, from each format's spec examples and
// testdata, whose skeleton aligned and located every translatable block when
// this was measured. A floor rather than an exact count: adding fixtures must
// not break the test, but a reader change that stops a format aligning, or a
// fixture sweep that stops the test reading one, must.
//
// Formats absent from the map align nothing, for a reason recorded beside the
// measurement in TestSkeletonExtents_InCoreFormats' log: a container whose
// skeleton describes an inner part (epub, openxml), a bilingual file whose refs
// name segments rather than blocks (po, tmx, ts, xliff, xliff2), or no skeleton
// at all (doclang, docling, kbf).
var alignmentFloor = map[string]int{
	"androidxml":    6,
	"applestrings":  8,
	"arb":           9,
	"asciidoc":      6,
	"csv":           19,
	"designtokens":  4,
	"html":          34,
	"i18next":       3,
	"json":          52,
	"markdown":      59,
	"mdx":           33,
	"messageformat": 22,
	"plaintext":     15,
	"properties":    24,
	"resx":          11,
	"srt":           4,
	"tsv":           1,
	"vtt":           14,
	"xcstrings":     11,
	"xml":           32,
	"yaml":          20,
}

// locatedFloor is, for a format with inputs whose skeleton confines some block
// to a region, the number of inputs that placed every translatable block
// exactly or confined it when this was measured. It is a floor for the reason
// alignmentFloor is.
var locatedFloor = map[string]int{
	"markdown": 60,
}

type alignmentTally struct {
	inputs, unreadable, noSkeleton, aligned, confined, unlocated int
	unavailable                                                  map[string]int
}

// TestSkeletonExtents_InCoreFormats reads every in-core format's spec examples
// and testdata fixtures with a skeleton store wired, and aligns each skeleton
// against the bytes it was read from. For every input that aligns it asserts,
// rather than assumes, that the skeleton with each ref replaced by its extent's
// bytes rebuilds the source byte for byte, and that corrupting one skeleton
// byte in the source changes or refuses the alignment. An input whose skeleton
// confines some blocks is counted apart, and held to the same corruption test.
func TestSkeletonExtents_InCoreFormats(t *testing.T) {
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)
	tallies := map[string]*alignmentTally{}

	check := func(label, id string, cfg map[string]any, input []byte) {
		tl := tallies[id]
		if tl == nil {
			tl = &alignmentTally{unavailable: map[string]int{}}
			tallies[id] = tl
		}
		tl.inputs++
		blocks, entries, ok := readWithSkeleton(reg, id, cfg, input)
		switch {
		case !ok:
			tl.unreadable++
			return
		case entries == nil:
			tl.noSkeleton++
			return
		}

		al, err := format.LocateSkeleton(input, entries)
		if err != nil {
			if !errors.Is(err, format.ErrExtentsUnavailable) {
				t.Errorf("%s: an alignment failure must wrap ErrExtentsUnavailable: %v", label, err)
			}
			tl.unavailable[unavailableKind(err)]++
			return
		}
		located := map[string]bool{}
		for _, x := range al.Extents {
			located[x.Block] = true
		}
		for _, c := range al.Confined {
			located[c.Region.Block] = true
		}
		everyBlockLocated := true
		for _, b := range blocks {
			if b.Translatable && !located[b.ID] {
				everyBlockLocated = false
			}
		}

		if len(al.Confined) > 0 {
			assertConfinedAlignmentIsSound(t, label, input, entries, al)
			if everyBlockLocated {
				tl.confined++
			} else {
				tl.unlocated++
			}
			return
		}
		if got := rebuildFromExtents(input, entries, al.Extents); !bytes.Equal(got, input) {
			t.Errorf("%s: the skeleton and its extents rebuild %d bytes that differ from the %d-byte source", label, len(got), len(input))
			return
		}
		assertCorruptionIsNoticed(t, label, input, entries, al.Extents)
		if !everyBlockLocated {
			tl.unlocated++
			return
		}
		tl.aligned++
	}

	specs, err := filepath.Glob("*/spec.yaml")
	if err != nil || len(specs) == 0 {
		t.Fatalf("no spec.yaml found beside the formats (%v)", err)
	}
	for _, path := range specs {
		dir := filepath.Dir(path)
		if _, err := reg.NewReader(registry.FormatID(dir)); err != nil {
			continue
		}
		s, err := spec.Load(path)
		if err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		for _, feat := range s.Features {
			for _, ex := range feat.Examples {
				if ex.BridgeOnly || ex.CaseClass() == "invalid" {
					continue
				}
				input, err := spec.ResolveInput(s, ex)
				if err != nil {
					// An okapi: or corpus: input that is not fetched here.
					continue
				}
				check(dir+"/spec.yaml "+ex.CaseID(), dir, spec.MergeConfig(feat.Config, ex.Config), input)
			}
		}
	}
	err = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.Contains("/"+filepath.ToSlash(path), "/testdata/") {
			return err
		}
		id, derr := reg.Detect(path, registry.DetectOptions{ExtensionOnly: true})
		if derr != nil || id == "" {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		check(path, string(id), nil, data)
		return nil
	})
	if err != nil {
		t.Fatalf("walk testdata: %v", err)
	}

	t.Log(alignmentTable(tallies))
	for id, floor := range alignmentFloor {
		got := 0
		if tl := tallies[id]; tl != nil {
			got = tl.aligned
		}
		if got < floor {
			t.Errorf("%s: %d inputs aligned with every translatable block located, below the floor of %d", id, got, floor)
		}
	}
	for id, floor := range locatedFloor {
		got := 0
		if tl := tallies[id]; tl != nil {
			got = tl.aligned + tl.confined
		}
		if got < floor {
			t.Errorf("%s: %d inputs placed or confined every translatable block, below the floor of %d", id, got, floor)
		}
	}
}

// assertCorruptionIsNoticed changes the source byte just after the first extent
// that ends before the end of the source. That byte begins skeleton text, so the
// alignment must either refuse the corrupted source or place the blocks
// differently: an alignment that returned the same extents would not be reading
// the bytes it claims to align.
func assertCorruptionIsNoticed(t *testing.T, label string, input []byte, entries []format.SkeletonEntry, extents []format.Extent) {
	t.Helper()
	at := -1
	for _, x := range extents {
		if x.End < len(input) {
			at = x.End
			break
		}
	}
	if at < 0 {
		return
	}
	corrupt := bytes.Clone(input)
	corrupt[at] ^= 0x5a
	again, err := format.AlignSkeleton(corrupt, entries)
	if err == nil && reflect.DeepEqual(again, extents) {
		t.Errorf("%s: corrupting byte %d left the alignment unchanged", label, at)
	}
}

// assertConfinedAlignmentIsSound checks an alignment that confines some blocks:
// every extent and every region lies inside the source, and corrupting the
// byte after the first extent that ends before the end of the source changes or
// refuses the alignment, as assertCorruptionIsNoticed does for an alignment
// that places every block.
func assertConfinedAlignmentIsSound(t *testing.T, label string, input []byte, entries []format.SkeletonEntry, al format.Alignment) {
	t.Helper()
	inside := func(x format.Extent) bool { return 0 <= x.Start && x.Start <= x.End && x.End <= len(input) }
	at := -1
	for _, x := range al.Extents {
		if !inside(x) {
			t.Errorf("%s: block %s's extent [%d,%d) lies outside the %d-byte source", label, x.Block, x.Start, x.End, len(input))
		}
		if at < 0 && x.End < len(input) {
			at = x.End
		}
	}
	for _, c := range al.Confined {
		if !inside(c.Region) {
			t.Errorf("%s: block %s's region [%d,%d) lies outside the %d-byte source", label, c.Region.Block, c.Region.Start, c.Region.End, len(input))
		}
	}
	if at < 0 {
		return
	}
	corrupt := bytes.Clone(input)
	corrupt[at] ^= 0x5a
	again, err := format.LocateSkeleton(corrupt, entries)
	if err == nil && reflect.DeepEqual(again, al) {
		t.Errorf("%s: corrupting byte %d left the alignment unchanged", label, at)
	}
}

func readWithSkeleton(reg *registry.FormatRegistry, id string, cfg map[string]any, input []byte) ([]*model.Block, []format.SkeletonEntry, bool) {
	reader, err := reg.NewReader(registry.FormatID(id))
	if err != nil {
		return nil, nil, false
	}
	defer reader.Close()
	if len(cfg) > 0 {
		if reader.Config() == nil || reader.Config().ApplyMap(cfg) != nil {
			return nil, nil, false
		}
	}
	emitter, ok := reader.(format.SkeletonStoreEmitter)
	if !ok {
		return nil, nil, true
	}
	store := format.NewMemorySkeletonStore()
	emitter.SetSkeletonStore(store)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	doc := &model.RawDocument{SourceLocale: "en", Encoding: "UTF-8", Reader: io.NopCloser(bytes.NewReader(input))}
	if err := reader.Open(ctx, doc); err != nil {
		return nil, nil, false
	}
	var blocks []*model.Block
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			return nil, nil, false
		}
		if b, ok := pr.Part.Resource.(*model.Block); ok {
			blocks = append(blocks, b)
		}
	}
	if err := store.Flush(); err != nil {
		return nil, nil, false
	}
	var entries []format.SkeletonEntry
	for {
		e, err := store.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, false
		}
		entries = append(entries, e)
	}
	return blocks, entries, true
}

// rebuildFromExtents replays a skeleton the way a writer replays an unedited
// document, taking each ref's bytes from its extent.
func rebuildFromExtents(src []byte, entries []format.SkeletonEntry, extents []format.Extent) []byte {
	var out bytes.Buffer
	next := 0
	for _, e := range entries {
		switch e.Type {
		case format.SkeletonText, format.SkeletonLang:
			out.Write(e.Data)
		case format.SkeletonTrimmed:
			_, trimmed, _ := format.DecodeSkeletonPair(e.Data)
			out.Write(trimmed)
		case format.SkeletonRef:
			if next >= len(extents) {
				return nil
			}
			x := extents[next]
			out.Write(src[x.Start:x.End])
			next++
		}
	}
	if next != len(extents) {
		return nil
	}
	return out.Bytes()
}

func unavailableKind(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "ambiguous"):
		return "ambiguous"
	case strings.Contains(msg, "adjacent"):
		return "adjacent refs"
	case strings.Contains(msg, "does not occur"), strings.Contains(msg, "ends at byte"), strings.Contains(msg, "starts at byte"):
		return "skeleton not in source"
	default:
		return "other"
	}
}

func alignmentTable(tallies map[string]*alignmentTally) string {
	ids := make([]string, 0, len(tallies))
	for id := range tallies {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n%-14s %6s %8s %8s %8s %8s %9s  %s\n", "format", "inputs", "aligned", "confined", "unlocatd", "no skel", "unreadabl", "extents unavailable")
	for _, id := range ids {
		tl := tallies[id]
		kinds := make([]string, 0, len(tl.unavailable))
		for k, n := range tl.unavailable {
			kinds = append(kinds, fmt.Sprintf("%s %d", k, n))
		}
		sort.Strings(kinds)
		fmt.Fprintf(&sb, "%-14s %6d %8d %8d %8d %8d %9d  %s\n", id, tl.inputs, tl.aligned, tl.confined, tl.unlocated, tl.noSkeleton, tl.unreadable, strings.Join(kinds, ", "))
	}
	return sb.String()
}

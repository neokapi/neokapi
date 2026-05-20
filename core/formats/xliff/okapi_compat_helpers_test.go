package xliff

import (
	"bytes"
	"regexp"
	"sync"
	"testing"
)

// TestRegexCacheConcurrent exercises the tagStartRE / innerAttrRE caches
// from many goroutines at once. Before the cache was guarded by a mutex,
// the first-time map writes raced; running this test with -race flags it.
// It also asserts the cached regexes still strip attributes correctly so
// the locking didn't change behavior.
//
// The distinct (tag, attr) pairs force fresh compilations to overlap, and
// the loop re-runs the same pairs so cache hits and misses interleave.
func TestRegexCacheConcurrent(t *testing.T) {
	// Reset the caches so this test always drives first-time compilation.
	reCacheMu.Lock()
	tagStartREs = map[string]*regexp.Regexp{}
	innerAttrREs = map[string]*regexp.Regexp{}
	reCacheMu.Unlock()

	const goroutines = 16
	const iterations = 200

	tags := []string{"trans-unit", "phase", "group", "file", "source", "target"}
	attrs := []string{"approved", "date", "id", "xml:lang", "state", "translate"}

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				tag := tags[i%len(tags)]
				attr := attrs[i%len(attrs)]
				// stripAttrInTag fetches both a tagStartRE and an innerAttrRE.
				input := []byte(`<` + tag + ` ` + attr + `="x" keep="y">body</` + tag + `>`)
				out := stripAttrInTag(input, tag, attr)
				if bytes.Contains(out, []byte(attr+`="x"`)) {
					// Use t.Error from a goroutine is allowed; fail loudly.
					t.Errorf("stripAttrInTag(%q) did not strip %q: %q", input, attr, out)
					return
				}
				if !bytes.Contains(out, []byte(`keep="y"`)) {
					t.Errorf("stripAttrInTag(%q) dropped unrelated attr: %q", input, out)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// TestApplySkeletonAttrStrippingConcurrent runs the public entry point
// that lazily compiles the trans-unit/phase regexes from multiple
// goroutines, mirroring several writers stripping attributes in parallel.
func TestApplySkeletonAttrStrippingConcurrent(t *testing.T) {
	reCacheMu.Lock()
	tagStartREs = map[string]*regexp.Regexp{}
	innerAttrREs = map[string]*regexp.Regexp{}
	reCacheMu.Unlock()

	compat := OkapiCompatConfig{
		StripTransUnitApprovedAttr: true,
		StripPhaseDateAttr:         true,
	}
	src := []byte(`<trans-unit id="1" approved="no"><source>x</source></trans-unit>` +
		`<phase phase-name="p" date="2020-01-01"/>`)

	var wg sync.WaitGroup
	wg.Add(8)
	for g := 0; g < 8; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				out := applySkeletonAttrStripping(src, compat)
				if bytes.Contains(out, []byte(`approved="no"`)) || bytes.Contains(out, []byte(`date="2020-01-01"`)) {
					t.Errorf("attrs not stripped: %q", out)
					return
				}
			}
		}()
	}
	wg.Wait()
}

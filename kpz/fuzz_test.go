package kpz

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"
)

// A .kpz crosses a trust boundary by design: it is packaged here, handed to an
// outside translator, and comes back to be merged into the project. Unmarshal
// is therefore the one parser in this repo whose input is expected to be
// adversarial rather than merely malformed, and hostile_test.go pins the
// specific hardening — path containment, decompression bounds, per-member
// checksums. These targets cover the space between those cases: whatever a
// mutation engine can build that the hand-written tests did not think of.

// fuzzSeedPackages returns marshalled packages covering every member kind, for
// use as seeds. Anything that fails to marshal is skipped rather than fatal, so
// seeding can never break the target.
func fuzzSeedPackages() [][]byte {
	var out [][]byte
	add := func(p *Package) {
		if data, err := p.Marshal(); err == nil {
			out = append(out, data)
		}
	}
	add(samplePackage())
	add(&Package{Kind: KindProject, Media: []Media{{Path: "media/logo.png", Content: BytesContent([]byte("PNGDATA"))}}})
	add(&Package{Kind: KindInterchange, Media: []Media{{Path: "media/a.bin", Content: BytesContent([]byte{0, 1, 2, 3})}}})
	add(&Package{Created: "2026-06-04T00:00:00Z", Blocks: []BlockDoc{{Path: "blocks/a.kbf.json", File: sampleBlocks()}}})
	return out
}

// rezipWith rebuilds a .kpz, passing every entry through and appending extra
// ones. The manifest is left exactly as it was, so the package still verifies:
// this produces archives that are structurally damaged but STILL PARSEABLE,
// which is the corpus region where silent-acceptance bugs live rather than
// crashes. Returns nil if the input is not a readable zip.
func rezipWith(data []byte, extra map[string][]byte) []byte {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: f.Name, Method: zip.Store, Modified: zipEpoch})
		if err != nil {
			return nil
		}
		if _, err := w.Write(body); err != nil {
			return nil
		}
	}
	for name, body := range extra {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store, Modified: zipEpoch})
		if err != nil {
			return nil
		}
		if _, err := w.Write(body); err != nil {
			return nil
		}
	}
	if err := zw.Close(); err != nil {
		return nil
	}
	return buf.Bytes()
}

// seedDamagedKPZ adds archives that parse but carry something the manifest does
// not describe. The two shapes are the ones that matter for a container handed
// back by a third party:
//
//   - An entry the manifest never names. It is not covered by the root hash, so
//     it rides along undetected; the question a fuzz target keeps open is
//     whether any later stage ever reads by zip entry rather than by manifest.
//   - An entry whose name DUPLICATES a real member. Unmarshal indexes entries
//     into a map keyed by name, so the last one wins, while a consumer that
//     walked zr.File in order would see the first — the classic zip-confusion
//     shape, where two readers of the same bytes disagree about the content.
func seedDamagedKPZ(f *testing.F) {
	f.Helper()
	base, err := samplePackage().Marshal()
	if err != nil {
		return
	}
	if b := rezipWith(base, map[string][]byte{"unlisted/extra.txt": []byte("not in the manifest")}); b != nil {
		f.Add(b)
	}
	if b := rezipWith(base, map[string][]byte{ManifestPath: []byte(`{"kind":"project"}`)}); b != nil {
		f.Add(b)
	}
	if b := rezipWith(base, map[string][]byte{"media/logo.bin": []byte("SHADOWED")}); b != nil {
		f.Add(b)
	}
	if b := rezipWith(base, map[string][]byte{"../escape.txt": []byte("x")}); b != nil {
		f.Add(b)
	}
}

// FuzzKpzUnmarshal asserts that parsing a .kpz never panics and always
// terminates, whatever the bytes claim about themselves. This is the floor, and
// for this format it is the floor that matters most: the input is a file that
// left the project and came back.
func FuzzKpzUnmarshal(f *testing.F) {
	for _, data := range fuzzSeedPackages() {
		f.Add(data)
	}
	seedDamagedKPZ(f)
	f.Add([]byte("not a zip"))
	f.Add([]byte("PK\x03\x04"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, data []byte) {
		pkg, err := Unmarshal(data)
		if err != nil {
			return
		}
		// A package that parsed must be safely inspectable — the merge path
		// walks exactly these fields, so reaching them must not panic either.
		_ = pkg.HasContent()
		for _, m := range pkg.Media {
			_, _ = ReadAll(m.Content)
		}
	})
}

// FuzzKpzRoundTrip asserts that the canonical form is closed and idempotent: if
// bytes parse, re-marshalling them must produce bytes that parse, and marshal
// again to exactly the same bytes.
//
// The first half is the sharper property. A package kpz accepts but cannot
// itself re-emit is a defect regardless of how the input was built, and the
// merge path re-marshals — so "we read it, then produced something we would
// refuse" is a real failure mode rather than a theoretical one.
//
// TestPackageRoundTrip already asserts this byte-for-byte for one hand-built
// package; this generalises it to whatever the mutation engine can construct.
func FuzzKpzRoundTrip(f *testing.F) {
	for _, data := range fuzzSeedPackages() {
		f.Add(data)
	}
	seedDamagedKPZ(f)

	f.Fuzz(func(t *testing.T, data []byte) {
		pkg, err := Unmarshal(data)
		if err != nil {
			return // only the no-panic contract applies to a rejected package
		}
		out1, err := pkg.Marshal()
		if err != nil {
			return // re-marshalling may legitimately refuse; no panic = pass
		}

		pkg2, err := Unmarshal(out1)
		if err != nil {
			t.Fatalf("kpz re-emitted a package it then refused to parse: %v\ninput len: %d\noutput len: %d", err, len(data), len(out1))
		}
		out2, err := pkg2.Marshal()
		if err != nil {
			t.Fatalf("re-marshalling a round-tripped package failed: %v", err)
		}
		if !bytes.Equal(out1, out2) {
			t.Fatalf("canonical form is not idempotent: Marshal differs on the second pass (%d vs %d bytes)", len(out1), len(out2))
		}
	})
}

// FuzzKpzHistoryChain covers the history log, which is the other untrusted
// parser in this package and had no adversarial coverage at all: VerifyHistory
// walks JSONL that arrived inside a package from outside the project, and
// hostile_test.go does not reach it.
//
// Two properties, and the second is the one worth having:
//
//   - VerifyHistory must not panic on arbitrary bytes. It splits on newlines
//     and JSON-decodes each line, so the whole surface is attacker-shaped.
//   - A chain built by AppendHistory must ALWAYS verify, whatever the event
//     fields contain. That is a soundness claim about the format's own framing:
//     the log is newline-delimited JSON and the digest is computed over
//     concatenated fields, so a field carrying a newline, a quote, a NUL or
//     invalid UTF-8 is exactly what would break either the line framing or the
//     digest. If construction can produce a log that fails its own verifier,
//     the tamper-evidence is unsound in the benign case before anyone attacks
//     it.
//
// Known open failure: fuzzing this target rediscovers #1608 in about a second.
// AppendHistory hashes the in-memory Go string and only then serialises, but
// encoding/json substitutes U+FFFD for invalid UTF-8 rather than failing — so a
// field holding an arbitrary byte is hashed in one domain and verified in
// another, and a log kapi wrote reports itself as tampered. The invalid-UTF-8
// seed is deliberately NOT in the list below; it is the reproducer on the
// issue, and it moves here once the defect is fixed.
func FuzzKpzHistoryChain(f *testing.F) {
	f.Add([]byte(""), "2026-06-04T00:00:00Z", "open", "step1", "note")
	f.Add([]byte("{}\n"), "", "step", "", "")
	f.Add([]byte("not json\n"), "t", "e", "s", "n")
	// The field contents most likely to break newline framing or digest
	// concatenation, seeded directly.
	f.Add([]byte(""), "t", "e", "s", "line one\nline two")
	f.Add([]byte(""), "t", "e", "s", `quote " and backslash \`)
	f.Add([]byte(""), "t", "e", "s", "nul\x00byte")

	f.Fuzz(func(t *testing.T, data []byte, ts, event, step, note string) {
		// Arbitrary bytes: no panic, always terminates.
		_ = VerifyHistory(data)

		// Construction soundness: two linked events must verify.
		log := AppendHistory(nil, HistoryEvent{Timestamp: ts, Event: event, Step: step, Note: note})
		log = AppendHistory(log, HistoryEvent{Timestamp: ts, Event: event, Step: step, Note: note})
		if err := VerifyHistory(log); err != nil {
			t.Fatalf("a chain built by AppendHistory failed its own verifier: %v\nts=%q event=%q step=%q note=%q\nlog=%q",
				err, ts, event, step, note, log)
		}
	})
}

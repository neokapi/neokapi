// Client-only ICU4X bridge for the segmentation lab.
//
// The Go-wasm "uax29" segmentation engine (core/segment/icu4xjs) has no ICU in
// the browser, so it calls out via syscall/js to a host-provided global:
//
//   globalThis.kapiICU4XSentenceBreaks(text, locale) => number[]  (code-point offsets)
//
// This module satisfies that contract using ICU4X's official `icu` npm package
// (ESM, wasm loaded via top-level await + import.meta.url). It must run in the
// browser only — `icu` is SSR-fragile — so it is imported lazily from a
// client-only component and dynamic-imports `icu` itself.

let loading: Promise<void> | null = null;

/**
 * Load ICU4X (once) and install globalThis.kapiICU4XSentenceBreaks. Resolves
 * when the ICU4X wasm is ready and the bridge is installed. A no-op on the
 * server or once already installed.
 */
export function loadICU4X(): Promise<void> {
  if (typeof window === "undefined") return Promise.resolve();
  if (icu4xReady()) return Promise.resolve();
  if (loading) return loading;
  loading = (async () => {
    // Dynamic import so the icu chunk (and its ~6.6 MB wasm) loads only when the
    // lab needs UAX-29, never during SSR or on other pages. The `icu` package is
    // patched (patches/icu@2.2.1.patch) to always use its browser wasm loader —
    // its stock build branches on `globalThis.process`, which Docusaurus shims at
    // runtime, wrongly routing it to a Node fs path that fails in the browser.
    const icu = (await import("icu")) as unknown as {
      SentenceSegmenter: new () => { segment(s: string): { next(): number } };
    };
    // Sentence breaking is locale-independent in ICU4X (the default segmenter
    // takes no content locale), so one segmenter serves every locale.
    const seg = new icu.SentenceSegmenter();
    (globalThis as Record<string, unknown>).kapiICU4XSentenceBreaks = (
      text: string,
      _locale: string,
    ): number[] => {
      const it = seg.segment(text);
      // ICU4X yields ascending UTF-16 boundary offsets, 0 and text.length
      // included; keep the interior ones.
      const u16: number[] = [];
      for (let b = it.next(); b !== -1; b = it.next()) {
        if (b > 0 && b < text.length) u16.push(b);
      }
      // Convert UTF-16 offsets → code-point offsets in one pass (the Go side
      // anchors spans by rune). A non-BMP char is 2 UTF-16 units = 1 code point.
      const out: number[] = [];
      let i = 0;
      let cp = 0;
      for (let k = 0; k < u16.length; k++) {
        while (i < u16[k]) {
          const c = text.codePointAt(i) as number;
          i += c > 0xffff ? 2 : 1;
          cp++;
        }
        out.push(cp);
      }
      return out;
    };
  })();
  return loading;
}

/** Whether the ICU4X bridge global is installed and ready. */
export function icu4xReady(): boolean {
  return typeof (globalThis as Record<string, unknown>).kapiICU4XSentenceBreaks === "function";
}

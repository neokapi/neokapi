// Client-only Intl.Segmenter bridge for the segmentation lab + WASM CLI.
//
// The Go-wasm "intl" segmentation engine (core/segment/intljs) has no native
// segmenter in the browser, so it calls out via syscall/js to a host-provided
// global:
//
//   globalThis.kapiIntlSentenceBreaks(text, locale) => number[]  (code-point offsets)
//
// Unlike the ICU4X ("uax29") bridge, this needs no companion wasm — Intl.Segmenter
// is built into the browser — so installing it is synchronous and free.

// installIntlSegmenter installs globalThis.kapiIntlSentenceBreaks (idempotent).
// No-op on the server. Returns whether the platform supports Intl.Segmenter.
export function installIntlSegmenter(): boolean {
  if (typeof window === "undefined") return false;
  const Seg = (Intl as unknown as { Segmenter?: typeof Intl.Segmenter }).Segmenter;
  if (!Seg) return false;
  if (intlSegmenterReady()) return true;
  (globalThis as Record<string, unknown>).kapiIntlSentenceBreaks = (
    text: string,
    locale: string,
  ): number[] => {
    const seg = new Seg(locale || "en", { granularity: "sentence" });
    // Each sentence segment carries its UTF-16 start index; the interior
    // boundaries are those start indices > 0 (the first segment starts at 0).
    const u16: number[] = [];
    for (const part of seg.segment(text)) {
      const idx = (part as { index: number }).index;
      if (idx > 0 && idx < text.length) u16.push(idx);
    }
    // Convert UTF-16 offsets → code-point offsets (the Go side anchors spans by
    // rune). A non-BMP char is 2 UTF-16 units = 1 code point.
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
  return true;
}

/** Whether the Intl.Segmenter bridge global is installed. */
export function intlSegmenterReady(): boolean {
  return typeof (globalThis as Record<string, unknown>).kapiIntlSentenceBreaks === "function";
}

/** Whether the platform provides Intl.Segmenter at all. */
export function intlSegmenterSupported(): boolean {
  return (
    typeof window !== "undefined" &&
    typeof (Intl as unknown as { Segmenter?: unknown }).Segmenter === "function"
  );
}

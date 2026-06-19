// Minimal OOXML (.docx/.pptx/.xlsx) embedded-image extractor for the browser.
//
// An OOXML file is a ZIP; embedded pictures are stored verbatim under
// `word/media/` (or `ppt/media`, `xl/media`). This reads those entries straight
// from the package — the same bytes the engine's openxml reader surfaces as a
// Media part (ExtractMedia) — so the Vision Lab can OCR/layout an image embedded
// in a document. It is extraction, not a reimplemented OOXML parser: the image
// bytes are the document's own, and the vision models that run on them are real.
//
// Uses the platform DecompressionStream("deflate-raw") — no dependency. Parses
// the central directory (robust to Word's data-descriptor local headers).

export interface EmbeddedImage {
  name: string;
  bytes: Uint8Array;
  mime: string;
}

const EOCD_SIG = 0x06054b50;
const CEN_SIG = 0x02014b50;
const LOC_SIG = 0x04034b50;

function mimeFor(name: string): string {
  const n = name.toLowerCase();
  if (n.endsWith(".png")) return "image/png";
  if (n.endsWith(".jpg") || n.endsWith(".jpeg")) return "image/jpeg";
  if (n.endsWith(".gif")) return "image/gif";
  return "";
}

async function inflateRaw(data: Uint8Array): Promise<Uint8Array> {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const DS = (globalThis as any).DecompressionStream;
  if (!DS) throw new Error("ooxml: DecompressionStream is unavailable in this browser");
  // Uint8Array is a valid BodyInit (BufferSource) at runtime; the cast sidesteps
  // the ArrayBufferLike generic-variance mismatch in the DOM lib types.
  const stream = new Response(data as BodyInit).body!.pipeThrough(new DS("deflate-raw"));
  return new Uint8Array(await new Response(stream).arrayBuffer());
}

/**
 * extractEmbeddedImages returns the PNG/JPEG/GIF pictures embedded under a media
 * folder of an OOXML package, in archive order.
 */
export async function extractEmbeddedImages(zip: Uint8Array): Promise<EmbeddedImage[]> {
  const dv = new DataView(zip.buffer, zip.byteOffset, zip.byteLength);

  // Locate the End Of Central Directory record (scan back over the ≤64 KB comment).
  let eocd = -1;
  for (let i = zip.length - 22; i >= 0 && i >= zip.length - 22 - 0x10000; i--) {
    if (dv.getUint32(i, true) === EOCD_SIG) {
      eocd = i;
      break;
    }
  }
  if (eocd < 0) throw new Error("ooxml: not a zip (no EOCD)");
  const count = dv.getUint16(eocd + 10, true);
  let p = dv.getUint32(eocd + 16, true); // central directory offset

  const out: EmbeddedImage[] = [];
  for (let i = 0; i < count; i++) {
    if (dv.getUint32(p, true) !== CEN_SIG) break;
    const method = dv.getUint16(p + 10, true);
    const compSize = dv.getUint32(p + 20, true);
    const nameLen = dv.getUint16(p + 28, true);
    const extraLen = dv.getUint16(p + 30, true);
    const commentLen = dv.getUint16(p + 32, true);
    const localOff = dv.getUint32(p + 42, true);
    const name = new TextDecoder().decode(zip.subarray(p + 46, p + 46 + nameLen));
    p += 46 + nameLen + extraLen + commentLen;

    const mime = mimeFor(name);
    if (!/\/media\//.test(name) || !mime) continue;

    // Resolve the entry's data offset from its local header (its name/extra
    // lengths can differ from the central record's).
    if (dv.getUint32(localOff, true) !== LOC_SIG) continue;
    const lNameLen = dv.getUint16(localOff + 26, true);
    const lExtraLen = dv.getUint16(localOff + 28, true);
    const dataStart = localOff + 30 + lNameLen + lExtraLen;
    const comp = zip.subarray(dataStart, dataStart + compSize);
    const bytes = method === 0 ? comp.slice() : await inflateRaw(comp);
    out.push({ name, bytes, mime });
  }
  return out;
}

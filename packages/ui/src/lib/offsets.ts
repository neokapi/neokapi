// offsets — convert between the position conventions the engine and the browser
// count in.
//
// Go indexes strings by UTF-8 byte, so every position a Go producer reports —
// an entity span, a term match, a check finding — is a byte offset into the
// block's plain text. JavaScript indexes UTF-16 code units, and the content
// model counts code points (Go's rune). Reading one as the other silently
// misplaces a highlight by one position per non-ASCII character before it.

/** UTF-8 byte length of one code point. */
export function utf8Length(ch: string): number {
  const cp = ch.codePointAt(0) ?? 0;
  if (cp < 0x80) return 1;
  if (cp < 0x800) return 2;
  if (cp < 0x10000) return 3;
  return 4;
}

/** Code-point offset of a UTF-8 byte offset into `text`. */
export function byteToCharOffset(text: string, byteOffset: number): number {
  if (byteOffset <= 0) return 0;
  let bytes = 0;
  let chars = 0;
  for (const ch of text) {
    if (bytes >= byteOffset) return chars;
    bytes += utf8Length(ch);
    chars++;
  }
  return chars;
}

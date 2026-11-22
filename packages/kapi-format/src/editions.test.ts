import { readFileSync } from "node:fs";

import { describe, expect, it } from "vite-plus/test";

import { formatEditionKey, parseEditionKey, type EditionKeyParts } from "./editions.ts";

interface Vector extends EditionKeyParts {
  text: string;
  /** Text the printer never writes (a malformed escape, a bare `=` or `%`): read, not printed. */
  readOnly?: boolean;
}

// The vectors the Go printer and parsers are tested against
// (core/model/edition_key_text_test.go), so the two languages agree.
const vectors: Vector[] = JSON.parse(
  readFileSync(
    new URL("../../../core/model/testdata/edition_key_text.json", import.meta.url),
    "utf8",
  ),
);

describe("edition key text form", () => {
  it("has vectors", () => {
    expect(vectors.length).toBeGreaterThan(0);
  });

  for (const v of vectors) {
    const parts: EditionKeyParts = { locale: v.locale };
    if (v.tone) parts.tone = v.tone;
    if (v.channel) parts.channel = v.channel;

    if (!v.readOnly) {
      it(`prints ${JSON.stringify(parts)} as ${v.text}`, () => {
        expect(formatEditionKey(parts)).toBe(v.text);
      });
    }

    it(`reads ${v.text} back`, () => {
      expect(parseEditionKey(v.text)).toEqual(parts);
    });
  }
});

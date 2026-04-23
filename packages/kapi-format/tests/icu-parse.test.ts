import { describe, expect, it } from "vitest";

import type { Run } from "../src/index.ts";
import { flattenRuns, parseICUPluralString } from "../src/index.ts";

describe("parseICUPluralString", () => {
  it("parses a simple two-form plural with default text parser", () => {
    const result = parseICUPluralString("{count, plural, one {1 message} other {# messages}}");
    expect(result).toEqual([
      {
        plural: {
          pivot: "count",
          forms: {
            one: [{ text: "1 message" }],
            other: [{ text: "# messages" }],
          },
        },
      },
    ]);
  });

  it("preserves all six CLDR forms", () => {
    const src =
      "{items, plural, zero {none} one {one} two {two} few {a few} many {many} other {other}}";
    const result = parseICUPluralString(src);
    const forms = (result![0] as { plural: { forms: Record<string, Run[]> } }).plural.forms;
    expect(Object.keys(forms).sort()).toEqual(
      ["few", "many", "one", "other", "two", "zero"].sort(),
    );
  });

  it("round-trips byte-identically with flattenRuns", () => {
    const src =
      "{count, plural, one {Sie haben 1 Nachricht} other {Sie haben {count} Nachrichten}}";
    const runs = parseICUPluralString(src, (s) => parseSimpleContent(s))!;
    expect(flattenRuns(runs)).toBe(src);
  });

  it("respects nested braces inside a form", () => {
    // Placeholder `{count}` inside the `other` branch must not be
    // treated as a brace boundary.
    const src = "{count, plural, one {Eins} other {{count} items}}";
    const result = parseICUPluralString(src);
    const other = (result![0] as { plural: { forms: { other: Run[] } } }).plural.forms.other;
    expect(other).toEqual([{ text: "{count} items" }]);
  });

  it("handles deeply-nested braces (paired codes inside a form)", () => {
    const src = "{n, plural, one {{=m0}Click here{/=m0}} other {{=m0}{n} more{/=m0}}}";
    const runs = parseICUPluralString(src)!;
    expect(runs).toEqual([
      {
        plural: {
          pivot: "n",
          forms: {
            one: [{ text: "{=m0}Click here{/=m0}" }],
            other: [{ text: "{=m0}{n} more{/=m0}" }],
          },
        },
      },
    ]);
  });

  it("routes explicit cases =0 / =1 / =2 onto the matching CLDR form", () => {
    const result = parseICUPluralString("{n, plural, =0 {none} =1 {one} =2 {two} other {many}}");
    const forms = (result![0] as { plural: { forms: Record<string, Run[]> } }).plural.forms;
    expect(forms.zero).toEqual([{ text: "none" }]);
    expect(forms.one).toEqual([{ text: "one" }]);
    expect(forms.two).toEqual([{ text: "two" }]);
    expect(forms.other).toEqual([{ text: "many" }]);
  });

  it("collapses =N (N>2) to other", () => {
    const result = parseICUPluralString("{n, plural, =42 {ok}}");
    const forms = (result![0] as { plural: { forms: Record<string, Run[]> } }).plural.forms;
    expect(Object.keys(forms)).toEqual(["other"]);
  });

  it("returns null for non-plural input", () => {
    expect(parseICUPluralString("Hello")).toBeNull();
    expect(parseICUPluralString("{name}")).toBeNull();
    expect(parseICUPluralString("")).toBeNull();
    expect(parseICUPluralString("{count, select, admin {hi} other {hi}}")).toBeNull();
  });

  it("returns null for malformed input", () => {
    // Missing closing brace
    expect(parseICUPluralString("{n, plural, one {ok}")).toBeNull();
    // No forms
    expect(parseICUPluralString("{n, plural, }")).toBeNull();
    // Unknown form name
    expect(parseICUPluralString("{n, plural, singular {ok}}")).toBeNull();
    // Missing pivot
    expect(parseICUPluralString("{, plural, one {ok}}")).toBeNull();
  });

  it("is insensitive to surrounding whitespace", () => {
    const result = parseICUPluralString("   {count, plural, one {a} other {b}}   ");
    expect(result).not.toBeNull();
  });

  it("uses a custom content parser to produce typed runs", () => {
    const parseContent = (s: string): Run[] => {
      // Split on {count} so the test can assert the placeholder run is
      // reconstructed with typed metadata.
      const parts = s.split(/(\{count\})/);
      const out: Run[] = [];
      for (const p of parts) {
        if (p === "") continue;
        if (p === "{count}") {
          out.push({ ph: { id: "count", type: "jsx:var", data: "{count}", equiv: "count" } });
        } else {
          out.push({ text: p });
        }
      }
      return out;
    };
    const src = "{count, plural, one {1 item} other {{count} items}}";
    const runs = parseICUPluralString(src, parseContent)!;
    const other = (runs[0] as { plural: { forms: { other: Run[] } } }).plural.forms.other;
    expect(other).toEqual([
      { ph: { id: "count", type: "jsx:var", data: "{count}", equiv: "count" } },
      { text: " items" },
    ]);
    // Round-trip: flattenRuns must reproduce the source string.
    expect(flattenRuns(runs)).toBe(src);
  });
});

/**
 * Minimal content parser for round-trip tests — just splits on
 * `{token}` and creates bare `ph` runs with equiv == id. The real
 * consumer (PluralTargetCell) uses the block's placeholder table to
 * populate typed metadata.
 */
function parseSimpleContent(text: string): Run[] {
  const out: Run[] = [];
  let buffer = "";
  let i = 0;
  while (i < text.length) {
    if (text[i] === "{") {
      const end = text.indexOf("}", i);
      if (end < 0) {
        buffer += text.slice(i);
        break;
      }
      const inner = text.slice(i + 1, end);
      if (/^[A-Za-z_][A-Za-z0-9_]*$/.test(inner)) {
        if (buffer) {
          out.push({ text: buffer });
          buffer = "";
        }
        out.push({ ph: { id: inner, type: "jsx:var", data: `{${inner}}`, equiv: inner } });
        i = end + 1;
        continue;
      }
      buffer += text[i];
      i++;
    } else {
      buffer += text[i];
      i++;
    }
  }
  if (buffer) out.push({ text: buffer });
  return out;
}

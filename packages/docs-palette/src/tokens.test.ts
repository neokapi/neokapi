import { describe, expect, it } from "vitest";

import { importedFiles, mergePalettes, readBlock, readPalette, stripComments } from "./tokens.ts";

const SAMPLE = `
/* a header comment
   --commented-out: oklch(0 0 0);
 */
@import "./semantic-colors.css";
@import url("./other.css");

:root {
  --background: oklch(0.985 0.003 240); /* trailing note */
  --brand-font-sans:
    "Inter Variable", "Inter",
    sans-serif;
}

.dark {
  --background: oklch(0.2 0.015 255);
}

.darker {
  --background: oklch(0 0 0);
}

.dark .kdx {
  --kdx-io: #ffffff;
}
`;

describe("stripComments", () => {
  it("removes a declaration that is inside a comment", () => {
    expect(stripComments(SAMPLE)).not.toContain("--commented-out");
  });
});

describe("importedFiles", () => {
  it("lists both the bare and the url() form, in source order", () => {
    expect(importedFiles(SAMPLE)).toEqual(["./semantic-colors.css", "./other.css"]);
  });
});

describe("readBlock", () => {
  it("reads the declarations of the named block", () => {
    expect(readBlock(SAMPLE, ":root").get("background")).toBe("oklch(0.985 0.003 240)");
  });

  it("folds a value that the source wrapped across lines", () => {
    expect(readBlock(SAMPLE, ":root").get("brand-font-sans")).toBe(
      '"Inter Variable", "Inter", sans-serif',
    );
  });

  it("matches the selector whole, never as the head of a longer class", () => {
    expect(readBlock(SAMPLE, ".dark").get("background")).toBe("oklch(0.2 0.015 255)");
  });

  it("ignores a descendant selector that merely starts with the same class", () => {
    expect(readBlock(SAMPLE, ".dark").has("kdx-io")).toBe(false);
  });

  it("returns an empty map for a selector the file does not open", () => {
    expect(readBlock(SAMPLE, ".light").size).toBe(0);
  });

  it("lets a later block of the same selector win, as the cascade does", () => {
    const twice = ":root { --a: 1; }\n:root { --a: 2; --b: 3; }";
    const tokens = readBlock(twice, ":root");
    expect(tokens.get("a")).toBe("2");
    expect(tokens.get("b")).toBe("3");
  });

  it("skips past a nested block rather than stopping at its closing brace", () => {
    const nested = ":root { --a: 1; }\n@media (min-width: 1px) { .x { color: red; } }\n";
    expect(readBlock(nested, ":root").get("a")).toBe("1");
  });
});

describe("readPalette and mergePalettes", () => {
  it("keeps the two themes apart", () => {
    const palette = readPalette(SAMPLE);
    expect(palette.light.get("background")).toBe("oklch(0.985 0.003 240)");
    expect(palette.dark.get("background")).toBe("oklch(0.2 0.015 255)");
  });

  it("lets the later palette override the earlier one, per theme", () => {
    const earlier = readPalette(":root { --a: 1; --b: 1; }\n.dark { --a: 1; }");
    const later = readPalette(":root { --a: 2; }\n.dark { --c: 9; }");
    const merged = mergePalettes(earlier, later);
    expect([...merged.light]).toEqual([
      ["a", "2"],
      ["b", "1"],
    ]);
    expect([...merged.dark]).toEqual([
      ["a", "1"],
      ["c", "9"],
    ]);
  });
});

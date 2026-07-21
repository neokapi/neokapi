/**
 * The extractor recognizes `@neokapi/i18n-react/head` hook calls and emits one
 * block per head string, hashed with the shared `head` descriptor — the same
 * hash the runtime hook self-computes, so the translation lands in the catalog
 * under the key the review store is later asked for.
 */

import { describe, expect, it } from "vitest";

import { extractDocument } from "../src/extract/index.ts";
import { hashKey } from "../src/plugin/hash.ts";
import { HEAD_DESCRIPTOR } from "../src/types.ts";

const SOURCE = `
import { useTranslatedTitle, useTranslatedDescription, useTranslatedMeta } from "@neokapi/i18n-react/head";

function Head() {
  useTranslatedTitle("Home Page");
  useTranslatedDescription("Welcome to our store.");
  useTranslatedMeta("Home Page", { property: "og:title" });
  return null;
}
`;

describe("head-hook extraction", () => {
  it("emits a block per head string, hashed with the head descriptor", () => {
    const doc = extractDocument(SOURCE, { filename: "Head.tsx" });
    expect(doc).not.toBeNull();
    const blocks = doc!.blocks;

    const byHash = new Map(blocks.map((b) => [b.hash, b]));
    const titleHash = hashKey("Home Page", HEAD_DESCRIPTOR);
    const descHash = hashKey("Welcome to our store.", HEAD_DESCRIPTOR);

    expect(byHash.has(titleHash)).toBe(true);
    expect(byHash.has(descHash)).toBe(true);

    const title = byHash.get(titleHash)!;
    expect(title.type).toBe("js:t");
    expect(title.properties.element).toBe("head");
    expect(title.properties.jsxPath).toBe("head");
    expect(title.source).toEqual([{ text: "Home Page" }]);
  });

  it("dedupes head kinds that share a source into one translation", () => {
    // useTranslatedTitle("Home Page") and useTranslatedMeta("Home Page", …)
    // share a source, so they collapse to a single catalog entry.
    const doc = extractDocument(SOURCE, { filename: "Head.tsx" });
    const homePageBlocks = doc!.blocks.filter((b) => b.source[0]?.text === "Home Page");
    expect(homePageBlocks).toHaveLength(1);
  });

  it("ignores head calls whose argument isn't a string literal", () => {
    const dynamic = `
import { useTranslatedTitle } from "@neokapi/i18n-react/head";
function Head({ t }: { t: string }) {
  useTranslatedTitle(t);
  return null;
}
`;
    const doc = extractDocument(dynamic, { filename: "Head.tsx" });
    expect(doc).toBeNull();
  });

  it("does not sweep up a like-named local that isn't the head import", () => {
    const local = `
function useTranslatedTitle(s: string) { return s; }
function Head() {
  useTranslatedTitle("Not extracted");
  return null;
}
`;
    const doc = extractDocument(local, { filename: "Head.tsx" });
    expect(doc).toBeNull();
  });
});

/**
 * The browser-safe runtime hasher (`runtime/hash.ts`) must produce byte-for-byte
 * the same key as the Node build-side hasher (`plugin/hash.ts`). The head hooks
 * self-hash their source at render time and the extractor hashes the same source
 * on the build side; if the two hashers drift, a head string's runtime lookup
 * misses its catalog entry and the review store 404s on save.
 */

import { describe, expect, it } from "vitest";

import { hashKey as runtimeHash } from "../src/runtime/hash.ts";
import { hashKey as buildHash } from "../src/plugin/hash.ts";
import { HEAD_DESCRIPTOR } from "../src/types.ts";

const TEXTS = [
  "",
  "Home",
  "Dashboard · Acme",
  "Manage your team, projects, and billing.",
  "Über uns", // non-ASCII
  "日本語のタイトル", // CJK
  "Price: $5 & rising", // ampersand + special chars
  'He said "hi"\n\ttab', // quotes, newline, tab — JSON.stringify escapes
  "emoji 🚀 title", // astral plane
];

const DESCRIPTORS = [HEAD_DESCRIPTOR, "t\x1f", "t\x1fUI language", "Page>h1"];

describe("runtime hash parity with the build hasher", () => {
  it("matches plugin/hash for every (text, descriptor) pair", () => {
    for (const text of TEXTS) {
      for (const desc of DESCRIPTORS) {
        expect(runtimeHash(text, desc)).toBe(buildHash(text, desc));
      }
    }
  });

  it("produces a stable, base62 key for a head string", () => {
    const key = runtimeHash("Home", HEAD_DESCRIPTOR);
    expect(key).toMatch(/^[0-9A-Za-z]+$/);
    expect(key).toBe(buildHash("Home", HEAD_DESCRIPTOR));
  });
});

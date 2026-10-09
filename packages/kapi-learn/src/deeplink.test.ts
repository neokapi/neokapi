import { describe, expect, it } from "vitest";
import {
  deserializeSession,
  diffAgainstSeed,
  formatLabLink,
  parseLabLink,
  serializeSession,
  SESSION_MAX_BYTES,
} from "./deeplink.ts";

const enc = new TextEncoder();

describe("deeplink", () => {
  it("round-trips a chapter and a session through the query string", () => {
    const session = {
      files: [{ path: "decisions.jsonl", content: '{"op":"term","term":"dock"}\n' }],
      removed: ["index.before.html"],
    };
    const url = formatLabLink("/learn/northsea-checks", { chapter: "decision", session });
    expect(url.startsWith("/learn/northsea-checks?c=decision&s=")).toBe(true);
    const link = parseLabLink(new URL(url, "https://example.test").search);
    expect(link.chapter).toBe("decision");
    expect(link.session).toEqual(session);
  });

  it("keeps non-ASCII content intact", () => {
    const session = {
      files: [{ path: "nb.json", content: '{"a":"Kaiplan – fartøy ⟦nb⟧"}' }],
      removed: [],
    };
    const token = serializeSession(session)!;
    expect(token).toMatch(/^[A-Za-z0-9_-]+$/);
    expect(deserializeSession(token)).toEqual(session);
  });

  it("ignores a malformed or foreign token and an odd chapter id", () => {
    expect(parseLabLink("?c=Not%20Valid&s=%%%")).toEqual({});
    expect(deserializeSession(btoa('{"v":9}'))).toBeNull();
    expect(parseLabLink("c=gate")).toEqual({ chapter: "gate" });
  });

  it("refuses a session that would not fit in a link", () => {
    const big = {
      files: [{ path: "big.txt", content: "x".repeat(SESSION_MAX_BYTES + 1) }],
      removed: [],
    };
    expect(serializeSession(big)).toBeNull();
    expect(formatLabLink("/learn/x", { chapter: "a", session: big })).toBe("/learn/x?c=a");
  });

  it("diffs the sandbox against the seed", () => {
    const seed = [
      { path: "a.md", content: "one" },
      { path: "b.md", content: "two" },
      { path: "gone.md", content: "three" },
    ];
    const current = [
      { path: "a.md", bytes: enc.encode("one") },
      { path: "b.md", bytes: enc.encode("two, edited") },
      { path: "new.jsonl", bytes: enc.encode("{}") },
      { path: "bin.dat", bytes: new Uint8Array([0xff, 0xfe, 0x00]) },
    ];
    expect(diffAgainstSeed(seed, current)).toEqual({
      files: [
        { path: "b.md", content: "two, edited" },
        { path: "new.jsonl", content: "{}" },
      ],
      removed: ["gone.md"],
    });
  });
});

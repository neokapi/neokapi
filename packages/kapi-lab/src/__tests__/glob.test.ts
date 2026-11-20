import { describe, expect, it } from "vite-plus/test";
import { globMatches, globToRegExp, isGlob, matchGlob } from "../glob";

describe("glob", () => {
  it("detects glob patterns", () => {
    expect(isGlob("*.json")).toBe(true);
    expect(isGlob("**/*.json")).toBe(true);
    expect(isGlob("a.json")).toBe(false);
    expect(isGlob("src/messages.json")).toBe(false);
  });

  it("matches a single-segment wildcard", () => {
    expect(matchGlob("*.json", ["a.json", "b.xml", "c.json", "src/d.json"])).toEqual([
      "a.json",
      "c.json",
    ]);
  });

  it("matches ** across directories", () => {
    const paths = ["a.json", "src/b.json", "src/x/c.json", "src/d.xml"];
    expect(matchGlob("**/*.json", paths).sort()).toEqual(
      ["a.json", "src/b.json", "src/x/c.json"].sort(),
    );
  });

  it.each([
    ["**/api.md", "api.md", true],
    ["**/api.md", "docs/api.md", true],
    ["**/api.md", "docs/otherapi.md", false],
    ["src/**/*.json", "src/en.json", true],
    ["src/*.json", "src/locale/en.json", false],
    ["a**b", "a/x/b", false],
    ["docs/**", "docs", true],
    ["**/{api,guide}.[!x]d", "docs/api.md", true],
    ["?.json", "𐐀.json", true],
    ["[z-a].json", "b.json", false],
    ["[unfinished", "[unfinished", false],
    ["{api,guide", "{api,guide", false],
  ])("matches %s against %s", (pattern, path, matched) => {
    expect(globMatches(pattern, path)).toBe(matched);
  });

  it("matches brace alternation", () => {
    expect(matchGlob("*.{json,xliff}", ["a.json", "b.xliff", "c.xml"])).toEqual([
      "a.json",
      "b.xliff",
    ]);
  });

  it("matches a single-character wildcard", () => {
    expect(globMatches("a?.json", "ab.json")).toBe(true);
    expect(globMatches("a?.json", "abc.json")).toBe(false);
  });

  it("treats dots literally", () => {
    expect(globToRegExp("a.json").test("axjson")).toBe(false);
  });

  it("returns nothing for an empty pattern", () => {
    expect(matchGlob("", ["a.json"])).toEqual([]);
  });
});

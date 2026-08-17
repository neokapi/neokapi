// The prefix a collection's items share, and how a name reads under it.
//
// Mirrors TestCommonItemBase in bowrain/server/editor_item_base_test.go: the
// overview trims server-side over a paged list, the source page trims
// client-side over the whole one, and a reader moving between them must see the
// same names.
import { describe, it, expect } from "vite-plus/test";

import { commonItemBase, relativeItemName } from "../components/collections/itemBase";

describe("commonItemBase", () => {
  it("names the directory every item shares", () => {
    expect(
      commonItemBase([
        "bowrain/packages/app/i18n/apps/web/src/main.kbf.json",
        "bowrain/packages/app/i18n/apps/web/src/App.kbf.json",
      ]),
    ).toBe("bowrain/packages/app/i18n/apps/web/src/");
  });

  it("shares whole segments only", () => {
    // "docs/api" and "docs/apps" share "docs/", never "docs/ap".
    expect(commonItemBase(["docs/api/a.md", "docs/apps/b.md"])).toBe("docs/");
  });

  it("gives a single item its own directory, so it reads as the file", () => {
    expect(commonItemBase(["bowrain/emails/i18n/src/credits-warning.kbf.json"])).toBe(
      "bowrain/emails/i18n/src/",
    );
  });

  it("shares nothing when the items sit in different roots", () => {
    expect(commonItemBase(["docs/a.md", "web/b.md"])).toBe("");
    expect(commonItemBase(["a.md", "docs/b.md"])).toBe("");
  });

  it("is empty for an empty collection", () => {
    expect(commonItemBase([])).toBe("");
  });
});

describe("relativeItemName", () => {
  it("reads a name under the base", () => {
    expect(
      relativeItemName("bowrain/packages/app/i18n/main.kbf.json", "bowrain/packages/app/"),
    ).toBe("i18n/main.kbf.json");
  });

  it("leaves a name the base does not prefix whole rather than half-trimmed", () => {
    expect(relativeItemName("web/other.md", "docs/")).toBe("web/other.md");
  });

  it("leaves every name whole when there is no base", () => {
    expect(relativeItemName("docs/a.md", "")).toBe("docs/a.md");
  });
});

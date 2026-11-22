import { describe, expect, it } from "vitest";

import {
  previewDictionary,
  rankStories,
  storiesForSource,
  storyLabel,
  storyURL,
  visibleFragments,
} from "../src/preview.ts";

const index = {
  entries: {
    a: {
      id: "offline--default",
      title: "App/OfflineLaunch",
      name: "Default",
      importPath: "./src/components/OfflineLaunch.stories.tsx",
      type: "story",
    },
    b: {
      id: "offline--docs",
      title: "App/OfflineLaunch",
      name: "Docs",
      importPath: "./src/components/OfflineLaunch.stories.tsx",
      type: "docs",
    },
    c: {
      id: "offline-banner--default",
      title: "App/OfflineBanner",
      name: "Default",
      importPath: "./src/components/OfflineBanner.stories.tsx",
      type: "story",
    },
  },
};

describe("live preview", () => {
  it("finds the stories beside the component's source file, docs pages left out", () => {
    const got = storiesForSource(index, "bowrain/apps/x/src/components/OfflineLaunch.tsx");
    expect(got.map((e) => e.id)).toEqual(["offline--default"]);
    expect(storiesForSource(index, "src/Other.tsx")).toEqual([]);
    expect(storiesForSource(index, "")).toEqual([]);
  });

  it("addresses one story without the Storybook chrome", () => {
    expect(storyURL("https://example.org/sb/", "offline--default")).toBe(
      "https://example.org/sb/iframe.html?id=offline--default&viewMode=story",
    );
    expect(storyLabel(index.entries.a)).toBe("OfflineLaunch · Default");
  });

  it("builds the dictionary from the units a runtime addresses", () => {
    expect(
      previewDictionary([
        { messageKey: "h1", runtimeText: "Réessayer" },
        { messageKey: "h2" },
        { runtimeText: "orphan" },
      ]),
    ).toEqual({ h1: "Réessayer" });
  });

  it("puts a story named for the component first and error or dark states last", () => {
    const e = (id: string, name: string) => ({
      id,
      name,
      title: "Components/Sheet",
      importPath: "./Sheet.stories.tsx",
      type: "story",
    });
    const got = rankStories(
      [
        e("a", "Connected No Permission"),
        e("b", "Overview tab (dark)"),
        e("c", "Map tab"),
        e("d", "PreviewBody"),
      ],
      "PreviewBody",
    );
    expect(got.map((x) => x.id)).toEqual(["d", "c", "b", "a"]);
  });

  it("finds the text a reader sees between codes, longest first", () => {
    expect(visibleFragments("Hei {name}, se {=m0}vilkårene{/=m0} her")).toEqual([
      "vilkårene",
      ", se",
      "Hei",
    ]);
    expect(visibleFragments("<=m0><serverURL/></=m0>")).toEqual([]);
    expect(visibleFragments("{count, plural, one {# fil} other {# filer}}")).toEqual([]);
  });
});

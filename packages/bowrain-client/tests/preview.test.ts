import { describe, expect, it } from "vitest";

import { previewDictionary, storiesForSource, storyLabel, storyURL } from "../src/preview.ts";

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
});

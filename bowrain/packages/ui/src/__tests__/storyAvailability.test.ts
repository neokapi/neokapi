/**
 * Which items a collection's preview host can actually show.
 *
 * A collection declaring a host is a claim about the collection, not about
 * every item in it: a Storybook renders the components someone wrote a story
 * for. These two functions are what let a list answer that per row before a
 * reviewer clicks — the answer used to arrive as "No story renders this item's
 * components", after the offer had been made.
 */
import { describe, it, expect } from "vite-plus/test";

import { hasStoryFor, storyComponents, type StoryIndex } from "../components/editor/storyIndex";

const index: StoryIndex = {
  entries: {
    "context-hub-reachpanel--default": {
      id: "context-hub-reachpanel--default",
      title: "Context/ReachPanel",
      name: "Default",
      importPath: "./src/context-hub/experiments/ReachPanel.stories.tsx",
    },
    "context-hub-reachpanel--empty": {
      id: "context-hub-reachpanel--empty",
      title: "Context/ReachPanel",
      name: "Empty",
      importPath: "./src/context-hub/experiments/ReachPanel.stories.tsx",
    },
    "context-hub-reachpanel--docs": {
      id: "context-hub-reachpanel--docs",
      title: "Context/ReachPanel",
      name: "Docs",
      importPath: "./src/context-hub/experiments/Guide.stories.mdx",
      type: "docs",
    },
  },
};

describe("storyComponents", () => {
  it("names each component once, however many stories it has", () => {
    expect(storyComponents(index)).toEqual(new Set(["ReachPanel"]));
  });

  // A docs page is a page ABOUT the component; embedding one shows a manual
  // rather than the thing being reviewed — the rule storiesForComponents keeps.
  it("skips docs entries", () => {
    expect(storyComponents(index).has("Guide")).toBe(false);
  });

  it("is empty for an index with no entries", () => {
    expect(storyComponents({ entries: {} })).toEqual(new Set());
  });
});

describe("hasStoryFor", () => {
  const components = storyComponents(index);

  it("matches a source file to the story written beside it", () => {
    expect(hasStoryFor(components, "../ui/src/context-hub/experiments/ReachPanel.tsx")).toBe(true);
  });

  it("matches on the basename, not on where either file sits", () => {
    // The catalog tree mirrors the extractor's roots and the Storybook follows
    // its own config, so the basename is the only thing the two agree on —
    // which is why storyIndex joins them there. Both sides run one componentOf.
    expect(hasStoryFor(components, "somewhere/else/ReachPanel.jsx")).toBe(true);
  });

  it("says no for a component the host publishes no story for", () => {
    expect(hasStoryFor(components, "../ui/src/Coordinates.tsx")).toBe(false);
  });

  // Only the JSX reader records the `file` property source_path comes from, so
  // an item of any other format carries none. Answering yes for one would offer
  // a reading componentsOf then declines to find.
  it("says no for an item that carries no source path", () => {
    expect(hasStoryFor(components, undefined)).toBe(false);
    expect(hasStoryFor(components, "")).toBe(false);
  });

  it("says no when the index carried nothing", () => {
    expect(hasStoryFor(new Set(), "../ui/src/ReachPanel.tsx")).toBe(false);
  });
});

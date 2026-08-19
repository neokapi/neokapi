// Finding the story that renders an item, and the dictionary it needs.
//
// The join is by convention: a component file sits beside its stories file, and
// a published Storybook's index names the latter. Nothing is written into the
// bundle at extraction time, so a component that gains stories later is matched
// the next time someone looks.
import { describe, it, expect } from "vite-plus/test";

import {
  componentsOf,
  storiesForComponents,
  embeddedStoryURL,
  storyIndexURL,
  storyURL,
  translationsFor,
  type StoryIndex,
} from "../components/editor/storyIndex";
import type { BlockInfo } from "../types/api";

function block(over: Partial<BlockInfo> = {}): BlockInfo {
  return {
    id: "b",
    source: "Your AI credits are running low",
    translatable: true,
    has_spans: false,
    properties: {},
    targets: {},
    ...over,
  };
}

const index: StoryIndex = {
  entries: {
    "emails-credits-warning--at-80-percent": {
      id: "emails-credits-warning--at-80-percent",
      title: "Emails/Credits Warning",
      name: "At 80 Percent",
      importPath: "../emails/src/credits-warning.stories.tsx",
      type: "story",
    },
    "emails-credits-warning--free-plan": {
      id: "emails-credits-warning--free-plan",
      title: "Emails/Credits Warning",
      name: "Free Plan",
      importPath: "../emails/src/credits-warning.stories.tsx",
      type: "story",
    },
    "emails-credits-warning--docs": {
      id: "emails-credits-warning--docs",
      title: "Emails/Credits Warning",
      name: "Docs",
      importPath: "../emails/src/credits-warning.stories.tsx",
      type: "docs",
    },
    "emails-invite--default": {
      id: "emails-invite--default",
      title: "Emails/Invite",
      name: "Default",
      importPath: "../emails/src/invite.stories.tsx",
      type: "story",
    },
  },
};

describe("componentsOf", () => {
  it("names the components an item's blocks came from, in document order", () => {
    const blocks = [
      block({ properties: { file: "src/credits-warning.tsx" } }),
      block({ properties: { file: "src/credits-warning.tsx" } }),
      block({ properties: { file: "src/components/Footer.tsx" } }),
    ];
    expect(componentsOf(blocks)).toEqual(["credits-warning", "Footer"]);
  });

  it("skips a block the extractor recorded no file for", () => {
    expect(componentsOf([block({ properties: {} })])).toEqual([]);
  });
});

describe("storiesForComponents", () => {
  it("finds every story rendering the component, and no docs page", () => {
    const found = storiesForComponents(index, ["credits-warning"]);
    expect(found.map((s) => s.id)).toEqual([
      "emails-credits-warning--at-80-percent",
      "emails-credits-warning--free-plan",
    ]);
  });

  it("finds nothing for a component no story renders", () => {
    expect(storiesForComponents(index, ["NoSuchThing"])).toEqual([]);
    expect(storiesForComponents(index, [])).toEqual([]);
  });
});

describe("story URLs", () => {
  it("addresses one story with no Storybook chrome, whatever the base's slashes", () => {
    expect(storyURL("https://x.dev/storybook/bowrain/", "emails-invite--default")).toBe(
      "https://x.dev/storybook/bowrain/iframe.html?id=emails-invite--default&viewMode=story",
    );
  });

  // The frame goes through the embed origin, never straight at the customer's
  // host: the application's content policy names that one origin, because the
  // hosts a recipe may declare cannot be listed in advance.
  it("frames a story through the embed origin", () => {
    expect(
      embeddedStoryURL(
        "https://embed.bowrain.cloud",
        "https://x.dev/sb/",
        "emails-invite--default",
      ),
    ).toBe(
      "https://embed.bowrain.cloud/?src=" +
        encodeURIComponent("https://x.dev/sb/iframe.html?id=emails-invite--default&viewMode=story"),
    );
  });

  // Without one configured there is nothing to frame through, and returning the
  // story URL directly would be refused by the policy — a blank frame with
  // nothing to explain it.
  it("offers no framed URL when no embed origin is configured", () => {
    expect(embeddedStoryURL("", "https://x.dev/sb/", "s--1")).toBeUndefined();
  });

  // The index comes through this project's own API. The browser cannot fetch it
  // from the customer's host: connect-src names 'self', and that host has no
  // reason to send Access-Control-Allow-Origin.
  it("reads the index through the project's API, per collection", () => {
    expect(storyIndexURL("bowrain-hq", "proj-1", "main", "col-9")).toBe(
      "/api/v1/bowrain-hq/proj-1/collections/main/col-9/preview/index",
    );
  });
});

describe("translationsFor", () => {
  it("keys the dictionary by the message key the running component looks up", () => {
    const blocks = [
      block({
        properties: { hash: "90j0egM1IoD", file: "src/credits-warning.tsx" },
        targets: { nb: { text: "Dine AI-kreditter er i ferd med å ta slutt", status: "draft" } },
      }),
      block({
        properties: { hash: "eXjOLgmiqT6" },
        targets: { nb: { text: "Oppgrader planen", status: "translated" } },
      }),
    ];
    expect(translationsFor(blocks, "nb")).toEqual({
      "90j0egM1IoD": "Dine AI-kreditter er i ferd med å ta slutt",
      eXjOLgmiqT6: "Oppgrader planen",
    });
  });

  it("carries a draft, which is the whole point — it is in no built catalog", () => {
    const drafted = block({
      properties: { hash: "abc" },
      targets: { nb: { text: "Utkast", status: "draft" } },
    });
    expect(translationsFor([drafted], "nb")).toEqual({ abc: "Utkast" });
  });

  it("leaves out a block with no key, rather than guessing one", () => {
    const keyless = block({ targets: { nb: { text: "Oversatt", status: "translated" } } });
    expect(translationsFor([keyless], "nb")).toEqual({});
  });

  it("leaves out a block with nothing translated yet", () => {
    expect(translationsFor([block({ properties: { hash: "abc" } })], "nb")).toEqual({});
  });
});

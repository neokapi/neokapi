import { describe, expect, it } from "vite-plus/test";
import { topicsFromDocs } from "../components/guide/docs-topics";

const page = {
  path: "/app/overview",
  source: `---
title: The app
---

import { ThemedImage } from "@neokapi/docs-shared";

{/* The Help panel reads the sections with an id. */}

## Install

Run the installer.

## The home {#home}

The home lists your projects. It opens first.

<ThemedImage
  alt="The home"
  sources={{ light: "/a.png", dark: "/b.png" }}
/>

- **Projects** opens one. See [Checks](#checks) and [the guide](/app/guide#start).

### Keys {#home-keys}

\`\`\`bash
## not a heading
\`\`\`

## Checks {#checks}

A check reads each block: it reports findings.

<Tabs>
  <TabItem value="a">A</TabItem>
</Tabs>

## Next
`,
};

const read = (specs: { id: string; section: string }[]) =>
  topicsFromDocs(page, specs, { siteUrl: "https://docs.example" });

describe("topicsFromDocs", () => {
  it("reads one section per topic, up to the next heading at its level", () => {
    const { topics, missing } = read([{ id: "home", section: "home" }]);
    expect(missing).toEqual([]);
    const [home] = topics;
    expect(home.title).toBe("The home");
    expect(home.summary).toBe("The home lists your projects.");
    expect(home.moreUrl).toBe("https://docs.example/app/overview#home");
    expect(home.body).toContain("### Keys");
    expect(home.body).not.toContain("{#home-keys}");
    expect(home.body).toContain("## not a heading");
    expect(home.body).not.toContain("A check reads");
  });

  it("leaves out screenshots and comments, and reports other components", () => {
    const { topics, unsupported } = read([
      { id: "home", section: "home" },
      { id: "checks", section: "checks" },
    ]);
    expect(topics[0].body).not.toMatch(/ThemedImage|sources=|\{\/\*/);
    expect(unsupported).toEqual(["checks: <Tabs>"]);
  });

  it("opens a section the app shows in the panel, and any other link on the site", () => {
    const withChecks = read([
      { id: "home", section: "home" },
      { id: "checks-topic", section: "checks" },
    ]).topics[0].body;
    expect(withChecks).toContain("[Checks](#topic=checks-topic)");
    expect(withChecks).toContain("[the guide](https://docs.example/app/guide#start)");

    const alone = read([{ id: "home", section: "home" }]).topics[0].body;
    expect(alone).toContain("[Checks](https://docs.example/app/overview#checks)");
  });

  it("ends the summary at a colon as a sentence", () => {
    expect(read([{ id: "c", section: "checks" }]).topics[0].summary).toBe(
      "A check reads each block.",
    );
  });

  it("reports a section the page does not have", () => {
    expect(read([{ id: "x", section: "gone" }])).toEqual({
      topics: [],
      missing: ["gone"],
      unsupported: [],
    });
  });
});

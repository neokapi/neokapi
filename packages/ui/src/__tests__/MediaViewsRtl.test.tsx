// @vitest-environment jsdom
//
// SubtitleTimeline and VideoPlayer's burned-in subtitle are a fourth and
// fifth place that render a block's own source/target prose — the same
// contract as the rest of DocumentViewer applies: the element itself carries
// dir/lang for that text's locale.
import { describe, it, expect } from "vitest";
import { createElement } from "react";
import { createRoot } from "react-dom/client";
import { act } from "react";

import SubtitleTimeline from "../components/preview/SubtitleTimeline";
import VideoPlayer from "../components/preview/VideoPlayer";
import type { ContentNode, ContentTree } from "../components/preview/types";

function render(el: React.ReactElement): HTMLDivElement {
  const container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    createRoot(container).render(el);
  });
  return container;
}

function cueTree(node: Partial<ContentNode>): ContentTree {
  return {
    format: "srt",
    stats: {},
    root: [
      {
        kind: "block",
        id: "c1",
        translatable: true,
        timing: { startMs: 0, endMs: 1000 },
        ...node,
      } as ContentNode,
    ],
  } as ContentTree;
}

describe("SubtitleTimeline — writing direction", () => {
  it("puts dir/lang on the source cue row's own element", () => {
    const t = cueTree({ sourceLocale: "ar", source: [{ text: "مرحباً بالعالم" }] });
    const c = render(createElement(SubtitleTimeline, { tree: t }));
    const row = c.querySelector('[data-testid="cue-source"]');
    expect(row?.getAttribute("dir")).toBe("rtl");
    expect(row?.getAttribute("lang")).toBe("ar");
  });

  it("puts dir/lang on the target cue row, keyed by its own variant locale", () => {
    const t = cueTree({
      sourceLocale: "en",
      source: [{ text: "Hello world" }],
      targets: { "ar-EG#formal": [{ text: "مرحباً بالعالم" }] },
    });
    const c = render(createElement(SubtitleTimeline, { tree: t, side: "ar-EG#formal" }));
    const row = c.querySelector('[data-testid="cue-target"]');
    expect(row?.getAttribute("dir")).toBe("rtl");
    expect(row?.getAttribute("lang")).toBe("ar-EG");
  });
});

describe("VideoPlayer — writing direction", () => {
  it("puts dir/lang on the burned-in subtitle for the active cue", () => {
    const t = cueTree({ sourceLocale: "ar", source: [{ text: "مرحباً بالعالم" }] });
    const c = render(createElement(VideoPlayer, { src: "video.mp4", tree: t, currentTimeMs: 500 }));
    const subtitle = c.querySelector('[data-testid="burned-subtitle"] [dir]');
    expect(subtitle?.getAttribute("dir")).toBe("rtl");
    expect(subtitle?.getAttribute("lang")).toBe("ar");
    expect(subtitle?.textContent).toContain("مرحباً");
  });
});

import { describe, expect, it } from "vite-plus/test";

import { resolveOverlaySpans } from "./overlayHighlight";
import type { OverlayView, Run } from "./types";

const runs: Run[] = [{ text: "the server and the server list" }];

const term = (start: number, end: number, text: string): OverlayView => ({
  type: "term",
  side: "source",
  spans: [
    {
      range: { kind: "range", start: { run: 0, offset: start }, end: { run: 0, offset: end } },
      text,
      props: { term: "server" },
    },
  ],
});

describe("resolveOverlaySpans", () => {
  it("places a span where its anchor points, not at the first match of its text", () => {
    // The second "server", which a text search would have put on the first.
    const [span] = resolveOverlaySpans([term(19, 25, "server")], "source", runs);
    expect(span).toMatchObject({ start: 19, end: 25, type: "term" });
  });

  it("drops a span whose anchor addresses nothing in the runs", () => {
    expect(resolveOverlaySpans([term(40, 46, "server")], "source", runs)).toEqual([]);
  });

  it("resolves only the overlays on the side asked for", () => {
    expect(resolveOverlaySpans([{ ...term(4, 10, "server"), side: "nb" }], "source", runs)).toEqual(
      [],
    );
  });
});

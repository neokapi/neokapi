// @vitest-environment jsdom
import { describe, it, expect, afterEach } from "vitest";
import { createElement } from "react";
import { createRoot } from "react-dom/client";
import { act } from "react";
import { OriginsPopover } from "../components/resource-browser/OriginsPopover";
import type { OriginDTO } from "../components/resource-browser/types";

function renderToContainer(el: React.ReactElement): HTMLDivElement {
  const container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    createRoot(container).render(el);
  });
  return container;
}

const SAMPLE_ORIGINS: OriginDTO[] = [
  {
    source: "file",
    key: "apps/web/locales/en-US.json:errors.notFound",
    reference: "commit:abc123",
    added_at: new Date().toISOString(),
    added_by: "tmx-import",
  },
  {
    source: "tool",
    key: "translate",
    reference: "job-42",
    added_at: new Date().toISOString(),
    added_by: "kapi",
  },
];

describe("OriginsPopover", () => {
  afterEach(() => {
    while (document.body.firstChild) {
      document.body.removeChild(document.body.firstChild);
    }
  });

  it("renders nothing when no origins and no note", () => {
    const c = renderToContainer(createElement(OriginsPopover, { origins: [] }));
    expect(c.textContent).toBe("");
  });

  it("renders count badge when origins are present", () => {
    const c = renderToContainer(createElement(OriginsPopover, { origins: SAMPLE_ORIGINS }));
    expect(c.textContent).toContain("2");
  });

  it("renders trigger when only a note is present", () => {
    const c = renderToContainer(
      createElement(OriginsPopover, { origins: [], note: "Translator context" }),
    );
    expect(c.querySelector("button")).toBeTruthy();
  });

  it("renders singular count of 1", () => {
    const c = renderToContainer(createElement(OriginsPopover, { origins: [SAMPLE_ORIGINS[0]] }));
    expect(c.textContent).toContain("1");
  });

  it("shows origin with session_id without loading state", () => {
    const origin: OriginDTO = {
      source: "import",
      key: "corpus.tmx",
      session_id: "sess-1",
      added_at: new Date().toISOString(),
      added_by: "kapi",
    };
    const c = renderToContainer(createElement(OriginsPopover, { origins: [origin] }));
    expect(c.textContent).toContain("1");
    // No "Loading session..." — the popover no longer fetches sessions
  });
});

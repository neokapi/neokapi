// @vitest-environment jsdom
import { describe, it, expect, vi, afterEach } from "vitest";
import { createElement } from "react";
import { createRoot } from "react-dom/client";
import { act } from "react";
import { OriginsPopover } from "../components/resource-browser/OriginsPopover";
import type { OriginDTO, ImportSessionDTO } from "../components/resource-browser/types";

function renderToContainer(el: React.ReactElement): HTMLDivElement {
  const container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    createRoot(container).render(el);
  });
  return container;
}

async function flush() {
  // Allow queued microtasks + effect callbacks to run.
  await act(async () => {
    await new Promise<void>((resolve) => setTimeout(resolve, 0));
  });
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
    key: "ai-translate",
    reference: "job-42",
    added_at: new Date().toISOString(),
    added_by: "kapi",
  },
];

const SESSION_ORIGIN: OriginDTO = {
  source: "import",
  key: "acme-glossary.tmx",
  session_id: "sess-1",
  added_at: new Date().toISOString(),
  added_by: "tmx-import",
};

const SAMPLE_SESSION: ImportSessionDTO = {
  id: "sess-1",
  file_key: "acme-glossary.tmx",
  file_hash: "abcdef",
  file_size_bytes: 1024,
  imported_at: new Date().toISOString(),
  imported_by: "tmx-import",
  tool_name: "tmx-import",
  tool_version: "1.0.0",
  seg_type: "sentence",
  admin_lang: "",
  src_lang: "en-US",
  data_type: "plaintext",
  original_format: "TMX 1.4",
  original_encoding: "UTF-8",
  entry_count: 42,
};

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

  it("fetches and caches session info when popover opens", async () => {
    const getImportSession = vi.fn(async (id: string) => {
      if (id === "sess-1") return SAMPLE_SESSION;
      return null;
    });

    const c = renderToContainer(
      createElement(OriginsPopover, {
        origins: [SESSION_ORIGIN],
        getImportSession,
      }),
    );

    // Open the popover.
    const trigger = c.querySelector("button") as HTMLElement;
    act(() => {
      trigger.click();
    });
    await flush();
    // Let the fetch resolve and component rerender.
    await flush();

    // Session info rendered in portal — scan body.
    expect(document.body.textContent).toContain("tmx-import");
    expect(document.body.textContent).toContain("1.0.0");
    expect(document.body.textContent).toContain("42");
    expect(getImportSession).toHaveBeenCalledWith("sess-1");

    // Re-open should not fetch again (cache hit).
    act(() => {
      trigger.click();
    }); // close
    await flush();
    act(() => {
      trigger.click();
    }); // open
    await flush();
    expect(getImportSession).toHaveBeenCalledTimes(1);
  });
});

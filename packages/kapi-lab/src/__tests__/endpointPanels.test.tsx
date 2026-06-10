// @vitest-environment jsdom
import { createElement, act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { SourceContentPanel, SinkOutputPanel } from "../EndpointPanels";
import type { LabRuntime } from "../useLabRuntime";
import { jsonTree } from "../stories/previewFixtures";

// A minimal fake LabRuntime: ready, with inspect resolving the JSON fixture
// tree. Only the members the panels touch are real.
function fakeRuntime(overrides: Partial<LabRuntime> = {}): LabRuntime {
  return {
    status: "ready",
    error: null,
    ready: true,
    bootProgress: null,
    mkdir: vi.fn(),
    writeFile: vi.fn((f: string) => `/project/${f}`),
    inspect: vi.fn(async () => ({ ok: true, format: "json", tree: jsonTree })),
    inspectAnnotated: vi.fn(async () => ({ ok: true, format: "json", tree: jsonTree })),
    trace: vi.fn(async () => ({ ok: false, error: "unused" })),
    run: vi.fn(async () => 0),
    runCapture: vi.fn(async () => ({ code: 0, output: "" })),
    readFile: vi.fn(() => null),
    readBytes: vi.fn(() => null),
    klf: vi.fn(() => ({ ok: false, error: "unused" })),
    segment: vi.fn(() => ({ ok: false, error: "unused" })),
    segmentEngines: vi.fn(() => []),
    ...overrides,
  } as LabRuntime;
}

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  container = document.createElement("div");
  document.body.appendChild(container);
  act(() => {
    root = createRoot(container);
  });
});

afterEach(() => {
  act(() => root.unmount());
  container.remove();
});

const flush = async () => {
  // Let the inspect promise resolve and React commit.
  await act(async () => {
    await Promise.resolve();
  });
};

describe("SourceContentPanel", () => {
  it("inspects the file and renders its content tree", async () => {
    const runtime = fakeRuntime();
    act(() => {
      root.render(
        createElement(SourceContentPanel, {
          runtime,
          file: { filename: "messages.json", label: "Messages", content: '{"a":"b"}' },
        }),
      );
    });
    await flush();
    expect(runtime.inspect).toHaveBeenCalledWith("messages.json", '{"a":"b"}');
    expect(container.textContent).toContain("messages.json");
    // The explainer ties the tree to the pipeline.
    expect(container.textContent).toContain("what flows into the first tool");
  });

  it("prefers raw bytes when the file carries them", async () => {
    const runtime = fakeRuntime();
    const bytes = new Uint8Array([1, 2, 3]);
    act(() => {
      root.render(
        createElement(SourceContentPanel, {
          runtime,
          file: { filename: "doc.docx", label: "Doc", content: "", bytes },
        }),
      );
    });
    await flush();
    expect(runtime.inspect).toHaveBeenCalledWith("doc.docx", bytes);
  });

  it("surfaces reader errors", async () => {
    const runtime = fakeRuntime({
      inspect: vi.fn(async () => ({ ok: false, error: "bad bytes" })),
    });
    act(() => {
      root.render(
        createElement(SourceContentPanel, {
          runtime,
          file: { filename: "x.bin", label: "X", content: "?" },
        }),
      );
    });
    await flush();
    expect(container.textContent).toContain("bad bytes");
  });
});

describe("SinkOutputPanel", () => {
  it("hints at Run before anything was written", () => {
    act(() => {
      root.render(
        createElement(SinkOutputPanel, {
          runtime: fakeRuntime(),
          outPath: null,
          version: 0,
          baseline: null,
        }),
      );
    });
    expect(container.textContent).toContain("Nothing written yet");
  });

  it("explains the round-trip and shows the output viewer once a run wrote", async () => {
    const out = new TextEncoder().encode('{"a":"B"}');
    const runtime = fakeRuntime({ readBytes: vi.fn(() => out) });
    act(() => {
      root.render(
        createElement(SinkOutputPanel, {
          runtime,
          outPath: "/project/flow-out-messages.json",
          version: 1,
          baseline: '{"a":"b"}',
        }),
      );
    });
    await flush();
    expect(container.textContent).toContain("round-trips exactly");
    expect(runtime.readBytes).toHaveBeenCalledWith("/project/flow-out-messages.json");
  });
});

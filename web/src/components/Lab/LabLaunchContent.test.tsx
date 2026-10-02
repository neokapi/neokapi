// @vitest-environment jsdom
import React from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { LabLaunch } from "./LabLaunch";
import ContentLab from "../../../../packages/kapi-lab/src/ContentLab";

const { bootEngine, engineState } = vi.hoisted(() => ({
  bootEngine: vi.fn<() => Promise<unknown>>(() => new Promise(() => {})),
  engineState: { phase: "idle" },
}));
vi.mock("@neokapi/kapi-playground/runtime", () => ({ onBootProgress: () => () => {} }));
vi.mock("@neokapi/kapi-playground/plugins", () => ({
  bootEngine,
  configurePlugins: vi.fn(),
  ensurePlugin: vi.fn(),
  PLUGIN_DESCRIPTORS: [],
  usePluginManager: () => ({ state: { engine: engineState, plugins: {} } }),
}));
vi.mock("../../../../packages/kapi-lab/src/FileSelectorField", () => ({
  default: () => <div>Sample controls</div>,
}));
vi.mock("../../../../packages/kapi-lab/src/OutputView", () => ({ default: () => null }));
vi.mock("@neokapi/ui-primitives/preview", () => ({ DocumentViewer: () => null }));

afterEach(() => {
  cleanup();
  bootEngine.mockReset().mockImplementation(() => new Promise(() => {}));
  engineState.phase = "idle";
});
const assets = { wasmExecUrl: "/wasm/wasm_exec.js", wasmUrl: "/wasm/kapi-cli.wasm" };

describe("Content lab launch", () => {
  it("starts the engine with one explicit launch and no second Run gate", () => {
    render(
      <LabLaunch>
        <ContentLab assets={assets} lessonIds={["anatomy"]} autoStart />
      </LabLaunch>,
    );
    expect(bootEngine).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Open experiment" }));
    expect(bootEngine).toHaveBeenCalledOnce();
    expect(screen.queryByRole("button", { name: "Run in your browser" })).toBeNull();
    expect(screen.getByRole("status").textContent).toContain("Starting the engine");
  });

  it("keeps the embedded default gated until Run", () => {
    render(<ContentLab assets={assets} lessonIds={["anatomy"]} />);
    expect(bootEngine).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Run in your browser" }));
    expect(bootEngine).toHaveBeenCalledOnce();
  });
  it("waits for Run before reading when another lab already warmed the engine", async () => {
    engineState.phase = "ready";
    const inspect = vi.fn().mockResolvedValue({ ok: false, error: "test result" });
    bootEngine.mockResolvedValue({
      vol: { writeFile: vi.fn() },
      inspect,
      inspectAnnotated: inspect,
    });
    render(<ContentLab assets={assets} lessonIds={["anatomy"]} />);
    await waitFor(() => expect(bootEngine).toHaveBeenCalledOnce());
    await act(async () => {});
    expect(inspect).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Run in your browser" }));
    await waitFor(() => expect(inspect).toHaveBeenCalledOnce());
  });
});

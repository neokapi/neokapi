// @vitest-environment jsdom
import React, { StrictMode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import PdfExplorer from "./PdfExplorer";

vi.mock("./useLabRuntime", () => ({
  useLabRuntime: () => ({ ready: false, status: "idle", boot: vi.fn() }),
}));
vi.mock("./useRunGate", () => ({ useRunGate: () => ({ ready: false }) }));
vi.mock("./GateOverlay", () => ({ default: () => null }));
vi.mock("@neokapi/ui-primitives/preview", () => ({ DocumentViewer: () => null }));
vi.mock("./FileSelectorField", () => ({ default: () => null }));
vi.mock("./ActiveFileSwitcher", () => ({
  default: ({ files }: { files: { name: string }[] }) => (
    <div>
      {files.map((file) => (
        <p key={file.name}>{file.name}</p>
      ))}
    </div>
  ),
}));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
const samples = [{ url: "/samples/report.pdf", name: "report.pdf" }];
const response = () => ({
  ok: true,
  arrayBuffer: async () => new Uint8Array([37, 80, 68, 70]).buffer,
});

describe("PDF sample loading", () => {
  it("selects the sample after StrictMode replays effects", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockImplementation(async () => response()),
    );
    render(
      <StrictMode>
        <PdfExplorer assets={null} samples={samples} />
      </StrictMode>,
    );
    expect(await screen.findByText("report.pdf")).toBeTruthy();
    await waitFor(() => expect(screen.queryByRole("status")).toBeNull());
  });

  it("reports failed assets and retries without starting the engine", async () => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce({ ok: false, status: 404 })
      .mockImplementation(async () => response());
    vi.stubGlobal("fetch", fetch);
    render(<PdfExplorer assets={null} samples={samples} />);
    expect(await screen.findByRole("alert")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Retry sample download" }));
    expect(await screen.findByText("report.pdf")).toBeTruthy();
    expect(screen.queryByRole("alert")).toBeNull();
  });
});

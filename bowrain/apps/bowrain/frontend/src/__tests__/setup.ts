import "@testing-library/jest-dom";
import { vi } from "vitest";

// Polyfill ResizeObserver for @xyflow/react in jsdom.
if (typeof globalThis.ResizeObserver === "undefined") {
  globalThis.ResizeObserver = class ResizeObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
}

// Mock @wailsio/runtime to prevent network calls during tests.
vi.mock("@wailsio/runtime", () => ({
  Call: vi.fn().mockRejectedValue(new Error("not in wails")),
  CancellablePromise: vi.fn(),
  Create: vi.fn(),
  Events: {
    On: vi.fn(() => vi.fn()),
    Emit: vi.fn(),
  },
}));

// Mock the Wails-generated bindings used by useApi hooks.
vi.mock("../../bindings/github.com/neokapi/neokapi/bowrain/apps/bowrain/backend/app.js", () => ({
  ListFlowDefinitions: vi.fn().mockResolvedValue([]),
  GetFlowDefinition: vi.fn(),
  SaveFlowDefinition: vi.fn(),
  DeleteFlowDefinition: vi.fn(),
  ListTools: vi.fn().mockResolvedValue([]),
  GetToolSchema: vi.fn().mockResolvedValue(null),
  ListFormats: vi.fn().mockResolvedValue([]),
  ListPlugins: vi.fn().mockResolvedValue([]),
  PluginDir: vi.fn().mockResolvedValue(""),
  GetVersion: vi.fn().mockResolvedValue({ version: "dev", commit: "", build_date: "" }),
}));

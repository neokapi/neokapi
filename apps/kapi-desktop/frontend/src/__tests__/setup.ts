import "@testing-library/jest-dom";

// Polyfill ResizeObserver for @xyflow/react in jsdom.
if (typeof globalThis.ResizeObserver === "undefined") {
  globalThis.ResizeObserver = class ResizeObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
}
import { vi } from "vitest";

// Mock @wailsio/runtime to prevent network calls during tests.
// The Wails runtime tries to connect to localhost when imported.
vi.mock("@wailsio/runtime", () => ({
  Call: vi.fn().mockRejectedValue(new Error("not in wails")),
  CancellablePromise: vi.fn(),
  Create: vi.fn(),
  Events: {
    On: vi.fn(() => vi.fn()),
    Emit: vi.fn(),
  },
}));

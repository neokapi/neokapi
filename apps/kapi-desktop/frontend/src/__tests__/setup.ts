import "@testing-library/jest-dom";

// Polyfill ResizeObserver, which several UI primitives observe, for jsdom.
if (typeof globalThis.ResizeObserver === "undefined") {
  globalThis.ResizeObserver = class ResizeObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
}
// Radix's popper-backed controls (Select, DropdownMenu) call these while
// opening. jsdom implements neither, so without them a click on a trigger
// throws and the menu never renders.
if (typeof Element !== "undefined") {
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.setPointerCapture ??= () => {};
  Element.prototype.releasePointerCapture ??= () => {};
  Element.prototype.scrollIntoView ??= () => {};
}

import { vi } from "vitest";

// Mock @wailsio/runtime to prevent network calls during tests.
// The Wails runtime tries to connect to localhost when imported.
vi.mock("@wailsio/runtime", () => ({
  Call: vi.fn().mockRejectedValue(new Error("not in wails")),
  CancellablePromise: vi.fn(),
  Create: vi.fn(),
  Clipboard: {
    SetText: vi.fn().mockResolvedValue(undefined),
    Text: vi.fn().mockResolvedValue(""),
  },
  Events: {
    On: vi.fn(() => vi.fn()),
    Emit: vi.fn(),
  },
}));

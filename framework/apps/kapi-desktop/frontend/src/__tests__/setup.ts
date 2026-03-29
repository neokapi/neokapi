import "@testing-library/jest-dom";
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

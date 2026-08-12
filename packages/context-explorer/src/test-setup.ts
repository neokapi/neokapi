import "@testing-library/jest-dom/vitest";
import { afterEach } from "vitest";
import { cleanup } from "@testing-library/react";

// JSDOM doesn't implement ResizeObserver, which the primitives these views build
// on construct on mount. Without a stub the constructor throws asynchronously —
// an unhandled error that intermittently fails whichever case is in flight.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver = ResizeObserverStub as unknown as typeof ResizeObserver;

// Component tests render into jsdom; unmount between cases to avoid DOM bleed.
afterEach(cleanup);

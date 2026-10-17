import "@testing-library/jest-dom/vitest";
import { afterEach } from "vite-plus/test";
import { cleanup } from "@testing-library/react";

// JSDOM has no ResizeObserver, which the views and the primitives construct on
// mount. Without a stub the constructor throws asynchronously, an unhandled
// error that fails whichever case is in flight.
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver = ResizeObserverStub as unknown as typeof ResizeObserver;

// Component tests render into jsdom; unmount between cases to avoid DOM bleed.
afterEach(cleanup);

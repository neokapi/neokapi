import "@testing-library/jest-dom/vitest";
import { afterEach } from "vite-plus/test";
import { cleanup } from "@testing-library/react";

// Component tests render into jsdom; unmount between cases to avoid DOM bleed.
afterEach(cleanup);

// The list-and-detail primitives observe their size, which jsdom cannot.
if (typeof globalThis.ResizeObserver === "undefined") {
  globalThis.ResizeObserver = class ResizeObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
}

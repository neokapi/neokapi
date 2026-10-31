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

// The pickers open a Radix menu, which captures the pointer and scrolls the
// picked item into view; jsdom implements neither. Each stand-in answers
// nothing, which reads as no pointer captured.
if (typeof Element !== "undefined") {
  const proto = Element.prototype as unknown as Record<string, unknown>;
  for (const name of [
    "hasPointerCapture",
    "setPointerCapture",
    "releasePointerCapture",
    "scrollIntoView",
  ]) {
    if (typeof proto[name] !== "function") proto[name] = () => undefined;
  }
}

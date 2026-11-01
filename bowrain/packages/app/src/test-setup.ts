import * as matchers from "@testing-library/jest-dom/matchers";
import { cleanup } from "@testing-library/react";
import { afterEach, expect } from "vite-plus/test";

expect.extend(matchers);

// JSDOM doesn't implement matchMedia — provide a minimal stub.
Object.defineProperty(window, "matchMedia", {
  writable: true,
  value: (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  }),
});

// JSDOM doesn't implement ResizeObserver either; Radix's ScrollArea observes
// its viewport on mount.
if (typeof globalThis.ResizeObserver === "undefined") {
  globalThis.ResizeObserver = class ResizeObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
}

// A Radix picker captures the pointer and scrolls the picked item into view;
// JSDOM implements neither. Each stand-in answers nothing, which reads as no
// pointer captured.
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

afterEach(() => {
  cleanup();
  localStorage.clear();
});

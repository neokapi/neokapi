import * as matchers from "@testing-library/jest-dom/matchers";
import { cleanup } from "@testing-library/react";
import { afterEach, expect } from "vite-plus/test";

expect.extend(matchers);

// JSDOM doesn't implement ResizeObserver — provide a minimal stub (needed by cmdk).
class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
globalThis.ResizeObserver = ResizeObserverStub as unknown as typeof ResizeObserver;

// JSDOM doesn't implement scrollIntoView — provide a no-op (needed by cmdk).
Element.prototype.scrollIntoView = function () {};

// JSDOM implements no pointer capture, which Radix's Select trigger asks about
// on every pointer-down. Without these a click on a Select throws before the
// listbox opens, so a test driving one fails on the environment rather than on
// the component.
Element.prototype.hasPointerCapture = function () {
  return false;
};
Element.prototype.setPointerCapture = function () {};
Element.prototype.releasePointerCapture = function () {};

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

afterEach(() => {
  cleanup();
});

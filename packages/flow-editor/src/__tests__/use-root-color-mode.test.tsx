// @vitest-environment jsdom
import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vite-plus/test";
import { useRootColorMode } from "../useRootColorMode";

/**
 * React Flow 13 follows the system colour scheme unless forceColorMode says
 * otherwise. The editor passes the app's own theme, the `dark` class on the
 * root element, so a light app on a dark-mode machine keeps a light canvas.
 */
describe("useRootColorMode", () => {
  afterEach(() => {
    document.documentElement.classList.remove("dark");
  });

  it("reads light when the root has no dark class", () => {
    const { result } = renderHook(() => useRootColorMode());
    expect(result.current).toBe("light");
  });

  it("follows the dark class as the app toggles it", async () => {
    const { result } = renderHook(() => useRootColorMode());
    await act(async () => {
      document.documentElement.classList.add("dark");
      await Promise.resolve();
    });
    expect(result.current).toBe("dark");
    await act(async () => {
      document.documentElement.classList.remove("dark");
      await Promise.resolve();
    });
    expect(result.current).toBe("light");
  });
});

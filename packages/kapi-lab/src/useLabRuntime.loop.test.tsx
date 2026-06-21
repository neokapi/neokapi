// @vitest-environment jsdom
import { afterEach, expect, test, vi } from "vitest";
import { cleanup, render, waitFor } from "@testing-library/react";
import React, { useRef } from "react";

// Mock the boot path so the hook runs without loading a real 70MB binary. Boot
// is routed through the plugin manager (bootEngine), so mock that; onBootProgress
// is still subscribed directly from the runtime subpath.
vi.mock("@neokapi/kapi-playground/runtime", () => ({
  onBootProgress: vi.fn(() => () => {}),
}));
vi.mock("@neokapi/kapi-playground/plugins", () => ({
  configurePlugins: vi.fn(),
  bootEngine: vi.fn(async () => ({}) as never),
}));

import { useLabRuntime } from "./useLabRuntime";

afterEach(cleanup);

// Regression: a host that returns a FRESH config object every render (the real
// useKapiPlaygroundConfig bug) must NOT send the boot effect into an infinite
// ready↔booting loop. The hook keys its effect on the URL strings, so the effect
// settles after one boot regardless of the object identity churning each render.
test("does not loop when the assets object identity changes every render", async () => {
  let renders = 0;
  function Harness(): React.ReactElement {
    renders++;
    // A new object literal every render — exactly what the unmemoized adapter did.
    // autoBoot:true so the hook boots on mount (no Run gate in this hook-level test).
    const rt = useLabRuntime(
      { wasmExecUrl: "/x/exec.js", wasmUrl: "/x/k.wasm" },
      { autoBoot: true },
    );
    // Force a re-render once ready to make any ready→booting flip observable.
    const bumped = useRef(false);
    if (rt.ready && !bumped.current) {
      bumped.current = true;
    }
    return <span data-status={rt.status}>{rt.status}</span>;
  }

  const { container } = render(<Harness />);
  await waitFor(() => expect(container.querySelector("span")?.dataset.status).toBe("ready"));

  // Once ready, give React a few ticks; renders must not run away.
  const settled = renders;
  await new Promise((r) => setTimeout(r, 50));
  expect(renders - settled).toBeLessThan(5);
  expect(container.querySelector("span")?.dataset.status).toBe("ready");
});

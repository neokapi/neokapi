// @vitest-environment jsdom
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useToolSchemas } from "../useToolSchemas";
import type { ComponentSchema } from "../types";

const SCHEMA: ComponentSchema = {
  title: "Terminology Check",
  type: "object",
  properties: {
    caseSensitive: { type: "boolean", title: "Case Sensitive" },
  },
};

afterEach(cleanup);

describe("useToolSchemas", () => {
  it("answers null while a schema loads, then the schema", async () => {
    const fetchSchema = vi.fn((name: string) =>
      Promise.resolve(name === "term-check" ? SCHEMA : null),
    );
    const { result } = renderHook(() => useToolSchemas(fetchSchema));
    expect(result.current("term-check")).toBeNull();
    await waitFor(() => expect(result.current("term-check")).toEqual(SCHEMA));
    expect(fetchSchema).toHaveBeenCalledTimes(1);
    expect(fetchSchema).toHaveBeenCalledWith("term-check");
  });

  it("asks once per tool, however many reads arrive before the answer", async () => {
    let resolve!: (schema: ComponentSchema | null) => void;
    const fetchSchema = vi.fn(
      () =>
        new Promise<ComponentSchema | null>((r) => {
          resolve = r;
        }),
    );
    const { result } = renderHook(() => useToolSchemas(fetchSchema));
    expect(result.current("term-check")).toBeNull();
    expect(result.current("term-check")).toBeNull();
    expect(fetchSchema).toHaveBeenCalledTimes(1);

    await act(async () => {
      resolve(SCHEMA);
    });
    expect(result.current("term-check")).toEqual(SCHEMA);
    expect(fetchSchema).toHaveBeenCalledTimes(1);
  });

  it("re-renders the caller when a schema arrives", async () => {
    const renders = vi.fn();
    const fetchSchema = () => Promise.resolve(SCHEMA);
    const { result } = renderHook(() => {
      renders();
      return useToolSchemas(fetchSchema);
    });
    expect(renders).toHaveBeenCalledTimes(1);
    result.current("term-check");
    await waitFor(() => expect(renders).toHaveBeenCalledTimes(2));
    expect(result.current("term-check")).toEqual(SCHEMA);
  });

  it("remembers a tool without a schema and a failed fetch as null", async () => {
    const fetchSchema = vi.fn((name: string) =>
      name === "broken" ? Promise.reject(new Error("offline")) : Promise.resolve(null),
    );
    const { result } = renderHook(() => useToolSchemas(fetchSchema));
    expect(result.current("qa")).toBeNull();
    expect(result.current("broken")).toBeNull();
    expect(fetchSchema).toHaveBeenCalledTimes(2);

    // Let both fetches settle.
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
    expect(result.current("qa")).toBeNull();
    expect(result.current("broken")).toBeNull();
    expect(fetchSchema).toHaveBeenCalledTimes(2);
  });

  it("reads through the latest fetcher without losing the cache", async () => {
    const first = vi.fn(() => Promise.resolve(SCHEMA));
    const second = vi.fn(() => Promise.resolve(null));
    const { result, rerender } = renderHook(({ fetch }) => useToolSchemas(fetch), {
      initialProps: { fetch: first },
    });
    result.current("term-check");
    await waitFor(() => expect(result.current("term-check")).toEqual(SCHEMA));

    rerender({ fetch: second });
    expect(result.current("term-check")).toEqual(SCHEMA);
    expect(result.current("qa")).toBeNull();
    await waitFor(() => expect(second).toHaveBeenCalledWith("qa"));
    expect(first).toHaveBeenCalledTimes(1);
  });
});

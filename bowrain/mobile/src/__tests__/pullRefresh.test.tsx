import { QueryClientProvider, useQuery } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react-native";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import type { ReactNode } from "react";

import { usePullRefresh } from "../lib/pullRefresh";
import { createTestQueryClient } from "./testProviders";

/** A promise the test settles by hand. */
function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

/** A list query whose every fetch waits for the test, and the pull bound to it. */
async function setup() {
  const qc = createTestQueryClient();
  const fetches: ReturnType<typeof deferred<string[]>>[] = [];
  const queryFn = () => {
    const d = deferred<string[]>();
    fetches.push(d);
    return d.promise;
  };
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
  const hook = await renderHook(
    () => {
      // A screen reads these while it renders; the query re-renders it when they change.
      const { data, isSuccess, isError, isRefetching, refetch } = useQuery({
        queryKey: ["list"],
        queryFn,
      });
      const pull = usePullRefresh(refetch);
      return { list: { data, isSuccess, isError, isRefetching }, pull };
    },
    { wrapper },
  );
  return { qc, fetches, hook };
}

async function settleFirstLoad(s: Awaited<ReturnType<typeof setup>>) {
  await act(async () => s.fetches[0].resolve(["a"]));
  await waitFor(() => expect(s.hook.result.current.list.isSuccess).toBe(true));
}

describe("usePullRefresh", () => {
  it("does not report refreshing for a background refetch", async () => {
    const s = await setup();
    expect(s.hook.result.current.pull.refreshing).toBe(false);
    await settleFirstLoad(s);

    // A refetch the query client starts on its own: focus, reconnect, a decision.
    await act(async () => void s.qc.invalidateQueries({ queryKey: ["list"] }));
    await waitFor(() => expect(s.hook.result.current.list.isRefetching).toBe(true));
    expect(s.hook.result.current.pull.refreshing).toBe(false);

    await act(async () => s.fetches[1].resolve(["a", "b"]));
    await waitFor(() => expect(s.hook.result.current.list.data).toEqual(["a", "b"]));
    expect(s.hook.result.current.pull.refreshing).toBe(false);
  });

  it("reports refreshing from a pull until the refetch settles", async () => {
    const s = await setup();
    await settleFirstLoad(s);

    await act(async () => s.hook.result.current.pull.onRefresh());
    expect(s.hook.result.current.pull.refreshing).toBe(true);
    expect(s.fetches).toHaveLength(2);

    await act(async () => s.fetches[1].resolve(["a", "b"]));
    await waitFor(() => expect(s.hook.result.current.pull.refreshing).toBe(false));
    expect(s.hook.result.current.list.data).toEqual(["a", "b"]);
  });

  it("ends the spinner when the refetch fails", async () => {
    const s = await setup();
    await settleFirstLoad(s);

    await act(async () => s.hook.result.current.pull.onRefresh());
    expect(s.hook.result.current.pull.refreshing).toBe(true);

    await act(async () => s.fetches[1].reject(new Error("offline")));
    await waitFor(() => expect(s.hook.result.current.pull.refreshing).toBe(false));
    expect(s.hook.result.current.list.isError).toBe(true);
  });

  it("ends the spinner when the refresh function rejects or throws", async () => {
    const failing = deferred<void>();
    const rejects = await renderHook(() => usePullRefresh(() => failing.promise));
    await act(async () => rejects.result.current.onRefresh());
    expect(rejects.result.current.refreshing).toBe(true);
    await act(async () => failing.reject(new Error("offline")));
    expect(rejects.result.current.refreshing).toBe(false);

    const throws = await renderHook(() =>
      usePullRefresh(() => {
        throw new Error("no client");
      }),
    );
    await act(async () => throws.result.current.onRefresh());
    expect(throws.result.current.refreshing).toBe(false);
  });

  it("holds the spinner until every pull in flight settles", async () => {
    const first = deferred<void>();
    const second = deferred<void>();
    const pending = [first, second];
    const hook = await renderHook(() => usePullRefresh(() => pending.shift()!.promise));
    await act(async () => hook.result.current.onRefresh());
    await act(async () => hook.result.current.onRefresh());
    await act(async () => first.resolve());
    expect(hook.result.current.refreshing).toBe(true);
    await act(async () => second.resolve());
    expect(hook.result.current.refreshing).toBe(false);
  });
});

/** Every .tsx file under src/, tests left out. */
function screenFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) return name === "__tests__" ? [] : screenFiles(path);
    return path.endsWith(".tsx") ? [path] : [];
  });
}

describe("refresh controls", () => {
  const src = join(__dirname, "..");
  const files = screenFiles(src).map((path) => ({
    path: relative(src, path),
    text: readFileSync(path, "utf8"),
  }));

  it("are found by the scan", () => {
    expect(files.filter((f) => /onRefresh|RefreshControl/.test(f.text)).length).toBeGreaterThan(5);
  });

  // A spinner bound to isRefetching or isFetching shows for every background
  // refetch, and on iOS it pushes the whole list down while it shows.
  it("take refreshing from usePullRefresh on every screen", () => {
    const offending = files
      .filter((f) => /onRefresh|RefreshControl/.test(f.text))
      .filter(
        (f) =>
          !f.text.includes("usePullRefresh(") ||
          /refreshing=\{(?!pull\.refreshing\})/.test(f.text) ||
          /\bisRefetching\b/.test(f.text),
      )
      .map((f) => f.path);
    expect(offending).toEqual([]);
  });
});

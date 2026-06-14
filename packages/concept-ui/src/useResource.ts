// A tiny loader for data-source reads (Apache-2.0). The ConceptDataSource methods
// may return a value synchronously (an in-memory fixture) or a promise (a REST
// or SQLite-backed source); useResource resolves either, tracks loading/error,
// and re-runs when its deps change — so the components stay framework-agnostic
// without pulling in react-query.
import { useEffect, useState } from "react";
import type { Awaitable } from "./adapter";

export interface ResourceState<T> {
  data: T | undefined;
  loading: boolean;
  error: Error | undefined;
  /** Force a re-fetch (e.g. after a mutation). */
  reload: () => void;
}

/**
 * Resolve `load()` (sync or async). `load` is invoked on mount, whenever `deps`
 * change, and on reload(). A synchronous result settles without a loading flash.
 * Out-of-order resolutions are discarded so the latest deps win.
 */
export function useResource<T>(
  load: () => Awaitable<T>,
  deps: readonly unknown[],
): ResourceState<T> {
  const [data, setData] = useState<T | undefined>(undefined);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | undefined>(undefined);
  const [nonce, setNonce] = useState(0);

  useEffect(() => {
    let live = true;
    setError(undefined);

    let result: Awaitable<T>;
    try {
      result = load();
    } catch (err) {
      setError(err instanceof Error ? err : new Error(String(err)));
      setLoading(false);
      return;
    }

    if (result instanceof Promise) {
      setLoading(true);
      result.then(
        (value) => {
          if (!live) return;
          setData(value);
          setLoading(false);
        },
        (err: unknown) => {
          if (!live) return;
          setError(err instanceof Error ? err : new Error(String(err)));
          setLoading(false);
        },
      );
    } else {
      setData(result);
      setLoading(false);
    }

    return () => {
      live = false;
    };
    // load is intentionally excluded; callers pass a stable deps array.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, nonce]);

  return { data, loading, error, reload: () => setNonce((n) => n + 1) };
}

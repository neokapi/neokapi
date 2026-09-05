import { useCallback, useEffect, useReducer, useRef } from "react";
import type { ComponentSchema } from "./types";

/**
 * A per-tool cache of option schemas behind the editor's synchronous
 * `onGetSchema`.
 *
 * The editor asks for a step's schema on every render, so the answer has to
 * be immediate. The first read of a tool starts one fetch and answers null;
 * when the fetch settles the caller re-renders and the next read answers
 * from the cache. A tool without a schema, or a fetch that fails, settles as
 * null and is not asked again for the life of the surface: a step row without
 * options is the answer, and re-asking on every render would loop.
 */
export function useToolSchemas(
  fetchSchema: (toolName: string) => Promise<ComponentSchema | null | undefined>,
): (toolName: string) => ComponentSchema | null {
  const schemas = useRef<Record<string, ComponentSchema | null>>({});
  const fetching = useRef<Set<string>>(new Set());
  const fetcher = useRef(fetchSchema);
  useEffect(() => {
    fetcher.current = fetchSchema;
  }, [fetchSchema]);
  const [, rerender] = useReducer((n: number) => n + 1, 0);

  return useCallback((toolName: string): ComponentSchema | null => {
    if (toolName in schemas.current) return schemas.current[toolName] ?? null;
    if (!fetching.current.has(toolName)) {
      fetching.current.add(toolName);
      void fetcher
        .current(toolName)
        .then(
          (schema) => schema ?? null,
          () => null,
        )
        .then((schema) => {
          fetching.current.delete(toolName);
          schemas.current[toolName] = schema;
          rerender();
        });
    }
    return null;
  }, []);
}

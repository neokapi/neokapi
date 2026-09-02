import { useCallback, useEffect, useMemo } from "react";
import { useNavigate, useRouteContext, useSearch } from "@tanstack/react-router";
import { ExplorerSection } from "@neokapi/ui";
import type { ScopeTuple } from "@neokapi/context-explorer";
import type { WorkspaceRouteContext } from "..";

/**
 * The point a link carries, mirrored in the URL so a reader can share where
 * they were standing. Only the dimensions the reader moved appear — an address
 * that always spelled every axis would be noise on the common case.
 */
export interface ExplorerSearch {
  project?: string;
  stream?: string;
  collection?: string;
  path?: string;
  coordinate?: string;
  locale?: string;
}

export function ExplorerRoute() {
  const navigate = useNavigate();
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const search = useSearch({ strict: false }) as ExplorerSearch;

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Explorer · Context · ${activeWorkspace.name} · Bowrain`;
    }
  }, [activeWorkspace]);

  const scope = useMemo<ScopeTuple>(
    () => ({
      project: search.project,
      stream: search.stream,
      collection: search.collection,
      path: search.path,
      coordinate: search.coordinate,
      locale: search.locale,
    }),
    [
      search.project,
      search.stream,
      search.collection,
      search.path,
      search.coordinate,
      search.locale,
    ],
  );

  const onScopeChange = useCallback(
    (next: ScopeTuple) => {
      void navigate({
        to: ".",
        search: {
          project: next.project,
          stream: next.stream,
          collection: next.collection,
          path: next.path,
          coordinate: next.coordinate,
          locale: next.locale,
        },
        replace: true,
      });
    },
    [navigate],
  );

  return <ExplorerSection scope={scope} onScopeChange={onScopeChange} />;
}

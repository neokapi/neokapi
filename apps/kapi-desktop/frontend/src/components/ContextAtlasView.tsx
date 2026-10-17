import { useQuery } from "@tanstack/react-query";
import { EmptyState, LoadingSpinner } from "@neokapi/ui-primitives";
import { ContextAtlas } from "@neokapi/context-atlas";
import type { AtlasData } from "@neokapi/context-atlas";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";

export interface ContextAtlasViewProps {
  tabID: string;
  /** Injected in tests and stories; production reads the Wails backend. */
  data?: AtlasData;
  /** Open a file the evidence names, in the hub's Ask section. */
  onOpenEvidence?: (path: string) => void;
}

/**
 * The Explore section: the project's rules, notes, terms and content on the
 * axes they sit on, over time. The atlas is read once per visit, the same
 * read `kapi context atlas --json` prints; every view after that is drawn from
 * it without another call, so moving the window or the columns is immediate.
 */
export function ContextAtlasView({ tabID, data, onOpenEvidence }: ContextAtlasViewProps) {
  const atlas = useQuery({
    queryKey: qk.projectContextAtlas(tabID),
    queryFn: () => api.projectContextAtlas(tabID),
    enabled: data === undefined,
  });
  const resolved = data ?? atlas.data ?? undefined;
  if (!resolved) {
    if (atlas.isError) {
      return (
        <EmptyState
          title="The atlas could not be read"
          description={String(atlas.error)}
          className="m-6"
        />
      );
    }
    return <LoadingSpinner />;
  }
  return (
    <ContextAtlas
      data={resolved}
      onOpenEvidence={onOpenEvidence}
      className="h-full overflow-auto px-6 py-5"
    />
  );
}

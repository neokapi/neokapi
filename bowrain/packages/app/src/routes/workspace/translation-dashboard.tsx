import { useEffect, useState } from "react";
import { useParams, useRouteContext } from "@tanstack/react-router";
import { keepPreviousData, useQuery, useSuspenseQuery } from "@tanstack/react-query";
import {
  TranslationDashboard,
  TranslationDashboardSkeleton,
  useApi,
  useStream,
  type DashboardItemSort,
  type FileProgressPaging,
} from "@neokapi/ui";
import {
  DASHBOARD_ITEM_PAGE_SIZE,
  projectQueryOptions,
  translationDashboardQueryOptions,
} from "../../queries";
import type { WorkspaceRouteContext } from "..";

export function TranslationDashboardRoute() {
  const { projectId } = useParams({ strict: false });
  const adapter = useApi();
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const ws = activeWorkspace.slug;
  const { activeStream } = useStream();

  // Server-side item paging: the table's sort is pushed down to the endpoint
  // and "Load more" grows the page. The route loader primes the default
  // (first page, name asc) so the initial render is a cache hit.
  const [itemSort, setItemSort] = useState<{ field: DashboardItemSort; dir: "asc" | "desc" }>({
    field: "name",
    dir: "asc",
  });
  const [pageCount, setPageCount] = useState(1);

  const { data: project } = useSuspenseQuery(
    projectQueryOptions(adapter, ws, projectId!, activeStream),
  );
  const {
    data: stats,
    isPending,
    isFetching,
  } = useQuery({
    ...translationDashboardQueryOptions(adapter, ws, projectId!, activeStream, {
      itemLimit: pageCount * DASHBOARD_ITEM_PAGE_SIZE,
      itemSort: itemSort.field,
      itemDir: itemSort.dir,
    }),
    // Keep the previous page on screen while a sort change / load-more fetches.
    placeholderData: keepPreviousData,
  });

  useEffect(() => {
    document.title = `Dashboard — ${project.name} — ${activeWorkspace.name} — Bowrain`;
  }, [project.name, activeWorkspace.name]);

  if (isPending || !stats) {
    return <TranslationDashboardSkeleton />;
  }

  const itemTotal = stats.item_total ?? stats.item_stats.length;
  const itemsPaging: FileProgressPaging = {
    total: itemTotal,
    sortField: itemSort.field,
    sortDir: itemSort.dir,
    onSortChange: (field, dir) => {
      setItemSort({ field, dir });
      setPageCount(1);
    },
    hasMore: stats.item_stats.length < itemTotal,
    onLoadMore: () => setPageCount((c) => c + 1),
    isLoading: isFetching,
  };

  return (
    <div className="mx-auto max-w-6xl p-6">
      <TranslationDashboard stats={stats} projectName={project.name} itemsPaging={itemsPaging} />
    </div>
  );
}

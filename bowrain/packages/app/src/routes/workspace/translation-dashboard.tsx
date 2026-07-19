import { useEffect, useState } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { keepPreviousData, useQueries, useQuery, useSuspenseQuery } from "@tanstack/react-query";
import {
  DeliveryPanel,
  TranslationDashboard,
  TranslationDashboardSkeleton,
  useApi,
  useStream,
  type DashboardItemSort,
  type DeliveryConnectorStatus,
  type FileProgressPaging,
} from "@neokapi/ui";
import {
  DASHBOARD_ITEM_PAGE_SIZE,
  projectQueryOptions,
  translationDashboardQueryOptions,
} from "../../queries";
import type { WorkspaceRouteContext } from "..";

export function TranslationDashboardRoute() {
  const navigate = useNavigate();
  const { workspace, projectId } = useParams({ strict: false });
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

  // Delivery panel data: the workspace's connectors and each one's last
  // delivery (same query keys as the connectors panel, so the caches align).
  const { data: connectors } = useQuery({
    queryKey: ["connectors", ws],
    queryFn: () => adapter.listConnectors(ws),
    staleTime: 15_000,
  });
  const statusQueries = useQueries({
    queries: (connectors ?? []).map((c) => ({
      queryKey: ["connector-status", ws, c.id],
      queryFn: () => adapter.getConnectorStatus(ws, c.id),
      staleTime: 30_000,
      retry: false,
    })),
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

  const deliveryConnectors: DeliveryConnectorStatus[] | undefined = connectors?.map((c, i) => {
    const status = statusQueries[i]?.data;
    // The status endpoint appends the connector's stored last sync failure as
    // the final entry of `errors` (cleared on the next successful sync).
    const errors = status?.errors ?? [];
    return {
      id: c.id,
      name: c.name || c.id,
      lastSync: status?.lastSync,
      lastError: errors.length > 0 ? errors[errors.length - 1] : undefined,
    };
  });

  const routeParams = {
    workspace: workspace ?? ws,
    projectId: project.id,
    stream: activeStream,
  };

  const delivery = (
    <DeliveryPanel
      localeStats={stats.locale_stats}
      connectors={deliveryConnectors}
      onOpenConnectors={() =>
        navigate({ to: "/$workspace/p/$projectId/s/$stream/connectors", params: routeParams })
      }
      onOpenReview={() =>
        navigate({ to: "/$workspace/p/$projectId/s/$stream/review", params: routeParams })
      }
    />
  );

  return (
    <div className="mx-auto max-w-6xl p-6">
      <TranslationDashboard
        stats={stats}
        projectName={project.name}
        itemsPaging={itemsPaging}
        delivery={delivery}
      />
    </div>
  );
}

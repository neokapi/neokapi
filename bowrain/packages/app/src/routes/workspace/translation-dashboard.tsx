import { useEffect, useState } from "react";
import { useNavigate, useParams, useRouteContext, useSearch } from "@tanstack/react-router";
import { keepPreviousData, useQuery, useSuspenseQuery } from "@tanstack/react-query";
import {
  CollectionItemsView,
  DeliveryPanel,
  TranslationDashboard,
  TranslationDashboardSkeleton,
  useApi,
  useBreadcrumbExtra,
  useStream,
  fetchConnectorStatuses,
  type ApiAdapter,
  type CollectionScope,
  type DashboardItemSort,
  type DeliveryConnectorStatus,
  type FileProgressPaging,
  type ItemPreviewBinding,
} from "@neokapi/ui";
import {
  DASHBOARD_ITEM_PAGE_SIZE,
  projectQueryOptions,
  translationDashboardQueryOptions,
} from "../../queries";
import type { WorkspaceRouteContext } from "..";

/**
 * The overview's URL state. Absent, the route renders the collection overview;
 * present, it renders that scope's item list.
 *
 * `collection` and `ungrouped` mirror the endpoint's own parameters — the
 * collection-less bucket needs a flag of its own because its collection id is
 * the empty string. `items=all` is the collection-agnostic list, kept reachable
 * so the grouping hides nothing.
 */
export interface DashboardSearch {
  collection?: string;
  ungrouped?: boolean;
  items?: "all";
  /** The coordinate axis the overview groups by, when the reader picks one. */
  axis?: string;
  /** The item whose preview is open, by name. */
  preview?: string;
}

/** Which scope the search names, or undefined for the overview. */
function itemScope(search: DashboardSearch): DashboardSearch | undefined {
  if (search.collection) return { collection: search.collection };
  if (search.ungrouped) return { ungrouped: true };
  if (search.items === "all") return { items: "all" };
  return undefined;
}

/**
 * One collection's items (or, unscoped, the project's whole list). Mounted
 * under a key derived from the scope, so switching collections starts a fresh
 * page rather than carrying the previous collection's rows across as
 * placeholder data; within a scope, sorting and "Load more" keep the visible
 * page while the next one arrives.
 */
function ScopedItems({
  adapter,
  workspaceSlug,
  projectId,
  stream,
  scope,
  onBack,
  onReview,
  preview,
}: {
  adapter: ApiAdapter;
  workspaceSlug: string;
  projectId: string;
  stream: string;
  scope: DashboardSearch;
  onBack: () => void;
  onReview: (scope?: CollectionScope) => void;
  preview: ItemPreviewBinding & { projectId: string };
}) {
  const [itemSort, setItemSort] = useState<{ field: DashboardItemSort; dir: "asc" | "desc" }>({
    field: "name",
    dir: "asc",
  });
  const [pageCount, setPageCount] = useState(1);

  const { data: stats, isFetching } = useQuery({
    ...translationDashboardQueryOptions(adapter, workspaceSlug, projectId, stream, {
      itemCollection: scope.collection,
      itemUngrouped: scope.ungrouped,
      itemLimit: pageCount * DASHBOARD_ITEM_PAGE_SIZE,
      itemSort: itemSort.field,
      itemDir: itemSort.dir,
    }),
    placeholderData: keepPreviousData,
  });

  if (!stats) return <TranslationDashboardSkeleton />;

  const itemTotal = stats.item_total ?? stats.item_stats.length;
  const paging: FileProgressPaging = {
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

  const all = scope.items === "all";
  const rollup = all
    ? undefined
    : scope.ungrouped
      ? stats.collection_stats.find((c) => c.ungrouped)
      : stats.collection_stats.find((c) => c.collection_id === scope.collection);
  const title = all
    ? "All items"
    : scope.ungrouped
      ? "In no collection"
      : (rollup?.collection_name ?? scope.collection ?? "");

  return (
    <>
      {/* The shell's trail ends at the project; only this route knows what the
          `?collection=` id is called, so it appends that step itself. */}
      <ScopedBreadcrumb title={title} />
      <CollectionItemsView
        collection={rollup}
        title={title}
        itemStats={stats.item_stats}
        localeStats={stats.locale_stats}
        itemBase={stats.item_base}
        paging={paging}
        onBack={onBack}
        onReview={
          all
            ? undefined
            : () =>
                onReview({
                  collectionId: scope.collection ?? "",
                  ungrouped: Boolean(scope.ungrouped),
                })
        }
        preview={preview}
      />
    </>
  );
}

/** Appends one step to the shell's trail and renders nothing else. */
function ScopedBreadcrumb({ title }: { title: string }) {
  useBreadcrumbExtra([{ label: title }]);
  return null;
}

export function TranslationDashboardRoute() {
  const navigate = useNavigate();
  const { workspace, projectId } = useParams({ strict: false });
  const search = useSearch({ strict: false }) as DashboardSearch;
  const adapter = useApi();
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const ws = activeWorkspace.slug;
  const { activeStream } = useStream();

  const scope = itemScope(search);

  const { data: project } = useSuspenseQuery(
    projectQueryOptions(adapter, ws, projectId!, activeStream),
  );

  // The overview reads the loader-primed default page — the same cache entry
  // the review session holds. Collection rollups are complete on every
  // response, and the overview renders no item rows of its own, so drilling in
  // is what pays for an item list: its own scoped read, in which the table's
  // "N of M" counts the collection rather than a page of the project.
  const { data: stats, isPending } = useQuery({
    ...translationDashboardQueryOptions(adapter, ws, projectId!, activeStream),
    enabled: !scope,
  });

  // Delivery is an overview band; a drill-down neither shows nor fetches it.
  const { data: connectors } = useQuery({
    queryKey: ["connectors", ws],
    queryFn: () => adapter.listConnectors(ws),
    staleTime: 15_000,
    enabled: !scope,
  });
  // One batch read for the panel: a request per connector turned a workspace
  // with many delivery targets into a fan-out on every dashboard visit. The
  // cheap (unprobed) read is right here — this surface polls.
  const connectorIds = (connectors ?? []).map((c) => c.id);
  const { data: connectorStatuses } = useQuery({
    queryKey: ["connector-status", ws, "batch", connectorIds.join(",")],
    queryFn: () => fetchConnectorStatuses(adapter, ws, connectorIds),
    enabled: connectorIds.length > 0,
    staleTime: 30_000,
    retry: false,
  });

  useEffect(() => {
    document.title = `Overview — ${project.name} — ${activeWorkspace.name} — Bowrain`;
  }, [project.name, activeWorkspace.name]);

  const routeParams = {
    workspace: workspace ?? ws,
    projectId: project.id,
    stream: activeStream,
  };
  // The overview is the stream root: a scope change is a search-param change on
  // the address the project already sits at.
  const dashboardRoute = "/$workspace/p/$projectId/s/$stream" as const;

  const openScope = (next: DashboardSearch) =>
    void navigate({
      to: dashboardRoute,
      params: routeParams,
      search: (prev: DashboardSearch) => ({ axis: prev.axis, ...next }),
    });

  // Review carries the collection it was entered from, so the queue opens on
  // the scope the reader was looking at rather than the whole project.
  const openReview = (reviewScope?: CollectionScope) =>
    void navigate({
      to: "/$workspace/p/$projectId/s/$stream/review",
      params: routeParams,
      search: reviewScope
        ? {
            collection: reviewScope.ungrouped ? undefined : reviewScope.collectionId,
            ungrouped: reviewScope.ungrouped || undefined,
          }
        : {},
    });

  // Opening and closing the preview is a search-param change on the address the
  // drill-down already sits at, so the reading deep-links and Back closes it.
  const setPreview = (item: string | undefined) =>
    void navigate({
      to: dashboardRoute,
      params: routeParams,
      search: (prev: DashboardSearch) => ({ ...prev, preview: item }),
    });

  const previewBinding: ItemPreviewBinding & { projectId: string } = {
    projectId: project.id,
    itemName: search.preview ?? null,
    onOpen: (itemName) => setPreview(itemName),
    onClose: () => setPreview(undefined),
    targetLocales: project.target_languages,
    sourceLocale: project.default_source_language,
    onOpenTranslate: (itemName) =>
      void navigate({
        to: "/$workspace/p/$projectId/s/$stream/translate/$",
        params: { ...routeParams, _splat: itemName },
      }),
    onOpenReview: (itemName) =>
      void navigate({
        to: "/$workspace/p/$projectId/s/$stream/review/$",
        params: { ...routeParams, _splat: itemName },
      }),
    onOpenPreProcess: (itemName) =>
      void navigate({
        to: "/$workspace/p/$projectId/s/$stream/pre-process/$",
        params: { ...routeParams, _splat: itemName },
      }),
  };

  if (scope) {
    return (
      <div className="mx-auto max-w-6xl p-6">
        <ScopedItems
          key={scope.collection ?? (scope.ungrouped ? "~ungrouped" : "~all")}
          adapter={adapter}
          workspaceSlug={ws}
          projectId={project.id}
          stream={activeStream}
          scope={scope}
          onBack={() =>
            void navigate({
              to: dashboardRoute,
              params: routeParams,
              search: (prev: DashboardSearch) => ({ axis: prev.axis }),
            })
          }
          onReview={openReview}
          preview={previewBinding}
        />
      </div>
    );
  }

  if (isPending || !stats) {
    return <TranslationDashboardSkeleton />;
  }

  const deliveryConnectors: DeliveryConnectorStatus[] | undefined = connectors?.map((c) => {
    const status = connectorStatuses?.[c.id];
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

  const delivery = (
    <DeliveryPanel
      localeStats={stats.locale_stats}
      connectors={deliveryConnectors}
      onOpenConnectors={() =>
        navigate({ to: "/$workspace/p/$projectId/s/$stream/connectors", params: routeParams })
      }
      onOpenReview={() => openReview()}
    />
  );

  return (
    <div className="mx-auto max-w-6xl p-6">
      <TranslationDashboard
        stats={stats}
        projectName={project.name}
        delivery={delivery}
        groupingAxis={search.axis}
        onGroupingAxisChange={(axis) =>
          void navigate({
            to: dashboardRoute,
            params: routeParams,
            search: { axis },
            replace: true,
          })
        }
        onOpenCollection={(collectionScope) =>
          openScope(
            collectionScope.ungrouped
              ? { ungrouped: true }
              : { collection: collectionScope.collectionId },
          )
        }
        onOpenAllItems={() => openScope({ items: "all" })}
        onReviewCollection={openReview}
        onAddContent={() =>
          void navigate({ to: "/$workspace/p/$projectId/s/$stream/source", params: routeParams })
        }
      />
    </div>
  );
}

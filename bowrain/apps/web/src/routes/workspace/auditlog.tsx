import { useEffect, useState, useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { AuditLogView, useWorkspace, useApi, Card } from "@neokapi/ui";
import type { AuditEntry, AuditQuery, AuditChainVerification, FilterToken } from "@neokapi/ui";
import { projectsQueryOptions } from "../../queries";

export function AuditLogRoute() {
  const { activeWorkspace } = useWorkspace();
  const adapter = useApi();
  const ws = activeWorkspace?.slug ?? "";

  const [filters, setFilters] = useState<FilterToken[]>([]);
  const [search, setSearch] = useState("");
  const [allEntries, setAllEntries] = useState<AuditEntry[]>([]);
  const [hasMore, setHasMore] = useState(false);
  const [offset, setOffset] = useState(0);
  const LIMIT = 50;

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Audit Log — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  const { data: projects } = useQuery({
    ...projectsQueryOptions(adapter, ws),
    enabled: !!ws,
  });

  // Build API query from filter tokens
  const apiQuery: AuditQuery = {
    project: filters.find((f) => f.key === "project")?.value,
    type: filters.find((f) => f.key === "type")?.value,
    actor: filters.find((f) => f.key === "actor")?.value,
    search: search || undefined,
    limit: LIMIT,
    offset,
  };

  const { data, isFetching } = useQuery({
    queryKey: ["auditlog", ws, apiQuery],
    queryFn: () => adapter.listWorkspaceAuditLog(ws, apiQuery),
    enabled: !!ws,
    staleTime: 10_000,
  });

  useEffect(() => {
    if (data) {
      if (offset === 0) {
        setAllEntries(data);
      } else {
        setAllEntries((prev) => [...prev, ...data]);
      }
      setHasMore(data.length === LIMIT);
    }
  }, [data, offset]);

  // Reset offset when filters change
  const handleFiltersChange = useCallback((newFilters: FilterToken[]) => {
    setFilters(newFilters);
    setOffset(0);
  }, []);

  const handleSearchChange = useCallback((newSearch: string) => {
    setSearch(newSearch);
    setOffset(0);
  }, []);

  const handleLoadMore = useCallback(() => {
    setOffset((o) => o + LIMIT);
  }, []);

  const [verification, setVerification] = useState<AuditChainVerification | null>(null);
  const [verifying, setVerifying] = useState(false);
  const handleVerify = useCallback(async () => {
    if (!ws) return;
    setVerifying(true);
    try {
      setVerification(await adapter.verifyWorkspaceAuditChain(ws));
    } finally {
      setVerifying(false);
    }
  }, [adapter, ws]);

  if (!activeWorkspace) {
    return (
      <Card className="mt-8 max-w-md mx-auto p-8 text-center text-muted-foreground text-sm">
        Select a workspace
      </Card>
    );
  }

  return (
    <div className="mx-auto w-full max-w-5xl p-4 md:p-6">
      <AuditLogView
        entries={allEntries}
        projects={projects}
        loading={isFetching}
        hasMore={hasMore}
        onLoadMore={handleLoadMore}
        onFiltersChange={handleFiltersChange}
        onSearchChange={handleSearchChange}
        activeFilters={filters}
        activeSearch={search}
        verification={verification}
        onVerify={handleVerify}
        verifying={verifying}
      />
    </div>
  );
}

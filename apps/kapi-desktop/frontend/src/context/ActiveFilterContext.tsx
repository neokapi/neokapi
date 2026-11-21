import { createContext, useContext, useCallback } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import type { ProjectFilter, ProjectFilters } from "../types/api";

interface ActiveFilterValue {
  /** All saved filters for the active project (shared + personal). */
  filters: ProjectFilter[];
  /** The active filter id; "" means no filter ("All"). */
  activeId: string;
  /** The resolved active filter object, or null for "All". */
  active: ProjectFilter | null;
  /** True when a saved project is in scope (filters only apply in project mode). */
  enabled: boolean;
  setActive: (id: string) => Promise<void>;
  saveFilter: (f: ProjectFilter) => Promise<ProjectFilter | null>;
  deleteFilter: (id: string) => Promise<void>;
  reload: () => Promise<void>;
}

const noop = async () => {};

const ActiveFilterContext = createContext<ActiveFilterValue>({
  filters: [],
  activeId: "",
  active: null,
  enabled: false,
  setActive: noop,
  saveFilter: async () => null,
  deleteFilter: noop,
  reload: noop,
});

/**
 * Provides the active project tab's saved filters and active selection to the
 * whole app, so the menu-bar control and every view share one filter. Scoped to
 * the active tab: switching tabs reloads that project's filters.
 */
export function ActiveFilterProvider({
  tabID,
  enabled,
  children,
}: {
  tabID: string | null;
  enabled: boolean;
  children: React.ReactNode;
}) {
  const qc = useQueryClient();

  const filtersQuery = useQuery({
    queryKey: qk.projectFilters(tabID ?? ""),
    queryFn: () => api.getProjectFilters(tabID!),
    enabled: !!tabID && enabled,
  });
  const filters: ProjectFilter[] = tabID && enabled ? (filtersQuery.data?.filters ?? []) : [];
  const serverActiveId = tabID && enabled ? (filtersQuery.data?.active ?? "") : "";

  const selection = useMutation({
    scope: { id: `active-filter:${tabID}` },
    mutationFn: ({ tab, id }: { tab: string; id: string }) => api.setActiveFilter(tab, id),
    onSuccess: async (_, { tab, id }) => {
      const queryKey = qk.projectFilters(tab);
      await qc.cancelQueries({ queryKey });
      qc.setQueryData<ProjectFilters>(queryKey, (previous) =>
        previous ? { ...previous, active: id } : previous,
      );
      await qc.invalidateQueries({ queryKey });
    },
  });
  const activeId =
    enabled && selection.isPending && selection.variables.tab === tabID
      ? selection.variables.id
      : serverActiveId;

  const reload = useCallback(async () => {
    await qc.invalidateQueries({ queryKey: qk.projectFilters(tabID ?? "") });
  }, [qc, tabID]);

  const { mutateAsync: select } = selection;
  const setActive = useCallback(
    async (id: string) => {
      if (tabID && enabled) await select({ tab: tabID, id });
    },
    [tabID, enabled, select],
  );

  const saveFilter = useCallback(
    async (f: ProjectFilter) => {
      if (!tabID) return null;
      const saved = await api.saveProjectFilter(tabID, f);
      await reload();
      return saved ?? null;
    },
    [tabID, reload],
  );

  const deleteFilter = useCallback(
    async (id: string) => {
      if (!tabID) return;
      await api.deleteProjectFilter(tabID, id);
      await reload();
    },
    [tabID, reload],
  );

  const active = filters.find((f) => f.id === activeId) ?? null;

  return (
    <ActiveFilterContext.Provider
      value={{ filters, activeId, active, enabled, setActive, saveFilter, deleteFilter, reload }}
    >
      {children}
    </ActiveFilterContext.Provider>
  );
}

export function useActiveFilter() {
  return useContext(ActiveFilterContext);
}

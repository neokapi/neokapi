import { useCallback } from "react";
import { useNavigate, useSearch } from "@tanstack/react-router";
import type { FilterToken } from "@neokapi/ui";

/**
 * Syncs FilterBar state (filters + search) with URL search params.
 * Each filter token becomes a query param (e.g. plan=pro&status=active).
 * Free-text search is stored as ?q=... in the URL.
 *
 * @param filterKeys - The known filter keys (e.g. ["plan", "status"]).
 * @param routePath - The route path for navigation (e.g. "/workspaces").
 */
export function useUrlFilters(filterKeys: string[], routePath: string) {
  const search = useSearch({ strict: false }) as Record<string, string | undefined>;
  const navigate = useNavigate();

  // Derive FilterBar state from URL params.
  const filters: FilterToken[] = [];
  for (const key of filterKeys) {
    if (search[key]) {
      filters.push({ key, value: search[key]! });
    }
  }
  const searchText = search.q ?? "";

  const setFilters = useCallback(
    (newFilters: FilterToken[]) => {
      const params: Record<string, string | undefined> = { q: search.q };
      // Clear all filter keys first.
      for (const key of filterKeys) {
        params[key] = undefined;
      }
      // Set new filter values.
      for (const f of newFilters) {
        params[f.key] = f.value;
      }
      void navigate({ to: routePath, search: params, replace: true });
    },
    [navigate, routePath, filterKeys, search.q],
  );

  const setSearch = useCallback(
    (newSearch: string) => {
      const params: Record<string, string | undefined> = {};
      // Preserve existing filter keys.
      for (const key of filterKeys) {
        params[key] = search[key];
      }
      params.q = newSearch || undefined;
      void navigate({ to: routePath, search: params, replace: true });
    },
    [navigate, routePath, filterKeys, search],
  );

  return { filters, search: searchText, setFilters, setSearch };
}

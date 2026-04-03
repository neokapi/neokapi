import { createContext, useContext, useState, useCallback, useEffect, type ReactNode } from "react";
import { useNavigate, useParams, useLocation } from "react-router-dom";
import { useApi } from "@/context/ApiContext";

export interface FilterToken {
  key: string;
  value: string;
  label: string;
}

interface FilterContextType {
  tokens: FilterToken[];
  addToken: (token: FilterToken) => void;
  removeToken: (index: number) => void;
  clearTokens: () => void;
  search: string;
  setSearch: (s: string) => void;
  // Derived for backward compat
  workspace: string | null;
  agent: string | null;
  status: string | null;
}

export const FilterContext = createContext<FilterContextType>({
  tokens: [],
  addToken: () => {},
  removeToken: () => {},
  clearTokens: () => {},
  search: "",
  setSearch: () => {},
  workspace: null,
  agent: null,
  status: null,
});

export function useFilter() {
  return useContext(FilterContext);
}

function tokensToPath(tokens: FilterToken[]): string {
  const wsToken = tokens.find((t) => t.key === "workspace");
  const agentToken = tokens.find((t) => t.key === "agent");

  if (wsToken && agentToken) {
    return `/workspace/${wsToken.value}/agent/${agentToken.value}`;
  }
  if (wsToken) {
    return `/workspace/${wsToken.value}`;
  }
  return "/";
}

function deriveValue(tokens: FilterToken[], key: string): string | null {
  const token = tokens.find((t) => t.key === key);
  return token?.value ?? null;
}

export function FilterProvider({ children }: { children: ReactNode }) {
  const navigate = useNavigate();
  const params = useParams();
  const location = useLocation();
  const api = useApi();
  const [tokens, setTokens] = useState<FilterToken[]>([]);
  const [search, setSearch] = useState("");
  const [initialized, setInitialized] = useState(false);

  // Initialize tokens from URL on first load
  useEffect(() => {
    if (initialized) return;
    const initial: FilterToken[] = [];
    const slug = params.slug;
    const agentId = params.agentId;

    if (slug) {
      // Try API workspaces first, then use slug as label fallback
      const ws = api.workspaces.find((w) => w.slug === slug);
      initial.push({ key: "workspace", value: slug, label: ws?.name ?? slug });
    }
    if (agentId) {
      const ag = api.agents.find((a) => a.agent === agentId);
      initial.push({ key: "agent", value: agentId, label: ag?.agent ?? agentId });
    }

    // Parse query params for status, time, tool
    const searchParams = new URLSearchParams(location.search);
    const statusParam = searchParams.get("status");
    if (statusParam) {
      initial.push({ key: "status", value: statusParam, label: statusParam });
    }
    const timeParam = searchParams.get("time");
    if (timeParam) {
      initial.push({ key: "time", value: timeParam, label: timeParam });
    }
    const toolParam = searchParams.get("tool");
    if (toolParam) {
      initial.push({ key: "tool", value: toolParam, label: toolParam });
    }
    const q = searchParams.get("q");
    if (q) {
      setSearch(q);
    }

    if (initial.length > 0) {
      setTokens(initial);
    }
    setInitialized(true);
  }, [params, location.search, initialized, api.workspaces, api.agents]);

  const syncUrl = useCallback(
    (newTokens: FilterToken[], newSearch?: string) => {
      const path = tokensToPath(newTokens);
      const qs = new URLSearchParams();

      for (const t of newTokens) {
        if (t.key === "status" || t.key === "time" || t.key === "tool") {
          qs.set(t.key, t.value);
        }
      }
      const s = newSearch ?? search;
      if (s) qs.set("q", s);

      const query = qs.toString();
      navigate(query ? `${path}?${query}` : path, { replace: true });
    },
    [navigate, search],
  );

  const addToken = useCallback(
    (token: FilterToken) => {
      setTokens((prev) => {
        // Replace existing token with same key
        const filtered = prev.filter((t) => t.key !== token.key);
        const next = [...filtered, token];
        syncUrl(next);
        return next;
      });
    },
    [syncUrl],
  );

  const removeToken = useCallback(
    (index: number) => {
      setTokens((prev) => {
        const next = prev.filter((_, i) => i !== index);
        syncUrl(next);
        return next;
      });
    },
    [syncUrl],
  );

  const clearTokens = useCallback(() => {
    setTokens([]);
    setSearch("");
    navigate("/", { replace: true });
  }, [navigate]);

  const setSearchWithSync = useCallback((s: string) => {
    setSearch(s);
    // Don't navigate on every keystroke -- just update state
  }, []);

  // Derived values for backward compat
  const workspace = deriveValue(tokens, "workspace");
  const agent = deriveValue(tokens, "agent");
  const status = deriveValue(tokens, "status");

  return (
    <FilterContext.Provider
      value={{
        tokens,
        addToken,
        removeToken,
        clearTokens,
        search,
        setSearch: setSearchWithSync,
        workspace,
        agent,
        status,
      }}
    >
      {children}
    </FilterContext.Provider>
  );
}

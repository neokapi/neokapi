import {
  createContext,
  useContext,
  useState,
  useCallback,
  useMemo,
  type ReactNode,
} from 'react';
import type { AgentSession } from '@/data/sessions';

export interface FilterToken {
  key: string; // "workspace", "agent", "status", "time", "tool"
  value: string; // "excalidraw", "sophie-martin", "failed", "today"
  label: string; // "Excalidraw", "Sophie Martin", "Failed"
}

interface FilterContextValue {
  tokens: FilterToken[];
  addToken: (token: FilterToken) => void;
  removeToken: (index: number) => void;
  replaceTokens: (tokens: FilterToken[]) => void;
  clearTokens: () => void;
  search: string;
  setSearch: (s: string) => void;
  // Derived helpers
  getFilter: (key: string) => string | undefined;
  matchesSession: (session: AgentSession) => boolean;
  // Backward-compat derived values
  workspace: string | null;
  agent: string | null;
  status: string | null;
}

export const FilterContext = createContext<FilterContextValue>({
  tokens: [],
  addToken: () => {},
  removeToken: () => {},
  replaceTokens: () => {},
  clearTokens: () => {},
  search: '',
  setSearch: () => {},
  getFilter: () => undefined,
  matchesSession: () => true,
  workspace: null,
  agent: null,
  status: null,
});

export function useFilter() {
  return useContext(FilterContext);
}

function isToday(iso: string): boolean {
  const d = new Date(iso);
  const today = new Date();
  return d.toDateString() === today.toDateString();
}

function isYesterday(iso: string): boolean {
  const d = new Date(iso);
  const y = new Date();
  y.setDate(y.getDate() - 1);
  return d.toDateString() === y.toDateString();
}

function isThisWeek(iso: string): boolean {
  const d = new Date(iso);
  const now = new Date();
  const startOfWeek = new Date(now);
  startOfWeek.setDate(now.getDate() - now.getDay());
  startOfWeek.setHours(0, 0, 0, 0);
  return d >= startOfWeek;
}

function isThisMonth(iso: string): boolean {
  const d = new Date(iso);
  const now = new Date();
  return d.getMonth() === now.getMonth() && d.getFullYear() === now.getFullYear();
}

function isWithinDays(iso: string, days: number): boolean {
  const d = new Date(iso);
  const cutoff = new Date();
  cutoff.setDate(cutoff.getDate() - days);
  cutoff.setHours(0, 0, 0, 0);
  return d >= cutoff;
}

function matchesTime(iso: string, timeValue: string): boolean {
  switch (timeValue) {
    case 'today':
      return isToday(iso);
    case 'yesterday':
      return isYesterday(iso);
    case 'this-week':
      return isThisWeek(iso);
    case 'this-month':
      return isThisMonth(iso);
    case '7d':
      return isWithinDays(iso, 7);
    case '14d':
      return isWithinDays(iso, 14);
    case '30d':
      return isWithinDays(iso, 30);
    default:
      return true;
  }
}

export function FilterProvider({ children }: { children: ReactNode }) {
  const [tokens, setTokens] = useState<FilterToken[]>([]);
  const [search, setSearch] = useState('');

  const addToken = useCallback((token: FilterToken) => {
    setTokens((prev) => {
      // Replace existing token with same key
      const filtered = prev.filter((t) => t.key !== token.key);
      // If setting workspace, clear agent filter
      if (token.key === 'workspace') {
        return [...filtered.filter((t) => t.key !== 'agent'), token];
      }
      return [...filtered, token];
    });
  }, []);

  const removeToken = useCallback((index: number) => {
    setTokens((prev) => {
      const removed = prev[index];
      const next = prev.filter((_, i) => i !== index);
      // If removing workspace, also remove agent
      if (removed?.key === 'workspace') {
        return next.filter((t) => t.key !== 'agent');
      }
      return next;
    });
  }, []);

  const replaceTokens = useCallback((newTokens: FilterToken[]) => {
    setTokens(newTokens);
  }, []);

  const clearTokens = useCallback(() => {
    setTokens([]);
    setSearch('');
  }, []);

  const getFilter = useCallback(
    (key: string) => tokens.find((t) => t.key === key)?.value,
    [tokens]
  );

  const workspace = useMemo(() => tokens.find((t) => t.key === 'workspace')?.value ?? null, [tokens]);
  const agent = useMemo(() => tokens.find((t) => t.key === 'agent')?.value ?? null, [tokens]);
  const status = useMemo(() => tokens.find((t) => t.key === 'status')?.value ?? null, [tokens]);

  const matchesSession = useCallback(
    (session: AgentSession) => {
      for (const token of tokens) {
        switch (token.key) {
          case 'workspace':
            if (session.workspace !== token.value) return false;
            break;
          case 'agent':
            if (session.agentId !== token.value) return false;
            break;
          case 'status':
            if (session.status !== token.value) return false;
            break;
          case 'time':
            if (!matchesTime(session.startTime, token.value)) return false;
            break;
          case 'tool':
            if (!session.toolCalls.some((tc) => tc.tool === token.value)) return false;
            break;
        }
      }
      if (search) {
        const q = search.toLowerCase();
        if (!session.summary.toLowerCase().includes(q)) return false;
      }
      return true;
    },
    [tokens, search]
  );

  return (
    <FilterContext.Provider
      value={{
        tokens,
        addToken,
        removeToken,
        replaceTokens,
        clearTokens,
        search,
        setSearch,
        getFilter,
        matchesSession,
        workspace,
        agent,
        status,
      }}
    >
      {children}
    </FilterContext.Provider>
  );
}

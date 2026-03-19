import { createContext, useContext, useState, useCallback, type ReactNode } from 'react';

interface FilterState {
  workspace: string | null;
  agent: string | null;
  status: string | null;
  search: string;
  preset: string | null;
}

interface FilterContextValue extends FilterState {
  setWorkspace: (ws: string | null) => void;
  setAgent: (agent: string | null) => void;
  setStatus: (status: string | null) => void;
  setSearch: (search: string) => void;
  setPreset: (preset: string | null) => void;
  clearFilters: () => void;
}

export const FilterContext = createContext<FilterContextValue>({
  workspace: null,
  agent: null,
  status: null,
  search: '',
  preset: null,
  setWorkspace: () => {},
  setAgent: () => {},
  setStatus: () => {},
  setSearch: () => {},
  setPreset: () => {},
  clearFilters: () => {},
});

export function useFilter() {
  return useContext(FilterContext);
}

export function FilterProvider({ children }: { children: ReactNode }) {
  const [workspace, setWorkspaceRaw] = useState<string | null>(null);
  const [agent, setAgent] = useState<string | null>(null);
  const [status, setStatus] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [preset, setPreset] = useState<string | null>(null);

  const setWorkspace = useCallback((ws: string | null) => {
    setWorkspaceRaw(ws);
    setAgent(null);
  }, []);

  const clearFilters = useCallback(() => {
    setWorkspaceRaw(null);
    setAgent(null);
    setStatus(null);
    setSearch('');
    setPreset(null);
  }, []);

  return (
    <FilterContext.Provider
      value={{
        workspace,
        agent,
        status,
        search,
        preset,
        setWorkspace,
        setAgent,
        setStatus,
        setSearch,
        setPreset,
        clearFilters,
      }}
    >
      {children}
    </FilterContext.Provider>
  );
}

import { createContext, useContext, useState, type ReactNode } from 'react';

interface FilterState {
  workspace: string | null;
  project: string | null;
  agent: string | null;
}

interface FilterContextValue {
  filters: FilterState;
  selectedWorkspace: string | null;
  setSelectedWorkspace: (ws: string | null) => void;
  setProject: (project: string | null) => void;
  setAgent: (agent: string | null) => void;
}

export const FilterContext = createContext<FilterContextValue>({
  filters: { workspace: null, project: null, agent: null },
  selectedWorkspace: null,
  setSelectedWorkspace: () => {},
  setProject: () => {},
  setAgent: () => {},
});

export function useFilter() {
  return useContext(FilterContext);
}

export function FilterProvider({ children }: { children: ReactNode }) {
  const [filters, setFilters] = useState<FilterState>({
    workspace: null,
    project: null,
    agent: null,
  });

  const setSelectedWorkspace = (ws: string | null) => {
    setFilters({ workspace: ws, project: null, agent: null });
  };

  const setProject = (project: string | null) => {
    setFilters((prev) => ({ ...prev, project }));
  };

  const setAgent = (agent: string | null) => {
    setFilters((prev) => ({ ...prev, agent }));
  };

  return (
    <FilterContext.Provider
      value={{
        filters,
        selectedWorkspace: filters.workspace,
        setSelectedWorkspace,
        setProject,
        setAgent,
      }}
    >
      {children}
    </FilterContext.Provider>
  );
}

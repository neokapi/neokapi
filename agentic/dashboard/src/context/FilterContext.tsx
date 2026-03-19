import { createContext, useContext, useState, type ReactNode } from 'react';

interface FilterState {
  workspace: string | null;
  agent: string | null;
}

interface FilterContextValue extends FilterState {
  setWorkspace: (ws: string | null) => void;
  setAgent: (agent: string | null) => void;
}

export const FilterContext = createContext<FilterContextValue>({
  workspace: null,
  agent: null,
  setWorkspace: () => {},
  setAgent: () => {},
});

export function useFilter() {
  return useContext(FilterContext);
}

export function FilterProvider({ children }: { children: ReactNode }) {
  const [workspace, setWorkspace] = useState<string | null>(null);
  const [agent, setAgent] = useState<string | null>(null);

  return (
    <FilterContext.Provider
      value={{
        workspace,
        agent,
        setWorkspace: (ws) => {
          setWorkspace(ws);
          setAgent(null); // reset agent when workspace changes
        },
        setAgent,
      }}
    >
      {children}
    </FilterContext.Provider>
  );
}

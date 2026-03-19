import { createContext, useContext, useState, type ReactNode } from 'react';

interface FilterContextValue {
  selectedWorkspace: string | null; // null = all
  setSelectedWorkspace: (ws: string | null) => void;
}

export const FilterContext = createContext<FilterContextValue>({
  selectedWorkspace: null,
  setSelectedWorkspace: () => {},
});

export function useFilter() {
  return useContext(FilterContext);
}

export function FilterProvider({ children }: { children: ReactNode }) {
  const [selectedWorkspace, setSelectedWorkspace] = useState<string | null>(null);

  return (
    <FilterContext.Provider value={{ selectedWorkspace, setSelectedWorkspace }}>
      {children}
    </FilterContext.Provider>
  );
}

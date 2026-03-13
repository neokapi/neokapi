import { createContext, useContext, useState, useEffect, useCallback, type ReactNode } from "react";

interface BreadcrumbContextValue {
  breadcrumb: ReactNode;
  setBreadcrumb: (node: ReactNode) => void;
}

const BreadcrumbContext = createContext<BreadcrumbContextValue>({
  breadcrumb: null,
  setBreadcrumb: () => {},
});

export function BreadcrumbProvider({ children }: { children: ReactNode }) {
  const [breadcrumb, setBreadcrumb] = useState<ReactNode>(null);
  return (
    <BreadcrumbContext.Provider value={{ breadcrumb, setBreadcrumb }}>
      {children}
    </BreadcrumbContext.Provider>
  );
}

/** Read the current breadcrumb (used by the header/top bar area). */
export function useBreadcrumb() {
  return useContext(BreadcrumbContext).breadcrumb;
}

/** Set a breadcrumb for the current view. Clears on unmount. */
export function useSetBreadcrumb(node: ReactNode) {
  const { setBreadcrumb } = useContext(BreadcrumbContext);
  const stableSetBreadcrumb = useCallback((n: ReactNode) => setBreadcrumb(n), [setBreadcrumb]);
  useEffect(() => {
    stableSetBreadcrumb(node);
    return () => stableSetBreadcrumb(null);
  }, [node, stableSetBreadcrumb]);
}

import { createContext, useContext, type ReactNode } from "react";
import type { ApiAdapter } from "../api/adapter";

const ApiContext = createContext<ApiAdapter | null>(null);

export function ApiProvider({ adapter, children }: { adapter: ApiAdapter; children: ReactNode }) {
  return <ApiContext.Provider value={adapter}>{children}</ApiContext.Provider>;
}

export function useApi(): ApiAdapter {
  const ctx = useContext(ApiContext);
  if (!ctx) {
    throw new Error("useApi must be used within an ApiProvider");
  }
  return ctx;
}

import { createContext, useContext, useMemo, useState, useCallback } from "react";
import type { StreamInfo } from "../types/api";

interface StreamActions {
  onCreateStream?: () => void;
  onEditStream?: (stream: StreamInfo) => void;
  onMergeStream?: (streamName: string) => void;
  onDiffStream?: (streamName: string) => void;
  onDeleteStream?: (streamName: string) => void;
}

interface StreamActionsContextValue {
  actions: StreamActions;
  setActions: (actions: StreamActions) => void;
}

const StreamActionsContext = createContext<StreamActionsContextValue>({
  actions: {},
  setActions: () => {},
});

export function StreamActionsProvider({ children }: { children: React.ReactNode }) {
  const [actions, setActionsState] = useState<StreamActions>({});

  const setActions = useCallback((a: StreamActions) => {
    setActionsState(a);
  }, []);

  const value = useMemo(() => ({ actions, setActions }), [actions, setActions]);

  return (
    <StreamActionsContext.Provider value={value}>
      {children}
    </StreamActionsContext.Provider>
  );
}

export function useStreamActions() {
  return useContext(StreamActionsContext);
}

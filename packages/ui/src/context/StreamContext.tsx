import { createContext, useContext, useState, useCallback, type ReactNode } from "react";

interface StreamContextValue {
  /** The currently active stream name. Defaults to "main". */
  activeStream: string;
  /** Set the active stream. */
  setActiveStream: (stream: string) => void;
}

const StreamContext = createContext<StreamContextValue>({
  activeStream: "main",
  setActiveStream: () => {},
});

export function StreamProvider({ initialStream = "main", children }: { initialStream?: string; children: ReactNode }) {
  const [activeStream, setActiveStreamState] = useState(initialStream);

  const setActiveStream = useCallback((stream: string) => {
    setActiveStreamState(stream || "main");
  }, []);

  return (
    <StreamContext.Provider value={{ activeStream, setActiveStream }}>
      {children}
    </StreamContext.Provider>
  );
}

export function useStream(): StreamContextValue {
  return useContext(StreamContext);
}

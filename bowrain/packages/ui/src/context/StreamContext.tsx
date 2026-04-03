import { createContext, useContext, useState, useCallback, useEffect, type ReactNode } from "react";

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

export function StreamProvider({
  initialStream = "main",
  onStreamChange,
  children,
}: {
  initialStream?: string;
  /** Called when the active stream changes (e.g. to sync URL search params). */
  onStreamChange?: (stream: string) => void;
  children: ReactNode;
}) {
  const [activeStream, setActiveStreamState] = useState(initialStream);

  // Sync when parent changes initialStream (e.g., URL navigation).
  useEffect(() => {
    setActiveStreamState(initialStream);
  }, [initialStream]);

  const setActiveStream = useCallback(
    (stream: string) => {
      const s = stream || "main";
      setActiveStreamState(s);
      onStreamChange?.(s);
    },
    [onStreamChange],
  );

  return (
    <StreamContext.Provider value={{ activeStream, setActiveStream }}>
      {children}
    </StreamContext.Provider>
  );
}

export function useStream(): StreamContextValue {
  return useContext(StreamContext);
}

import {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
  useRef,
  type ReactNode,
} from "react";
import { AlertCircle, X, Copy, ChevronDown } from "lucide-react";
import { Button } from "@neokapi/ui-primitives";

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

interface ErrorEntry {
  id: number;
  message: string;
  details: string;
  timestamp: number;
}

interface ErrorContextValue {
  showError: (message: string, details?: unknown) => void;
}

/* ------------------------------------------------------------------ */
/*  Context                                                            */
/* ------------------------------------------------------------------ */

const ErrorContext = createContext<ErrorContextValue | null>(null);

export function useError(): ErrorContextValue {
  const ctx = useContext(ErrorContext);
  if (!ctx) throw new Error("useError must be used within <ErrorProvider>");
  return ctx;
}

/* ------------------------------------------------------------------ */
/*  Provider                                                           */
/* ------------------------------------------------------------------ */

let nextId = 0;

function formatDetails(details: unknown): string {
  if (details === undefined || details === null) return "";
  if (details instanceof Error) {
    return details.stack ?? details.message;
  }
  if (typeof details === "string") return details;
  try {
    return JSON.stringify(details, null, 2);
  } catch {
    return String(details);
  }
}

export function ErrorProvider({ children }: { children: ReactNode }) {
  const [errors, setErrors] = useState<ErrorEntry[]>([]);

  const showError = useCallback((message: string, details?: unknown) => {
    const entry: ErrorEntry = {
      id: ++nextId,
      message,
      details: formatDetails(details),
      timestamp: Date.now(),
    };
    setErrors((prev) => [entry, ...prev].slice(0, 3));
  }, []);

  const dismiss = useCallback((id: number) => {
    setErrors((prev) => prev.filter((e) => e.id !== id));
  }, []);

  return (
    <ErrorContext.Provider value={{ showError }}>
      {children}
      <ErrorBannerStack errors={errors} onDismiss={dismiss} />
    </ErrorContext.Provider>
  );
}

/* ------------------------------------------------------------------ */
/*  Banner Stack                                                       */
/* ------------------------------------------------------------------ */

function ErrorBannerStack({
  errors,
  onDismiss,
}: {
  errors: ErrorEntry[];
  onDismiss: (id: number) => void;
}) {
  if (errors.length === 0) return null;
  return (
    <div className="fixed bottom-4 right-4 z-[100] flex flex-col gap-2 max-w-sm">
      {errors.map((err) => (
        <ErrorBannerItem key={err.id} entry={err} onDismiss={onDismiss} />
      ))}
    </div>
  );
}

/* ------------------------------------------------------------------ */
/*  Single Banner                                                      */
/* ------------------------------------------------------------------ */

const AUTO_DISMISS_MS = 8000;

function ErrorBannerItem({
  entry,
  onDismiss,
}: {
  entry: ErrorEntry;
  onDismiss: (id: number) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const [copied, setCopied] = useState(false);
  const hovering = useRef(false);
  const timerRef = useRef<ReturnType<typeof setTimeout>>();

  const startTimer = useCallback(() => {
    clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => {
      if (!hovering.current) onDismiss(entry.id);
    }, AUTO_DISMISS_MS);
  }, [entry.id, onDismiss]);

  useEffect(() => {
    startTimer();
    return () => clearTimeout(timerRef.current);
  }, [startTimer]);

  const handleMouseEnter = () => {
    hovering.current = true;
    clearTimeout(timerRef.current);
  };

  const handleMouseLeave = () => {
    hovering.current = false;
    startTimer();
  };

  const handleCopy = async () => {
    const text = `${entry.message}\n\n${entry.details}`;
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      // Clipboard may be unavailable.
    }
  };

  return (
    <div
      onMouseEnter={handleMouseEnter}
      onMouseLeave={handleMouseLeave}
      className="animate-in slide-in-from-right rounded-lg border border-destructive/30 bg-destructive/10 p-3 shadow-lg backdrop-blur-sm"
    >
      {/* Header row */}
      <div className="flex items-start gap-2">
        <AlertCircle size={16} className="mt-0.5 shrink-0 text-destructive" />
        <p className="flex-1 text-sm font-medium text-foreground">{entry.message}</p>
        <Button
          variant="ghost"
          size="icon-xs"
          onClick={() => onDismiss(entry.id)}
          className="shrink-0"
          aria-label="Dismiss error"
        >
          <X size={14} />
        </Button>
      </div>

      {/* Actions row */}
      {entry.details && (
        <div className="mt-2 flex items-center gap-2 pl-6">
          <Button
            variant="ghost"
            size="xs"
            onClick={() => setExpanded((v) => !v)}
            className="px-0 h-auto text-[11px] text-muted-foreground hover:text-foreground"
          >
            <ChevronDown
              size={12}
              className={`transition-transform ${expanded ? "rotate-180" : ""}`}
            />
            Details
          </Button>
          <Button
            variant="ghost"
            size="xs"
            onClick={() => void handleCopy()}
            className="px-0 h-auto text-[11px] text-muted-foreground hover:text-foreground"
          >
            <Copy size={10} />
            {copied ? "Copied" : "Copy Details"}
          </Button>
        </div>
      )}

      {/* Expanded details */}
      {expanded && entry.details && (
        <pre className="mt-2 ml-6 max-h-32 overflow-auto rounded border border-border bg-background/80 p-2 text-[10px] text-muted-foreground whitespace-pre-wrap break-words">
          {entry.details}
        </pre>
      )}
    </div>
  );
}

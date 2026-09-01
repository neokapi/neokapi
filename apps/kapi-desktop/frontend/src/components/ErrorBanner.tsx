import {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
  useRef,
  type ReactNode,
} from "react";
import { X, Copy } from "lucide-react";
import { Button, ErrorNotice, parseAppError } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { writeClipboardText } from "../lib/clipboard";

/* ------------------------------------------------------------------ */
/*  Types                                                              */
/* ------------------------------------------------------------------ */

interface ErrorEntry {
  id: number;
  message: string;
  details: unknown;
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

export function ErrorProvider({ children }: { children: ReactNode }) {
  const [errors, setErrors] = useState<ErrorEntry[]>([]);

  const showError = useCallback((message: string, details?: unknown) => {
    const entry: ErrorEntry = {
      id: ++nextId,
      message,
      details,
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
  const [copied, setCopied] = useState(false);
  const hovering = useRef(false);
  const timerRef = useRef<ReturnType<typeof setTimeout>>(undefined);

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

  // When details are provided, the caller's message is the contextual title
  // and the details carry the underlying error; otherwise parse the message
  // itself (it may be a raw Wails envelope string).
  const hasDetails = entry.details !== undefined && entry.details !== null;
  const errorValue = hasDetails ? entry.details : entry.message;
  const parsed = parseAppError(errorValue);

  const handleCopy = async () => {
    const parts = [entry.message];
    if (hasDetails && parsed.title !== entry.message) parts.push(parsed.title);
    if (parsed.detail) parts.push(parsed.detail);
    if (parsed.raw) parts.push(parsed.raw);
    try {
      await writeClipboardText(parts.join("\n\n"));
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
      className="animate-in slide-in-from-right relative rounded-lg border border-destructive/30 bg-destructive/10 p-3 pr-9 shadow-lg backdrop-blur-sm"
    >
      <ErrorNotice
        error={errorValue}
        title={hasDetails ? entry.message : undefined}
        detailsLabel={t("Details")}
        actions={
          <Button
            variant="ghost"
            size="xs"
            onClick={() => void handleCopy()}
            className="px-0 h-auto text-[11px] text-muted-foreground hover:text-foreground"
          >
            <Copy size={10} />
            {copied ? t("Copied") : t("Copy Details")}
          </Button>
        }
      />
      <Button
        variant="ghost"
        size="icon-xs"
        onClick={() => onDismiss(entry.id)}
        className="absolute right-2 top-2 shrink-0"
        aria-label="Dismiss error"
      >
        <X size={14} />
      </Button>
    </div>
  );
}

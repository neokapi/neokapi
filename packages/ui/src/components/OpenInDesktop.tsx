import { useState, useEffect, useCallback } from "react";
import { Button } from "./ui/button";
import { X, Monitor } from "./icons";

interface OpenInDesktopProps {
  projectId: string;
  serverURL: string;
  workspaceSlug: string;
}

const DISMISSED_KEY = "bowrain-open-in-desktop-dismissed";

function detectOS(): "Mac" | "Windows" | "Linux" | "unknown" {
  const ua = navigator.userAgent;
  if (/Mac/.test(ua)) return "Mac";
  if (/Win/.test(ua)) return "Windows";
  if (/Linux/.test(ua)) return "Linux";
  return "unknown";
}

function buildDeepLink(projectId: string, serverURL: string, workspaceSlug: string): string {
  return `bowrain://project/${projectId}?server=${encodeURIComponent(serverURL)}&workspace=${encodeURIComponent(workspaceSlug)}`;
}

export function OpenInDesktop({ projectId, serverURL, workspaceSlug }: OpenInDesktopProps) {
  const [dismissed, setDismissed] = useState(() => {
    try {
      return localStorage.getItem(DISMISSED_KEY) === "true";
    } catch {
      return false;
    }
  });
  const [showFallback, setShowFallback] = useState(false);

  const os = detectOS();
  const deepLink = buildDeepLink(projectId, serverURL, workspaceSlug);

  const handleOpen = useCallback(() => {
    setShowFallback(false);

    // Try opening via deep link.
    window.location.href = deepLink;

    // If the app is installed, the window will blur as the OS switches to it.
    // If not, nothing visible happens and we show a download fallback.
    const timer = setTimeout(() => {
      setShowFallback(true);
    }, 1500);

    const onBlur = () => {
      clearTimeout(timer);
      window.removeEventListener("blur", onBlur);
    };
    window.addEventListener("blur", onBlur);

    return () => {
      clearTimeout(timer);
      window.removeEventListener("blur", onBlur);
    };
  }, [deepLink]);

  const handleDismiss = useCallback(() => {
    setDismissed(true);
    try {
      localStorage.setItem(DISMISSED_KEY, "true");
    } catch {
      // localStorage unavailable
    }
  }, []);

  // Reset fallback when project changes.
  useEffect(() => {
    setShowFallback(false);
  }, [projectId]);

  if (dismissed) return null;

  const osLabel = os !== "unknown" ? ` for ${os}` : "";

  return (
    <div className="mb-4 flex items-center justify-between rounded-lg border border-border bg-muted/50 px-4 py-3">
      <div className="flex items-center gap-3">
        <Monitor className="w-5 h-5 text-muted-foreground" />
        <div>
          <p className="text-sm font-medium">Open in Bowrain Desktop</p>
          <p className="text-xs text-muted-foreground">
            {showFallback
              ? `Bowrain Desktop not found.`
              : `Edit this project in the desktop app${osLabel}.`}
          </p>
        </div>
      </div>
      <div className="flex items-center gap-2">
        {showFallback ? (
          <Button variant="outline" size="sm" asChild>
            <a href="https://bowrain.dev/download" target="_blank" rel="noopener noreferrer">
              Download{osLabel}
            </a>
          </Button>
        ) : (
          <Button variant="outline" size="sm" onClick={handleOpen} data-testid="open-in-desktop-btn" data-href={deepLink}>
            Open{osLabel}
          </Button>
        )}
        <button
          onClick={handleDismiss}
          className="p-1 rounded hover:bg-muted text-muted-foreground"
          aria-label="Dismiss"
          data-testid="dismiss-open-in-desktop"
        >
          <X className="w-4 h-4" />
        </button>
      </div>
    </div>
  );
}

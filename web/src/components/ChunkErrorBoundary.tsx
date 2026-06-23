import React, { Suspense } from "react";
import { isChunkLoadError, maybeReloadForStaleChunk } from "../lib/chunkReload";

// Error boundary for the lazily-loaded lab explorers and the playground modal.
//
// Two failure modes are handled distinctly:
//   • Stale-deploy chunk failure (the common case): the widget's JS chunk was
//     removed by a newer deploy. We trigger a single guarded page reload — the
//     reload re-fetches the current deploy and the import resolves. While that
//     happens (or if the reload is suppressed by the loop guard) we show a small
//     "updating…" card with a manual refresh, never a red crash.
//   • Any other render error: contained to this widget so a broken lab can't
//     take down the surrounding docs page; logged for diagnosis.

interface Props {
  children: React.ReactNode;
  /** Fallback shown for non-chunk errors. Defaults to a quiet inline notice. */
  fallback?: React.ReactNode;
}

interface State {
  failed: boolean;
  stale: boolean;
}

const noticeStyle: React.CSSProperties = {
  padding: "1rem 1.25rem",
  border: "1px solid var(--ifm-color-emphasis-300)",
  borderRadius: "var(--ifm-global-radius, 8px)",
  background: "var(--ifm-background-surface-color)",
  color: "var(--ifm-color-emphasis-700)",
  fontSize: "0.95rem",
};

const linkButtonStyle: React.CSSProperties = {
  background: "none",
  border: "none",
  padding: 0,
  color: "var(--ifm-color-primary)",
  font: "inherit",
  cursor: "pointer",
  textDecoration: "underline",
};

export class ChunkErrorBoundary extends React.Component<Props, State> {
  state: State = { failed: false, stale: false };

  static getDerivedStateFromError(error: unknown): State {
    return { failed: true, stale: isChunkLoadError(error) };
  }

  componentDidCatch(error: unknown): void {
    if (isChunkLoadError(error)) {
      // Refresh to the current deploy. Guarded so it can't loop.
      maybeReloadForStaleChunk();
    } else {
      console.error("Lab widget failed to render:", error);
    }
  }

  render(): React.ReactNode {
    if (!this.state.failed) return this.props.children;

    if (this.state.stale) {
      return (
        <div style={noticeStyle} role="status" aria-live="polite">
          A newer version of this page is available. Refreshing…{" "}
          <button type="button" style={linkButtonStyle} onClick={() => window.location.reload()}>
            Reload now
          </button>
        </div>
      );
    }

    return (
      this.props.fallback ?? (
        <div style={noticeStyle} role="alert">
          This interactive demo failed to load. Try reloading the page.
        </div>
      )
    );
  }
}

/**
 * `<Suspense>` wrapped in a {@link ChunkErrorBoundary}. The boundary sits outside
 * Suspense so it catches the rejection thrown by a failed `React.lazy` import.
 */
export function ChunkSafeSuspense({
  fallback,
  children,
}: {
  fallback?: React.ReactNode;
  children: React.ReactNode;
}): React.ReactElement {
  return (
    <ChunkErrorBoundary>
      <Suspense fallback={fallback ?? null}>{children}</Suspense>
    </ChunkErrorBoundary>
  );
}

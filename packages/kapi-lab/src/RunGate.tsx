import React from "react";
import { Play } from "lucide-react";
import { Button, cn } from "@neokapi/ui-primitives";
import { PLUGIN_DESCRIPTORS, usePluginManager } from "@neokapi/kapi-playground/plugins";
import type { PluginId } from "@neokapi/kapi-playground/plugins";
import type { RunGate as RunGateState } from "./useRunGate";

// RunGate — the shared activation gate every lab renders until it is `ready`.
// Nothing in the lab boots, downloads, or runs until the user presses the
// primary action, so page load stays inert (matches the desktop/CLI "explicit
// run" model). One component, one copy pattern, for every lab and embed:
//
//   idle     title · one-line description · [▶ Run in your browser]
//            "~13 MB engine · runs locally — nothing leaves your machine"
//   booting  spinner + "Starting the engine…" (+ download progress bars)
//   error    the message + Retry
//
// When the engine singleton is already running (another lab on the page was
// activated), the gate stays up — running is always explicit — but the hint
// notes the engine is warm and the start is instant.
//
// `compact` renders the SAME states as a slim inline row (small button + the
// identical size-hint/runs-locally line) for widgets embedded in prose or a
// command strip, where the full card would dwarf the content it gates. Same
// copy pattern, same tokens — a density variant, not a third design.

export interface RunGateProps {
  gate: RunGateState;
  /** Heading, e.g. "Conversion Lab". */
  title?: string;
  /** One-line description of what Run will do. */
  description?: string;
  /** Primary action label (default "Run in your browser"). */
  label?: string;
  /**
   * Whether Run boots the shared wasm engine (default true). Labs that only
   * fetch ONNX models (e.g. Vision) pass false so the size hint lists just
   * their own downloads.
   */
  engine?: boolean;
  /**
   * Sizing/layout class for the gate's outer card. Callers pass a class that
   * reserves the height the activated content will occupy, so pressing play
   * swaps content in place without a layout shift. Defaults to `min-h-[180px]`.
   */
  className?: string;
  /**
   * Slim inline row instead of the centered card — for small widgets (curated
   * result views, command strips) where a card is out of scale. Identical
   * states and copy; `title`/`description` are not rendered in compact.
   */
  compact?: boolean;
}

/** The wasm engine module is ~13 MB (gzipped) on first fetch. */
const ENGINE_SIZE = "~13 MB";

function fmtSize(bytes?: number): string {
  if (!bytes) return "";
  if (bytes >= 1_000_000_000) return `${(bytes / 1_000_000_000).toFixed(1)} GB`;
  if (bytes >= 1_000_000) return `${(bytes / 1_000_000).toFixed(0)} MB`;
  return `${Math.round(bytes / 1000)} KB`;
}

function pluginLabel(id: PluginId): string {
  return PLUGIN_DESCRIPTORS.find((d) => d.id === id)?.label ?? id;
}

export default function RunGate({
  gate,
  title,
  description,
  label,
  engine = true,
  className,
  compact = false,
}: RunGateProps): React.ReactElement {
  const mgr = usePluginManager();
  const booting = gate.armed && gate.status === "booting";
  const error = gate.status === "error";
  // Engine already running (activated by a sibling lab or the navbar widget):
  // pressing Run attaches instantly — no engine download this time.
  const warm = engine && !gate.armed && mgr.state.engine.phase === "ready";

  // Engine download fraction (the ~13 MB module), when the server reports a length.
  const bp = gate.bootProgress;
  const engineFrac = bp && bp.total ? Math.min(1, bp.loaded / bp.total) : null;

  // Any required plugin still downloading, for a secondary progress line.
  const downloading = gate.requires
    .map((id) => ({ id, st: mgr.state.plugins[id] }))
    .filter((p) => p.st?.phase === "downloading");

  // "~13 MB engine + PDFium (5 MB)" — what the first press will fetch.
  const parts = [
    ...(engine ? [`${ENGINE_SIZE} engine`] : []),
    ...gate.requires.map((id) => {
      const d = PLUGIN_DESCRIPTORS.find((x) => x.id === id);
      return d?.sizeBytes ? `${pluginLabel(id)} (${fmtSize(d.sizeBytes)})` : pluginLabel(id);
    }),
  ];
  const downloadNote = warm
    ? "engine already running, so it starts instantly"
    : parts.length > 0
      ? parts.join(" + ")
      : "first download on demand";

  if (compact) {
    return (
      <div
        className={cn(
          "kapi-reference flex flex-wrap items-center gap-x-3 gap-y-1.5 rounded-md border bg-card/60 px-3 py-2 text-foreground",
          className,
        )}
      >
        {!gate.armed && (
          <>
            <Button type="button" size="sm" onClick={gate.run} className="gap-1.5 rounded-full">
              <Play className="size-3.5 fill-current" strokeWidth={0} aria-hidden="true" />
              {label ?? "Run in your browser"}
            </Button>
            <p className="m-0 text-xs text-muted-foreground">
              {downloadNote} · runs locally, nothing leaves your machine
            </p>
          </>
        )}

        {booting && (
          <div
            className="flex items-center gap-2 text-xs text-muted-foreground"
            role="status"
            aria-live="polite"
          >
            <span
              aria-hidden="true"
              className="size-3 animate-spin rounded-full border-2 border-muted-foreground/30 border-t-foreground motion-reduce:animate-none"
            />
            {engineFrac !== null ? "Downloading the engine…" : "Starting the engine…"}
            <span className="h-1 w-24 overflow-hidden rounded-full bg-secondary">
              <span
                className={cn(
                  "block h-full rounded-full bg-primary transition-[width] duration-200 motion-reduce:transition-none",
                  engineFrac === null && "animate-pulse motion-reduce:animate-none",
                )}
                style={{
                  width: engineFrac !== null ? `${Math.round(engineFrac * 100)}%` : "40%",
                }}
              />
            </span>
          </div>
        )}

        {error && (
          <div className="flex items-center gap-2">
            <p className="m-0 text-xs text-destructive">Failed to start: {gate.error}</p>
            <Button type="button" variant="outline" size="sm" onClick={gate.run}>
              Retry
            </Button>
          </div>
        )}
      </div>
    );
  }

  return (
    <div
      className={cn(
        "kapi-reference flex flex-col items-center justify-center gap-2.5 rounded-lg border bg-card/60 p-8 text-center text-foreground",
        className ?? "min-h-[180px]",
      )}
    >
      {title && <div className="text-base font-semibold">{title}</div>}
      {description && <p className="max-w-md text-sm text-muted-foreground">{description}</p>}

      {!gate.armed && (
        <>
          <Button
            type="button"
            size="lg"
            onClick={gate.run}
            className="mt-1 gap-2 rounded-full px-6 shadow-md transition-transform duration-150 hover:scale-[1.03] active:scale-95 motion-reduce:transition-none motion-reduce:hover:scale-100"
          >
            <Play className="size-4 fill-current" strokeWidth={0} aria-hidden="true" />
            {label ?? "Run in your browser"}
          </Button>
          <p className="text-xs text-muted-foreground">
            {downloadNote} · runs locally, nothing leaves your machine
          </p>
        </>
      )}

      {booting && (
        <div
          className="flex w-full max-w-xs flex-col items-center gap-2"
          role="status"
          aria-live="polite"
        >
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <span
              aria-hidden="true"
              className="size-3.5 animate-spin rounded-full border-2 border-muted-foreground/30 border-t-foreground motion-reduce:animate-none"
            />
            {engineFrac !== null ? "Downloading the engine…" : "Starting the engine…"}
          </div>
          <div className="h-1.5 w-full overflow-hidden rounded-full bg-secondary">
            <div
              className={cn(
                "h-full rounded-full bg-primary transition-[width] duration-200 motion-reduce:transition-none",
                engineFrac === null && "animate-pulse motion-reduce:animate-none",
              )}
              style={{
                width: engineFrac !== null ? `${Math.round(engineFrac * 100)}%` : "40%",
              }}
            />
          </div>
          {downloading.map((p) => {
            const pr = p.st.progress;
            const frac = pr?.frac ?? (pr?.total ? (pr.loaded ?? 0) / pr.total : 0);
            const pct = Math.round(Math.min(1, Math.max(0, frac)) * 100);
            return (
              <div key={p.id} className="w-full">
                <div className="h-1.5 w-full overflow-hidden rounded-full bg-secondary">
                  <div
                    className="h-full rounded-full bg-primary transition-[width] duration-200 motion-reduce:transition-none"
                    style={{ width: `${pct}%` }}
                  />
                </div>
                <div className="mt-0.5 text-xs text-muted-foreground">
                  Downloading {pluginLabel(p.id)} · {pct}%
                  {pr?.total ? ` · ${fmtSize(pr.loaded)} of ${fmtSize(pr.total)}` : ""}
                </div>
              </div>
            );
          })}
        </div>
      )}

      {error && (
        <div className="flex flex-col items-center gap-2">
          <p className="text-sm text-destructive">Failed to start: {gate.error}</p>
          <Button type="button" variant="outline" size="sm" onClick={gate.run}>
            Retry
          </Button>
        </div>
      )}
    </div>
  );
}

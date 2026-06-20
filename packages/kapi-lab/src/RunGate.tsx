import React from "react";
import { PLUGIN_DESCRIPTORS, usePluginManager } from "@neokapi/kapi-playground/plugins";
import type { PluginId } from "@neokapi/kapi-playground/plugins";
import type { RunGate as RunGateState } from "./useRunGate";

// RunGate — the idle/booting/error surface shown until a lab is `ready`.
// Nothing in the lab boots, downloads, or runs until the user presses Run, so
// page load stays inert (matches the desktop/CLI "explicit run" model).

export interface RunGateProps {
  gate: RunGateState;
  /** Heading, e.g. "Core Framework lab". */
  title?: string;
  /** One-line description of what Run will do. */
  description?: string;
  /** Run button label (default "Run"). */
  label?: string;
}

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
}: RunGateProps): React.ReactElement {
  const mgr = usePluginManager();
  const booting = gate.armed && gate.status === "booting";
  const error = gate.status === "error";

  // Engine download fraction (≈13 MB module), when the server reports a length.
  const bp = gate.bootProgress;
  const engineFrac = bp && bp.total ? Math.min(1, bp.loaded / bp.total) : null;

  // Any required plugin still downloading, for a secondary progress line.
  const downloading = gate.requires
    .map((id) => ({ id, st: mgr.state.plugins[id] }))
    .filter((p) => p.st?.phase === "downloading");

  const requiresNote =
    gate.requires.length > 0
      ? gate.requires
          .map((id) => {
            const d = PLUGIN_DESCRIPTORS.find((x) => x.id === id);
            return d?.sizeBytes ? `${pluginLabel(id)} (${fmtSize(d.sizeBytes)})` : pluginLabel(id);
          })
          .join(", ")
      : "";

  return (
    <div className="kapi-reference flex min-h-[180px] flex-col items-center justify-center gap-3 rounded-lg border border-dashed bg-card/40 p-8 text-center text-foreground">
      {title && <div className="text-base font-semibold">{title}</div>}
      {description && <p className="max-w-md text-sm text-muted-foreground">{description}</p>}

      {!gate.armed && (
        <>
          <button
            type="button"
            onClick={gate.run}
            className="rounded-md bg-primary px-5 py-2 text-sm font-medium text-primary-foreground hover:opacity-90"
          >
            ▶ {label ?? "Run"}
          </button>
          {requiresNote && (
            <p className="text-xs text-muted-foreground">
              First run downloads: {requiresNote}. Nothing is fetched until you press Run.
            </p>
          )}
        </>
      )}

      {booting && (
        <div className="flex w-full max-w-xs flex-col items-center gap-2">
          <div className="text-sm text-muted-foreground">
            {engineFrac !== null ? "Downloading engine…" : "Starting engine…"}
          </div>
          <div className="h-1.5 w-full overflow-hidden rounded-full bg-secondary">
            <div
              className="h-full rounded-full bg-primary transition-[width] duration-200"
              style={{ width: engineFrac !== null ? `${Math.round(engineFrac * 100)}%` : "40%" }}
            />
          </div>
          {downloading.map((p) => (
            <div key={p.id} className="w-full text-xs text-muted-foreground">
              Downloading {pluginLabel(p.id)}…{" "}
              {p.st.progress?.frac != null ? `${Math.round(p.st.progress.frac * 100)}%` : ""}
            </div>
          ))}
        </div>
      )}

      {error && (
        <div className="flex flex-col items-center gap-2">
          <p className="text-sm text-destructive">Failed to start: {gate.error}</p>
          <button
            type="button"
            onClick={gate.run}
            className="rounded-md border px-3 py-1.5 text-sm hover:bg-muted/60"
          >
            Retry
          </button>
        </div>
      )}
    </div>
  );
}

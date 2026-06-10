// FlowLegend — a collapsible canvas overlay explaining the port-type family
// colors and the transformer badge, so the typed chips on nodes and edges are
// self-documenting.

import { useState } from "react";
import { cn } from "@neokapi/ui-primitives";
import { Info, X, Layers } from "lucide-react";
import { PORT_FAMILIES, type PortFamily } from "./portTypes";

const TRANSFORMER_COLOR = "oklch(0.68 0.16 250)";

const FAMILY_ORDER: PortFamily[] = [
  "content",
  "linguistic",
  "quality",
  "suggestion",
  "metric",
  "note",
  "security",
];

export function FlowLegend() {
  const [open, setOpen] = useState(false);

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        className={cn(
          "flex items-center gap-1 rounded-md border border-border bg-card/90 px-2 py-1",
          "text-[10px] font-medium text-muted-foreground shadow-sm",
          "hover:text-foreground hover:border-foreground/30 transition-colors",
        )}
        title="What do the colors mean?"
      >
        <Info size={11} />
        Legend
      </button>
    );
  }

  return (
    <div className="w-56 rounded-md border border-border bg-card/95 shadow-md backdrop-blur-sm">
      <div className="flex items-center justify-between border-b border-border px-2.5 py-1.5">
        <span className="text-[11px] font-semibold">Data types</span>
        <button
          type="button"
          onClick={() => setOpen(false)}
          className="text-muted-foreground hover:text-foreground"
          aria-label="Close legend"
        >
          <X size={12} />
        </button>
      </div>
      <div className="px-2.5 py-2 space-y-1.5">
        {FAMILY_ORDER.map((fam) => {
          const f = PORT_FAMILIES[fam];
          return (
            <div key={fam} className="flex items-start gap-1.5">
              <span
                className="mt-0.5 size-2.5 shrink-0 rounded-sm"
                style={{ background: f.color }}
                aria-hidden
              />
              <div className="leading-tight">
                <div className="text-[10px] font-medium text-foreground">{f.label}</div>
                <div className="text-[9px] text-muted-foreground">{f.description}</div>
              </div>
            </div>
          );
        })}
      </div>
      <div className="border-t border-border px-2.5 py-2">
        <div
          className="flex items-center gap-1 text-[10px] font-medium"
          style={{ color: TRANSFORMER_COLOR }}
        >
          <Layers size={10} />
          Transformers
        </div>
        <p className="mt-0.5 text-[9px] text-muted-foreground leading-tight">
          Tools (e.g. redact) that rewrite the source. They run as ordinary ordered steps; the
          placement pass flags an unsafe position.
        </p>
      </div>
    </div>
  );
}

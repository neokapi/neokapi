import { useState, type ReactNode } from "react";
import { ChevronRight } from "lucide-react";
import { Card, CardContent, Collapsible, CollapsibleContent } from "@neokapi/ui-primitives";

/**
 * One layer of the review model, headed by the line a reviewer scans.
 *
 * The five layers together are the whole of what the model was told, and a
 * reviewer deciding on a unit reads all five. Read as five open cards they are
 * a page of detail with the answer buried somewhere in it, so each one leads
 * with its own verdict: the coordinates that govern the unit, how many blocks
 * sit around it, what was approved before, what the checks said, where the
 * wording came from. The detail sits under the summary and folds away.
 *
 * The summary is always drawn, open or closed, so folding a layer never hides
 * that the layer exists or what it concluded.
 */
export interface LayerCardProps {
  /** The layer's name, e.g. "Provenance". */
  title: string;
  /** The line a reviewer reads at a glance. Drawn whether open or closed. */
  summary?: ReactNode;
  /** A mark for the layer, drawn beside the title. */
  icon?: ReactNode;
  /** Start folded. Layers default to open: the reviewer sees the whole model. */
  defaultOpen?: boolean;
  /** Test/query hook, following the `data-slot` convention on the primitives. */
  dataSlot?: string;
  /** Accessible name for the fold control, e.g. "Provenance details". */
  toggleLabel?: string;
  children: ReactNode;
}

export function LayerCard({
  title,
  summary,
  icon,
  defaultOpen = true,
  dataSlot,
  toggleLabel,
  children,
}: LayerCardProps) {
  const [open, setOpen] = useState(defaultOpen);

  return (
    <Card data-slot={dataSlot} data-open={open || undefined}>
      <CardContent className="p-0">
        <button
          type="button"
          onClick={() => setOpen((v) => !v)}
          aria-expanded={open}
          aria-label={toggleLabel ?? title}
          className="flex w-full items-start gap-2 rounded-t-lg px-3 py-2 text-left transition-colors hover:bg-accent/40"
          data-slot={dataSlot ? `${dataSlot}-toggle` : undefined}
        >
          <ChevronRight
            size={12}
            className={`mt-0.5 shrink-0 text-muted-foreground transition-transform ${open ? "rotate-90" : ""}`}
            aria-hidden
          />
          {icon}
          <span className="shrink-0 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
            {title}
          </span>
          <span
            className="flex min-w-0 flex-1 flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground"
            data-slot={dataSlot ? `${dataSlot}-summary` : undefined}
          >
            {summary}
          </span>
        </button>
        <Collapsible open={open}>
          <CollapsibleContent>
            <div className="border-t px-3 pb-3 pt-2 text-xs">{children}</div>
          </CollapsibleContent>
        </Collapsible>
      </CardContent>
    </Card>
  );
}

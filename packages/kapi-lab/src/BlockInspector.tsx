import React, { useState } from "react";
import { ChevronDown, ChevronRight, Sparkles } from "lucide-react";
import {
  Badge,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  Separator,
  cn,
} from "@neokapi/ui-primitives";
import RunSequence from "./RunSequence";
import type { AnnotationView, ContentNode, OverlayView, Run, TargetMeta } from "./types";

export interface BlockInspectorProps {
  node: ContentNode;
  /** Expanded on first render (default false). */
  defaultOpen?: boolean;
  /** Flag the block as changed by a pipeline run (ring + badge). */
  changed?: boolean;
  className?: string;
}

const STATUS_TONE: Record<string, string> = {
  "": "text-muted-foreground",
  draft: "text-amber-600 dark:text-amber-400",
  translated: "text-sky-600 dark:text-sky-400",
  reviewed: "text-violet-600 dark:text-violet-400",
  "signed-off": "text-emerald-600 dark:text-emerald-400",
};

const OVERLAY_TONE: Record<string, string> = {
  segmentation: "#64748b",
  term: "#0f766e",
  entity: "#7c3aed",
  qa: "#dc2626",
  alignment: "#2563eb",
};

function runsText(runs: Run[] | undefined): string {
  if (!runs) return "";
  return runs.map((r) => (r.text !== undefined ? r.text : "")).join("");
}

function wordCount(runs: Run[] | undefined): number {
  const t = runsText(runs).trim();
  return t ? t.split(/\s+/).length : 0;
}

// BlockInspector shows a translatable Block in full: its source run sequence,
// every committed target variant with lifecycle status and provenance, the
// stand-off overlays interpreting it (segmentation, terms, entities, QA,
// alignment), block-level annotations, properties and flags. Collapsed it reads
// as a compact one-liner; expanded it reveals everything the content model
// carries on the part — so a learner can see what a pipeline produced.
export default function BlockInspector({
  node,
  defaultOpen = false,
  changed,
  className,
}: BlockInspectorProps): React.ReactElement {
  const [open, setOpen] = useState(defaultOpen);

  const targets = node.targets ?? {};
  const targetKeys = Object.keys(targets);
  const overlays = node.overlays ?? [];
  const annotations = node.annotations ?? [];
  const props = node.properties ?? {};
  const propKeys = Object.keys(props);

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className={cn(
        "rounded-lg border bg-card transition-shadow",
        changed && "ring-2 ring-warning/60",
        className,
      )}
    >
      <CollapsibleTrigger className="flex w-full items-center gap-2 px-2.5 py-1.5 text-left text-sm">
        {open ? (
          <ChevronDown className="size-3.5 text-muted-foreground" />
        ) : (
          <ChevronRight className="size-3.5 text-muted-foreground" />
        )}
        <Badge className="bg-[#3b82f6] text-white">block</Badge>
        <span className="font-mono text-xs text-foreground/80">{node.id}</span>
        {node.type && <span className="text-[0.7rem] text-muted-foreground">{node.type}</span>}
        {!open && (
          <span className="truncate text-muted-foreground">
            {runsText(node.source) || "(structure)"}
          </span>
        )}
        <span className="ml-auto flex items-center gap-1.5">
          {changed && (
            <Badge variant="outline" className="gap-1 border-warning/50 text-warning">
              <Sparkles className="size-3" /> updated
            </Badge>
          )}
          {!node.translatable && (
            <Badge variant="ghost" className="text-muted-foreground">
              non-translatable
            </Badge>
          )}
          <span className="text-[0.7rem] text-muted-foreground">
            {node.source?.length ?? 0} runs · {wordCount(node.source)} words
          </span>
        </span>
      </CollapsibleTrigger>

      <CollapsibleContent>
        <Separator />
        <div className="flex flex-col gap-3 px-3 py-2.5 text-sm">
          <Field label={`source${node.sourceLocale ? ` · ${node.sourceLocale}` : ""}`}>
            <RunSequence runs={node.source ?? []} segments={node.segments} />
          </Field>

          {targetKeys.length > 0 && (
            <div className="flex flex-col gap-2">
              {targetKeys.map((key) => (
                <TargetRow
                  key={key}
                  variant={key}
                  runs={targets[key]}
                  meta={node.targetMeta?.[key]}
                />
              ))}
            </div>
          )}

          {overlays.length > 0 && (
            <Field label="overlays">
              <div className="flex flex-col gap-1.5">
                {overlays.map((o, i) => (
                  <OverlayRow key={i} overlay={o} />
                ))}
              </div>
            </Field>
          )}

          {annotations.length > 0 && (
            <Field label="annotations">
              <div className="flex flex-col gap-1.5">
                {annotations.map((a, i) => (
                  <AnnotationRow key={i} annotation={a} />
                ))}
              </div>
            </Field>
          )}

          {propKeys.length > 0 && (
            <Field label="properties">
              <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 font-mono text-xs">
                {propKeys.map((k) => (
                  <React.Fragment key={k}>
                    <dt className="text-muted-foreground">{k}</dt>
                    <dd className="truncate">{props[k]}</dd>
                  </React.Fragment>
                ))}
              </dl>
            </Field>
          )}

          {(node.hasSkeleton || node.isReferent || node.preserveWhitespace || node.identity) && (
            <div className="flex flex-wrap items-center gap-1.5">
              {node.hasSkeleton && <Badge variant="secondary">skeleton</Badge>}
              {node.isReferent && <Badge variant="secondary">referent</Badge>}
              {node.preserveWhitespace && <Badge variant="secondary">preserve-whitespace</Badge>}
              {node.identity && (
                <Badge variant="ghost" className="font-mono text-muted-foreground">
                  #{node.identity.slice(0, 10)}
                </Badge>
              )}
            </div>
          )}
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}): React.ReactElement {
  return (
    <div className="flex flex-col gap-1">
      <span className="text-[0.65rem] font-semibold uppercase tracking-wide text-muted-foreground">
        {label}
      </span>
      {children}
    </div>
  );
}

function TargetRow({
  variant,
  runs,
  meta,
}: {
  variant: string;
  runs: Run[];
  meta?: TargetMeta;
}): React.ReactElement {
  const status = meta?.status ?? "";
  return (
    <div className="flex flex-col gap-1 rounded-md bg-muted/40 px-2 py-1.5">
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <Badge variant="outline" className="font-mono">
          {variant}
        </Badge>
        <span className={cn("font-medium", STATUS_TONE[status] ?? "text-muted-foreground")}>
          {status || "new"}
        </span>
        {typeof meta?.score === "number" && meta.score > 0 && (
          <span className="text-muted-foreground">score {meta.score.toFixed(2)}</span>
        )}
        {meta?.origin?.kind && (
          <span className="text-muted-foreground">
            via {meta.origin.kind}
            {meta.origin.engine ? ` · ${meta.origin.engine}` : ""}
            {meta.origin.tool ? ` · ${meta.origin.tool}` : ""}
          </span>
        )}
      </div>
      <RunSequence runs={runs} />
    </div>
  );
}

function OverlayRow({ overlay }: { overlay: OverlayView }): React.ReactElement {
  const tone = OVERLAY_TONE[overlay.type] ?? "#64748b";
  return (
    <div className="rounded-md border bg-background/60 px-2 py-1.5">
      <div className="flex flex-wrap items-center gap-2 text-xs">
        <Badge style={{ backgroundColor: tone }} className="text-white">
          {overlay.type}
        </Badge>
        <span className="text-muted-foreground">{overlay.side}</span>
        {overlay.layer && (
          <span className="font-mono text-muted-foreground">layer:{overlay.layer}</span>
        )}
        <span className="text-muted-foreground">
          {overlay.spans.length} span{overlay.spans.length === 1 ? "" : "s"}
        </span>
      </div>
      {overlay.spans.length > 0 && (
        <ul className="mt-1 flex flex-col gap-0.5 font-mono text-xs">
          {overlay.spans.map((s, i) => (
            <li key={i} className="flex flex-wrap items-baseline gap-x-2">
              {s.id && <span className="text-muted-foreground">{s.id}</span>}
              <span className="text-muted-foreground/70">
                [{s.range.startRun}:{s.range.endRun}]
              </span>
              {s.text && <span className="text-foreground/90">“{s.text}”</span>}
              {s.ignorable && (
                <Badge variant="ghost" className="text-muted-foreground">
                  ignorable
                </Badge>
              )}
              {s.props &&
                Object.entries(s.props)
                  .filter(([k]) => k !== "ignorable")
                  .map(([k, v]) => (
                    <span key={k} className="text-muted-foreground">
                      {k}={v}
                    </span>
                  ))}
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function AnnotationRow({ annotation }: { annotation: AnnotationView }): React.ReactElement {
  return (
    <div className="rounded-md border bg-background/60 px-2 py-1.5 text-xs">
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant="secondary">{annotation.type}</Badge>
        {annotation.summary && <span className="text-foreground/90">{annotation.summary}</span>}
      </div>
      {annotation.fields && Object.keys(annotation.fields).length > 0 && (
        <dl className="mt-1 grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 font-mono text-muted-foreground">
          {Object.entries(annotation.fields).map(([k, v]) => (
            <React.Fragment key={k}>
              <dt>{k}</dt>
              <dd className="truncate text-foreground/80">{formatField(v)}</dd>
            </React.Fragment>
          ))}
        </dl>
      )}
    </div>
  );
}

function formatField(v: unknown): string {
  if (v === null || v === undefined) return "";
  if (typeof v === "string") return v;
  return JSON.stringify(v);
}

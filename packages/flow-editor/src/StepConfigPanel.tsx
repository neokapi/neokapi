import { useState, useCallback, useEffect, useRef } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { X, GitBranch } from "lucide-react";
import {
  cn,
  SchemaForm,
  Button,
  Badge,
  Markdown,
  ScrollArea,
  SimpleTooltip,
} from "@neokapi/ui-primitives";
import { IoContract } from "./nodes/PortChip";
import { getCategoryStyle } from "./category";
import { createDebouncedSync, type DebouncedSync } from "./debouncedSync";
import { type PlacementDiagnostic } from "./placement";
import type { ToolInfo, ComponentSchema, ToolDoc, ToolDocParam } from "./types";

export interface StepConfigPanelProps {
  step: { tool: string };
  toolInfo: ToolInfo | null | undefined;
  schema: ComponentSchema | null | undefined;
  doc: ToolDoc | null | undefined;
  config: Record<string, unknown>;
  /**
   * Project-level preset for this tool (the recipe's defaults.tools entry).
   * The engine merges it under the step's own config — the step wins per key —
   * so the panel shows the inherited values and flags the overridden ones.
   */
  preset?: Record<string, unknown>;
  /** Required input ports nothing upstream produces (requirement analysis). */
  unmet?: string[];
  /** Transformer placement diagnostics for this step (AD-006 placement pass). */
  placement?: PlacementDiagnostic[];
  onConfigChange: (config: Record<string, unknown>) => void;
  onClose: () => void;
  onRemove?: () => void;
  /** Present the configuration without inviting edits (a read-only flow or the diagram view). */
  readOnly?: boolean;
  /** Open the host's project-defaults editor for this tool (see FlowEditorProps.onEditPresets). */
  onEditPresets?: () => void;
}

// Exported for colocated tests.
export function StepConfigPanel({
  step,
  toolInfo,
  schema,
  doc,
  config,
  preset,
  unmet,
  placement,
  onConfigChange,
  onClose,
  onRemove,
  readOnly,
  onEditPresets,
}: StepConfigPanelProps) {
  const [showDocs, setShowDocs] = useState(false);
  const category = toolInfo?.category || "pipeline";
  const catStyle = getCategoryStyle(category);
  const Icon = catStyle.icon;
  const displayName = toolInfo?.display_name || step.tool;

  // Local config state -- owns the values to prevent parent re-renders from
  // resetting inputs. Syncs to parent via a debounced controller.
  const [localConfig, setLocalConfig] = useState(config);

  // Keep the controller's emit target pointing at the latest onConfigChange
  // without re-creating the controller (which would drop a pending timer).
  const onConfigChangeRef = useRef(onConfigChange);
  onConfigChangeRef.current = onConfigChange;
  const syncRef = useRef<DebouncedSync<Record<string, unknown>>>(undefined);
  if (!syncRef.current) {
    syncRef.current = createDebouncedSync((cfg) => onConfigChangeRef.current(cfg), 300);
  }

  // Re-initialize when the selected tool changes (not on every config update).
  // (The panel is also keyed on the selected node id, so it normally remounts.)
  const toolRef = useRef(step.tool);
  if (step.tool !== toolRef.current) {
    toolRef.current = step.tool;
    setLocalConfig(config);
  }

  const handleLocalChange = useCallback((newConfig: Record<string, unknown>) => {
    setLocalConfig(newConfig);
    syncRef.current!.schedule(newConfig);
  }, []);

  // Flush any pending debounced edit on unmount / close so the last sub-300ms
  // edit is not dropped. With `key={selectedNodeId}` on the panel, switching
  // selection remounts it, which flushes the previous selection's pending edit.
  useEffect(() => {
    const sync = syncRef.current!;
    return () => sync.flush();
  }, []);

  return (
    <div
      className="flex h-full flex-col border-l border-border bg-background overflow-hidden"
      style={{ width: "min(320px, calc(100vw - 2rem))" }}
    >
      {/* Header */}
      <div className="px-3 py-2.5 border-b border-border flex flex-col gap-1.5">
        <div className="flex items-center gap-1.5">
          <div className="w-[3px] h-5 rounded-sm shrink-0" style={{ background: catStyle.color }} />
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-1 mb-0.5">
              <Icon size={11} style={{ color: catStyle.text }} />
              <span
                className="text-[9px] font-bold tracking-wide uppercase"
                style={{ color: catStyle.text }}
              >
                {catStyle.label}
              </span>
            </div>
            <div className="text-sm font-semibold text-foreground">{displayName}</div>
          </div>
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={onClose}
            className="self-start"
            aria-label="Close panel"
          >
            <X size={14} className="text-muted-foreground" />
          </Button>
        </div>

        {/* Description -- prefer doc overview, fall back to ToolInfo.description */}
        {(doc?.overview || toolInfo?.description) && (
          <div
            className={cn(
              "text-[11px] text-muted-foreground leading-relaxed",
              !showDocs && "line-clamp-3",
            )}
          >
            {doc?.overview || toolInfo?.description}
          </div>
        )}

        {/* IO contract info */}
        {(toolInfo?.cardinality ||
          toolInfo?.produces?.length ||
          toolInfo?.side_effects?.length) && (
          <div className="flex gap-1 flex-wrap items-center">
            {toolInfo.cardinality && (
              <span
                className="rounded px-1.5 py-0 h-4 inline-flex items-center text-[9px] font-mono font-semibold"
                style={{
                  background:
                    toolInfo.cardinality === "bilingual"
                      ? "oklch(0.55 0.15 250 / 0.1)"
                      : toolInfo.cardinality === "multilingual"
                        ? "oklch(0.55 0.15 320 / 0.1)"
                        : "oklch(0.5 0.02 0 / 0.06)",
                  color:
                    toolInfo.cardinality === "bilingual"
                      ? "oklch(0.55 0.15 250)"
                      : toolInfo.cardinality === "multilingual"
                        ? "oklch(0.55 0.15 320)"
                        : "var(--muted-foreground)",
                }}
              >
                {toolInfo.cardinality}
              </span>
            )}
            {toolInfo.default_locale && (
              <span
                className="rounded px-1.5 py-0 h-4 inline-flex items-center text-[9px] font-mono"
                style={{
                  background: "oklch(0.6 0.12 290 / 0.1)",
                  color: "oklch(0.55 0.12 290)",
                }}
              >
                default: {toolInfo.default_locale}
              </span>
            )}
            {toolInfo.side_effects?.map((se) => (
              <span
                key={se}
                className="rounded px-1.5 py-0 h-4 inline-flex items-center text-[9px]"
                style={{
                  background: "oklch(0.65 0.12 85 / 0.1)",
                  color: "oklch(0.55 0.12 85)",
                }}
              >
                ⚡ {se}
              </span>
            ))}
          </div>
        )}

        {/* Typed IO contract: what this tool reads → writes */}
        {((toolInfo?.consumes && toolInfo.consumes.length > 0) ||
          (toolInfo?.produces && toolInfo.produces.length > 0)) && (
          <div className="flex items-center gap-1.5">
            <span className="text-[9px] uppercase tracking-wide text-muted-foreground">IO</span>
            <IoContract
              consumes={toolInfo?.consumes}
              produces={toolInfo?.produces}
              max={8}
              showLabels
            />
          </div>
        )}

        {/* Requirements badges + docs toggle */}
        <div className="flex gap-1 flex-wrap items-center">
          {toolInfo?.requires?.map((req) => (
            <Badge key={req} variant="secondary" className="text-[9px] px-1.5 py-0 h-4">
              {req}
            </Badge>
          ))}
          {doc && (
            <Button
              variant={showDocs ? "outline" : "ghost"}
              size="xs"
              onClick={() => setShowDocs((v) => !v)}
              className={cn("ml-auto text-[9px] h-5 px-2", showDocs && "border-ring text-ring")}
            >
              {showDocs ? t("Hide Docs") : t("Docs")}
            </Button>
          )}
          {doc?.wikiUrl && (
            <SimpleTooltip content="Open wiki documentation">
              <a
                href={doc.wikiUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="text-[9px] text-muted-foreground no-underline px-1"
              >
                Wiki ↗
              </a>
            </SimpleTooltip>
          )}
        </div>

        {/* Unmet-requirement guidance: a required input nothing upstream produces. */}
        {unmet && unmet.length > 0 && (
          <div
            className="mt-2 rounded-md border px-2 py-1.5 text-[10px] leading-snug"
            style={{
              borderColor: "oklch(0.62 0.17 45 / 0.5)",
              background: "oklch(0.62 0.17 45 / 0.08)",
              color: "oklch(0.5 0.15 45)",
            }}
          >
            <span className="font-semibold">Missing input: </span>
            this tool needs <span className="font-mono">{unmet.join(", ")}</span>, but nothing
            earlier in the flow produces {unmet.length > 1 ? "them" : "it"}. Add a tool that
            produces {unmet.length > 1 ? "these" : "it"} before this step.
          </div>
        )}

        {/* Project preset (defaults.tools): inherited defaults the engine
            merges under this step's config — the step wins per key. */}
        {preset && Object.keys(preset).length > 0 && (
          <div
            className="mt-2 rounded-md border px-2 py-1.5 text-[10px] leading-snug"
            style={{
              borderColor: "oklch(0.6 0.12 290 / 0.5)",
              background: "oklch(0.6 0.12 290 / 0.08)",
              color: "oklch(0.5 0.12 290)",
            }}
          >
            <span className="font-semibold">Project preset: </span>
            this tool inherits defaults from the project recipe (
            <span className="font-mono">defaults.tools</span>). Values set here override them per
            key.
            <div className="mt-1 flex flex-col gap-0.5">
              {Object.entries(preset).map(([k, v]) => {
                const overridden = config[k] !== undefined;
                return (
                  <div
                    key={k}
                    className={cn("font-mono text-[9px]", overridden && "line-through opacity-60")}
                  >
                    {k}: {typeof v === "string" ? v : JSON.stringify(v)}
                    {overridden && (
                      <span className="ml-1 not-italic no-underline">(overridden)</span>
                    )}
                  </div>
                );
              })}
            </div>
            {onEditPresets && (
              <button
                type="button"
                className="mt-1 cursor-pointer font-semibold underline underline-offset-2"
                onClick={onEditPresets}
              >
                Edit project defaults
              </button>
            )}
          </div>
        )}

        {/* Transformer placement diagnostics (AD-006): the same errors the build
            gate rejects with, surfaced while composing. */}
        {placement?.map((d, i) => (
          <div
            key={`${d.rule}-${i}`}
            className="mt-2 rounded-md border px-2 py-1.5 text-[10px] leading-snug"
            style={
              d.severity === "error"
                ? {
                    borderColor: "oklch(0.55 0.2 25 / 0.5)",
                    background: "oklch(0.55 0.2 25 / 0.08)",
                    color: "oklch(0.45 0.18 25)",
                  }
                : {
                    borderColor: "oklch(0.62 0.17 45 / 0.5)",
                    background: "oklch(0.62 0.17 45 / 0.08)",
                    color: "oklch(0.5 0.15 45)",
                  }
            }
          >
            <span className="font-semibold">
              {d.severity === "error" ? t("Placement error: ") : t("Placement: ")}
            </span>
            {d.message}
          </div>
        ))}
      </div>

      {/* Docs panel (collapsible) */}
      {showDocs && doc && (
        <ScrollArea className="max-h-[260px] border-b border-border text-[11px] leading-relaxed">
          <div className="px-3 py-2">
            <DocsSidebar doc={doc} />
          </div>
        </ScrollArea>
      )}

      {/* Config form */}
      <ScrollArea className="flex-1">
        <div className="px-3 py-2">
          {schema ? (
            <SchemaForm
              schema={schema}
              values={localConfig}
              onChange={handleLocalChange}
              compact
              hideHeader
              readOnly={readOnly}
              paramDocs={doc?.parameters}
            />
          ) : (
            <div className="text-[11px] text-muted-foreground text-center py-5 italic">
              {toolInfo?.has_schema
                ? t("Loading configuration...")
                : t("No configurable parameters")}
            </div>
          )}
        </div>
      </ScrollArea>

      {/* Footer */}
      {onRemove && (
        <div className="px-3 py-2 border-t border-border">
          <Button
            variant="destructive"
            size="sm"
            className="w-full"
            onClick={onRemove}
            aria-label="Remove tool from flow"
          >
            Remove from flow
          </Button>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Inline documentation sidebar for the config panel
// ---------------------------------------------------------------------------

function DocsSidebar({ doc }: { doc: ToolDoc }) {
  const params = doc.parameters ? Object.entries(doc.parameters) : [];
  const hasExamples = doc.examples && doc.examples.length > 0;
  const hasLimitations = doc.limitations && doc.limitations.length > 0;
  const hasNotes = doc.processingNotes && doc.processingNotes.length > 0;

  return (
    <div className="flex flex-col gap-2.5">
      {/* Parameters */}
      {params.length > 0 && (
        <DocSection title="Parameters">
          <div className="flex flex-col gap-1.5">
            {params.map(([key, p]) => (
              <DocParamRow key={key} name={key} param={p} />
            ))}
          </div>
        </DocSection>
      )}

      {/* Examples */}
      {hasExamples && (
        <DocSection title="Examples">
          {doc.examples!.map((ex, i) => (
            <div
              key={i}
              className={cn(
                "px-2 py-1.5 rounded bg-secondary",
                i < doc.examples!.length - 1 && "mb-1",
              )}
            >
              <div className="font-semibold text-[10px] text-foreground">{ex.title}</div>
              {ex.description && (
                <div className="text-[10px] text-muted-foreground mt-0.5">
                  <Markdown inline>{ex.description}</Markdown>
                </div>
              )}
              {ex.input && (
                <pre className="text-[9px] font-mono bg-background rounded-sm px-1.5 py-1 mt-1 overflow-auto max-h-[60px] whitespace-pre-wrap text-foreground">
                  {ex.input}
                </pre>
              )}
            </div>
          ))}
        </DocSection>
      )}

      {/* Limitations */}
      {hasLimitations && (
        <DocSection title="Limitations">
          {doc.limitations!.map((lim, i) => (
            <div
              key={i}
              className="text-[10px] text-muted-foreground pl-2 mb-0.5"
              style={{ borderLeft: "2px solid color-mix(in oklch, var(--ring) 30%, transparent)" }}
            >
              {lim}
            </div>
          ))}
        </DocSection>
      )}

      {/* Processing Notes */}
      {hasNotes && (
        <DocSection title="Notes">
          {doc.processingNotes!.map((note, i) => (
            <div
              key={i}
              className="text-[10px] text-muted-foreground pl-2 mb-0.5"
              style={{
                borderLeft: "2px solid color-mix(in oklch, var(--accent) 40%, transparent)",
              }}
            >
              {note}
            </div>
          ))}
        </DocSection>
      )}
    </div>
  );
}

function DocSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div>
      <div className="text-[9px] font-bold uppercase tracking-wide text-muted-foreground mb-1">
        {title}
      </div>
      {children}
    </div>
  );
}

function DocParamRow({ name, param }: { name: string; param: ToolDocParam }) {
  return (
    <div className="px-2 py-1 rounded bg-secondary">
      <div className="flex items-center gap-1 mb-0.5">
        <code
          className="text-[10px] font-semibold px-1.5 py-px rounded-sm"
          style={{
            color: "var(--ring)",
            background: "color-mix(in oklch, var(--ring) 8%, transparent)",
          }}
        >
          {name}
        </code>
        {param.introducedIn && (
          <span
            className="text-[8px] px-1 py-px rounded-sm text-muted-foreground font-medium"
            style={{ background: "color-mix(in oklch, var(--accent) 20%, transparent)" }}
          >
            {param.introducedIn}
          </span>
        )}
      </div>
      <div className="text-[10px] text-muted-foreground leading-snug">
        <Markdown inline>{param.description}</Markdown>
      </div>
      {param.notes?.map((note, i) => (
        <div key={i} className="text-[9px] text-muted-foreground mt-0.5 italic opacity-80">
          <Markdown inline>{note}</Markdown>
        </div>
      ))}
      {param.dependsOn?.map((dep, i) => (
        <div key={i} className="text-[9px] mt-0.5 flex items-center gap-0.5">
          <GitBranch size={8} className="text-muted-foreground" />
          <code className="font-semibold text-muted-foreground">{dep.property}</code>
          <span className="text-muted-foreground opacity-70">{dep.condition}</span>
        </div>
      ))}
    </div>
  );
}

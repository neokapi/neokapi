import { useState, useMemo } from "react";
import { GitBranch, Layers } from "lucide-react";
import { cn } from "@neokapi/ui-primitives";
import type { FlowSpec } from "./types";
import { FLOW_TEMPLATES, type FlowTemplate } from "./templates";
import { getCategoryStyle } from "./category";

interface FlowTemplateLibraryProps {
  onSelect: (spec: FlowSpec) => void;
  onDismiss?: () => void;
}

/**
 * Template library for flow editor — shows common flow patterns
 * that users can start from. Displayed when the flow has no steps.
 */
export function FlowTemplateLibrary({ onSelect, onDismiss }: FlowTemplateLibraryProps) {
  const [filter, setFilter] = useState<string | null>(null);

  const filtered = useMemo(
    () => (filter ? FLOW_TEMPLATES.filter((t) => t.category === filter) : FLOW_TEMPLATES),
    [filter],
  );

  const categories = useMemo(() => {
    const cats = new Set(FLOW_TEMPLATES.map((t) => t.category));
    return [...cats];
  }, []);

  return (
    <div className="mx-auto flex max-w-2xl flex-col items-center gap-5 px-6 py-8">
      <div className="text-center">
        <h2 className="m-0 text-base font-bold text-foreground">
          Start from a template
        </h2>
        <p className="mt-1.5 mb-0 text-xs text-muted-foreground">
          Choose a common flow pattern or start with an empty canvas.
        </p>
      </div>

      {/* Category filter pills */}
      <div className="flex flex-wrap justify-center gap-1.5">
        <button
          onClick={() => setFilter(null)}
          className={cn(
            "cursor-pointer rounded-xl border px-2.5 py-0.5 text-[11px] font-medium",
            filter === null
              ? "border-accent bg-accent text-accent-foreground"
              : "border-border bg-transparent text-muted-foreground",
          )}
        >
          All
        </button>
        {categories.map((cat) => {
          const catStyle = getCategoryStyle(cat);
          const active = filter === cat;
          return (
            <button
              key={cat}
              onClick={() => setFilter(active ? null : cat)}
              className={cn(
                "cursor-pointer rounded-xl border px-2.5 py-0.5 text-[11px] font-medium",
                !active && "border-border bg-transparent text-muted-foreground",
              )}
              style={
                active
                  ? { borderColor: catStyle.color, background: catStyle.bg, color: catStyle.text }
                  : undefined
              }
            >
              {catStyle.label}
            </button>
          );
        })}
      </div>

      {/* Divider */}
      <div className="w-full border-t border-dashed border-border pt-1" />

      {/* Template cards */}
      <div className="mx-auto grid w-full max-w-xl grid-cols-2 gap-2.5">
        {filtered.map((tmpl, index) => (
          <TemplateCard
            key={tmpl.id}
            template={tmpl}
            index={index}
            onSelect={() => onSelect(tmpl.spec)}
          />
        ))}
      </div>

      {/* Empty canvas option */}
      {onDismiss && (
        <button
          onClick={onDismiss}
          className="cursor-pointer rounded-md border border-border bg-transparent px-4 py-1.5 text-[11px] text-muted-foreground"
        >
          Start with empty canvas
        </button>
      )}
    </div>
  );
}

function TemplateCard({
  template,
  index,
  onSelect,
}: {
  template: FlowTemplate;
  index: number;
  onSelect: () => void;
}) {
  const catStyle = getCategoryStyle(template.category);
  const Icon = catStyle.icon;

  return (
    <button
      onClick={onSelect}
      className="flex cursor-pointer flex-col gap-2 rounded-lg border border-border bg-card p-3.5 text-left transition-[transform,border-color,box-shadow] duration-150"
      style={{
        animation: "cardReveal 0.25s ease-out forwards",
        animationDelay: `${index * 60}ms`,
        opacity: 0,
      }}
      onMouseEnter={(e) => {
        e.currentTarget.style.transform = "translateY(-2px)";
        e.currentTarget.style.boxShadow = `0 0 0 1px ${catStyle.color}, 0 4px 12px ${catStyle.color}22`;
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.transform = "translateY(0)";
        e.currentTarget.style.boxShadow = "none";
      }}
    >
      <div className="flex items-center gap-1.5">
        <Icon size={13} style={{ color: catStyle.color }} />
        <span className="text-[13px] font-semibold text-foreground">{template.name}</span>
        {template.hasParallel && (
          <span title="Includes parallel steps" className="ml-auto">
            <GitBranch size={11} className="text-accent-foreground" />
          </span>
        )}
      </div>
      <p className="m-0 text-[11px] leading-snug text-muted-foreground">
        {template.description}
      </p>
      <div className="flex items-center gap-1.5">
        <Layers size={10} className="text-muted-foreground" />
        <span className="text-[10px] text-muted-foreground">
          {template.stepCount} step{template.stepCount !== 1 ? "s" : ""}
        </span>
        <span
          className="rounded-lg px-1.5 py-px text-[9px] font-medium"
          style={{ background: catStyle.bg, color: catStyle.text }}
        >
          {catStyle.label}
        </span>
      </div>
    </button>
  );
}

import React, { useState, useMemo } from "react";
import { t } from "@neokapi/kapi-react/runtime";
import { GitBranch, Layers } from "lucide-react";
import {
  Button,
  Card,
  CardHeader,
  CardTitle,
  CardAction,
  CardDescription,
  CardFooter,
} from "@neokapi/ui-primitives";
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
    <div className="flex flex-1 flex-col gap-6 p-6">
      <div className="text-center">
        <h2 className="m-0 text-lg font-bold text-foreground">Start from a template</h2>
        <p className="mt-2 mb-0 text-sm text-muted-foreground">
          Choose a common flow pattern or start with an empty canvas.
        </p>
      </div>

      {/* Category filter pills */}
      <div className="flex flex-wrap justify-center gap-2">
        <Button
          variant={filter === null ? "default" : "outline"}
          size="sm"
          onClick={() => setFilter(null)}
          className="rounded-full"
        >
          All
        </Button>
        {categories.map((cat) => {
          const catStyle = getCategoryStyle(cat);
          const active = filter === cat;
          return (
            <Button
              key={cat}
              variant={active ? "default" : "outline"}
              size="sm"
              onClick={() => setFilter(active ? null : cat)}
              className="rounded-full"
              style={
                active
                  ? { borderColor: catStyle.color, background: catStyle.bg, color: catStyle.text }
                  : undefined
              }
            >
              {catStyle.label}
            </Button>
          );
        })}
      </div>

      {/* Divider */}
      <div className="border-t border-dashed border-border" />

      {/* Template cards — auto-fill grid adapts to available width */}
      <div
        className="grid w-full gap-3"
        style={{ gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))" }}
      >
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
        <div className="text-center">
          <Button variant="outline" size="sm" onClick={onDismiss}>
            Start with empty canvas
          </Button>
        </div>
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
    <Card
      size="sm"
      className="cursor-pointer transition-[transform,box-shadow] duration-150 hover:-translate-y-0.5"
      onClick={onSelect}
      style={{
        animation: "cardReveal 0.25s ease-out forwards",
        animationDelay: `${index * 60}ms`,
        opacity: 0,
      }}
      onMouseEnter={(e: React.MouseEvent<HTMLDivElement>) => {
        e.currentTarget.style.boxShadow = `0 0 0 1px ${catStyle.color}, 0 4px 12px ${catStyle.color}22`;
      }}
      onMouseLeave={(e: React.MouseEvent<HTMLDivElement>) => {
        e.currentTarget.style.boxShadow = "";
      }}
    >
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Icon size={14} style={{ color: catStyle.color }} />
          <span>{template.name}</span>
        </CardTitle>
        {template.hasParallel && (
          <CardAction>
            <span title="Includes parallel steps">
              <GitBranch size={12} className="text-accent-foreground" />
            </span>
          </CardAction>
        )}
        <CardDescription>{template.description}</CardDescription>
      </CardHeader>
      <CardFooter className="gap-2">
        <Layers size={11} className="text-muted-foreground" />
        <span className="text-xs text-muted-foreground">
          {template.stepCount === 1
            ? t("1 step")
            : t("{count} steps", { count: template.stepCount })}
        </span>
        <span
          className="rounded-full px-2 py-0.5 text-[10px] font-medium"
          style={{ background: catStyle.bg, color: catStyle.text }}
        >
          {catStyle.label}
        </span>
      </CardFooter>
    </Card>
  );
}

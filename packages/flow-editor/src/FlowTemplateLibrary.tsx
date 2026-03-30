import { useState, useMemo } from "react";
import { GitBranch, Layers } from "lucide-react";
import type { FlowSpec } from "./types";
import { FLOW_TEMPLATES, type FlowTemplate } from "./templates";
import { getCategoryStyle } from "./category";
import { theme } from "./theme";

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
    <div
      style={{
        display: "flex",
        flexDirection: "column",
        alignItems: "center",
        padding: "32px 24px",
        gap: 20,
        maxWidth: 640,
        margin: "0 auto",
      }}
    >
      <div style={{ textAlign: "center" }}>
        <h2 style={{ fontSize: 16, fontWeight: 700, color: theme.fg, margin: 0 }}>
          Start from a template
        </h2>
        <p style={{ fontSize: 12, color: theme.fgMuted, margin: "6px 0 0" }}>
          Choose a common flow pattern or start with an empty canvas.
        </p>
      </div>

      {/* Category filter pills */}
      <div style={{ display: "flex", gap: 6, flexWrap: "wrap", justifyContent: "center" }}>
        <button
          onClick={() => setFilter(null)}
          style={{
            padding: "3px 10px",
            borderRadius: 12,
            border: `1px solid ${filter === null ? theme.accent : theme.border}`,
            background: filter === null ? theme.accent : "transparent",
            color: filter === null ? theme.accentFg : theme.fgMuted,
            fontSize: 11,
            fontWeight: 500,
            cursor: "pointer",
          }}
        >
          All
        </button>
        {categories.map((cat) => {
          const style = getCategoryStyle(cat);
          const active = filter === cat;
          return (
            <button
              key={cat}
              onClick={() => setFilter(active ? null : cat)}
              style={{
                padding: "3px 10px",
                borderRadius: 12,
                border: `1px solid ${active ? style.color : theme.border}`,
                background: active ? style.bg : "transparent",
                color: active ? style.text : theme.fgMuted,
                fontSize: 11,
                fontWeight: 500,
                cursor: "pointer",
              }}
            >
              {style.label}
            </button>
          );
        })}
      </div>

      {/* Divider */}
      <div style={{ width: "100%", borderTop: "1px dashed var(--border)", paddingTop: 4 }} />

      {/* Template cards */}
      <div
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(2, 1fr)",
          gap: 10,
          width: "100%",
          maxWidth: 560,
          margin: "0 auto",
        }}
      >
        {filtered.map((tmpl, index) => (
          <TemplateCard key={tmpl.id} template={tmpl} index={index} onSelect={() => onSelect(tmpl.spec)} />
        ))}
      </div>

      {/* Empty canvas option */}
      {onDismiss && (
        <button
          onClick={onDismiss}
          style={{
            padding: "6px 16px",
            borderRadius: 6,
            border: `1px solid ${theme.border}`,
            background: "transparent",
            color: theme.fgMuted,
            fontSize: 11,
            cursor: "pointer",
          }}
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
      style={{
        display: "flex",
        flexDirection: "column",
        gap: 8,
        padding: 14,
        borderRadius: 8,
        border: `1px solid ${theme.border}`,
        background: theme.bgCard,
        textAlign: "left",
        cursor: "pointer",
        transition: "transform 150ms, border-color 150ms, box-shadow 150ms",
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
      <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
        <Icon size={13} style={{ color: catStyle.color }} />
        <span style={{ fontSize: 13, fontWeight: 600, color: theme.fg }}>
          {template.name}
        </span>
        {template.hasParallel && (
          <GitBranch size={11} style={{ color: theme.accent, marginLeft: "auto" }} title="Includes parallel steps" />
        )}
      </div>
      <p style={{ fontSize: 11, color: theme.fgMuted, lineHeight: 1.4, margin: 0 }}>
        {template.description}
      </p>
      <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
        <Layers size={10} style={{ color: theme.fgMuted }} />
        <span style={{ fontSize: 10, color: theme.fgMuted }}>
          {template.stepCount} step{template.stepCount !== 1 ? "s" : ""}
        </span>
        <span
          style={{
            fontSize: 9,
            padding: "1px 6px",
            borderRadius: 8,
            background: catStyle.bg,
            color: catStyle.text,
            fontWeight: 500,
          }}
        >
          {catStyle.label}
        </span>
      </div>
    </button>
  );
}

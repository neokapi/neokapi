import { useMemo } from "react";
import { t } from "@neokapi/kapi-react/runtime";
import type { SpanInfo } from "../../types/span";
import { getDefaultRegistry } from "../../vocabularies";
import { TagChipComponent } from "./TagChipComponent";
import { resolveConstraints } from "./tagConstraints";
import { cn } from "../../lib/utils";
import { X, Lock, Copy, Shuffle } from "lucide-react";

interface InlineCodeLegendProps {
  /** Source spans present in the current block. */
  spans: SpanInfo[];
  /** Close the legend panel. */
  onClose: () => void;
}

// `get` accessors defer the t() dictionary lookup to render time, so
// translations loaded after module evaluation still apply.
const categoryLabels: Record<string, string> = {
  get formatting() {
    return t("Formatting", "inline code category");
  },
  get linking() {
    return t("Links", "inline code category");
  },
  get media() {
    return t("Media", "inline code category");
  },
  get structure() {
    return t("Structure", "inline code category");
  },
  get code() {
    return t("Code & Variables", "inline code category");
  },
  get generic() {
    return t("Other", "inline code category");
  },
};

const categoryOrder = ["formatting", "linking", "media", "structure", "code", "generic"];

/**
 * Collapsible legend panel listing all inline code types in the current block,
 * grouped by vocabulary category. Shows constraint indicators and visual chip
 * previews so translators can understand what each tag means.
 */
export function InlineCodeLegend({ spans, onClose }: InlineCodeLegendProps) {
  const registry = getDefaultRegistry();

  const groups = useMemo(() => {
    const typeMap = new Map<string, SpanInfo>();
    for (const span of spans) {
      if (!typeMap.has(span.type)) {
        typeMap.set(span.type, span);
      }
    }

    const catMap = new Map<string, Array<{ typeName: string; span: SpanInfo }>>();
    for (const [typeName, span] of typeMap) {
      const info = registry.lookupOrFallback(typeName);
      const cat = info.category;
      if (!catMap.has(cat)) catMap.set(cat, []);
      catMap.get(cat)!.push({ typeName, span });
    }

    return [...catMap.entries()]
      .sort((a, b) => categoryOrder.indexOf(a[0]) - categoryOrder.indexOf(b[0]))
      .map(([cat, entries]) => ({ category: cat, entries }));
  }, [spans, registry]);

  if (spans.length === 0) return null;

  return (
    <div className="rounded-lg border border-border/50 bg-card shadow-sm overflow-hidden">
      <div className="flex items-center justify-between px-3 py-1.5 bg-muted/30 border-b border-border/30">
        <span className="text-[11px] font-semibold text-foreground">
          Inline Tags in This Segment
        </span>
        <button
          type="button"
          onClick={onClose}
          className="p-0.5 rounded hover:bg-muted/60 transition-colors"
        >
          <X className="w-3 h-3 text-muted-foreground" />
        </button>
      </div>

      <div className="divide-y divide-border/20">
        {groups.map(({ category, entries }) => (
          <div key={category} className="px-3 py-2">
            <div className="text-[9px] font-bold text-muted-foreground uppercase tracking-wider mb-1.5">
              {categoryLabels[category] || category}
            </div>
            <div className="flex flex-col gap-1.5">
              {entries.map(({ typeName, span }) => {
                const info = registry.lookupOrFallback(typeName);
                const constraints = resolveConstraints(span);
                return (
                  <LegendEntry
                    key={typeName}
                    span={span}
                    label={info.label}
                    constraints={constraints}
                  />
                );
              })}
            </div>
          </div>
        ))}
      </div>

      <div className="px-3 py-2 bg-muted/20 border-t border-border/30">
        <div className="text-[9px] text-muted-foreground space-y-0.5">
          <div className="flex items-center gap-1">
            <Lock className="w-2.5 h-2.5" /> Required — must be kept in the translation
          </div>
          <div className="flex items-center gap-1">
            <Copy className="w-2.5 h-2.5" /> No duplicates — cannot be repeated
          </div>
          <div className="flex items-center gap-1">
            <Shuffle className="w-2.5 h-2.5" /> Fixed position — order must be preserved
          </div>
        </div>
      </div>
    </div>
  );
}

interface LegendEntryProps {
  span: SpanInfo;
  label: string;
  constraints: { deletable: boolean; cloneable: boolean; reorderable: boolean };
}

function LegendEntry({ span, label, constraints }: LegendEntryProps) {
  return (
    <div className="flex items-center gap-2">
      <div className="shrink-0">
        <TagChipComponent spanInfo={span} />
      </div>
      <span className="text-[11px] text-foreground font-medium flex-1 min-w-0 truncate">
        {label}
      </span>
      <div className="flex items-center gap-1 shrink-0">
        {!constraints.deletable && (
          <span title="Required — cannot be removed">
            <Lock className={cn("w-2.5 h-2.5 text-destructive")} />
          </span>
        )}
        {!constraints.cloneable && (
          <span title="Cannot be duplicated">
            <Copy className={cn("w-2.5 h-2.5 text-warning")} />
          </span>
        )}
        {!constraints.reorderable && (
          <span title="Fixed position">
            <Shuffle className={cn("w-2.5 h-2.5 text-purple-500")} />
          </span>
        )}
        {constraints.deletable && constraints.cloneable && constraints.reorderable && (
          <span className="text-[9px] text-success font-medium">flexible</span>
        )}
      </div>
    </div>
  );
}

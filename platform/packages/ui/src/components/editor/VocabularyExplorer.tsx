import { useState, useMemo } from "react";
import { getDefaultRegistry, type SpanTypeInfo, type ColorScheme } from "../../vocabularies";
import { TagChipComponent } from "./TagChipComponent";
import type { SpanInfo } from "../../types/api";
import { ChevronDown } from "../icons";
import { cn } from "../../lib/utils";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface CategoryGroup {
  category: string;
  label: string;
  types: Array<{ name: string; info: SpanTypeInfo }>;
}

interface VocabularyExplorerProps {
  /** Highlight only types present in this array (filter mode). */
  activeTypes?: string[];
  /** Compact mode hides descriptions and shows fewer details. */
  compact?: boolean;
  /** Called when user clicks a type entry. */
  onTypeSelect?: (typeName: string) => void;
}

// ---------------------------------------------------------------------------
// Category display labels
// ---------------------------------------------------------------------------

const categoryLabels: Record<string, string> = {
  formatting: "Text Formatting",
  linking: "Links & References",
  media: "Images & Media",
  structure: "Document Structure",
  code: "Code & Variables",
  generic: "Other",
};

const categoryDescriptions: Record<string, string> = {
  formatting:
    "Visual text styles like bold, italic, and underline that should be preserved in translations.",
  linking:
    "Hyperlinks and cross-references — the linked text is translated but the URL is preserved.",
  media: "Embedded images, videos, and other media that appear inline within text.",
  structure: "Structural elements like line breaks and footnotes that control document layout.",
  code: "Variables, placeholders, and function calls that must not be modified during translation.",
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function constraintBadges(
  info: SpanTypeInfo,
): Array<{ label: string; color: string; description: string }> {
  const badges: Array<{ label: string; color: string; description: string }> = [];
  if (!info.constraints.deletable) {
    badges.push({
      label: "Required",
      color: "text-red-500 bg-red-500/10 border-red-500/25",
      description: "Must be present in the translation",
    });
  }
  if (!info.constraints.cloneable) {
    badges.push({
      label: "No duplicates",
      color: "text-amber-600 bg-amber-500/10 border-amber-500/25",
      description: "Cannot be duplicated in the translation",
    });
  }
  if (!info.constraints.reorderable) {
    badges.push({
      label: "Fixed position",
      color: "text-purple-500 bg-purple-500/10 border-purple-500/25",
      description: "Must stay in the same relative position",
    });
  }
  return badges;
}

function makeSpanInfo(
  typeName: string,
  spanType: "opening" | "closing" | "placeholder",
  id: string,
): SpanInfo {
  return { span_type: spanType, type: typeName, id, data: "" };
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function VocabularyExplorer({
  activeTypes,
  compact,
  onTypeSelect,
}: VocabularyExplorerProps) {
  const registry = getDefaultRegistry();
  const [expandedCategory, setExpandedCategory] = useState<string | null>(null);

  const groups: CategoryGroup[] = useMemo(() => {
    const cats = registry.categories();
    return cats.map((cat) => {
      const typeNames = registry.typesInCategory(cat);
      return {
        category: cat,
        label: categoryLabels[cat] || cat,
        types: typeNames.map((name) => ({
          name,
          info: registry.lookupOrFallback(name),
        })),
      };
    });
  }, [registry]);

  const toggleCategory = (cat: string) => {
    setExpandedCategory((prev) => (prev === cat ? null : cat));
  };

  return (
    <div className="flex flex-col gap-1">
      {groups.map((group) => {
        const isExpanded = expandedCategory === group.category;
        const hasActiveTypes = activeTypes
          ? group.types.some((t) => activeTypes.includes(t.name))
          : true;

        return (
          <div
            key={group.category}
            className={cn(
              "rounded-md border border-border/50 overflow-hidden transition-all",
              !hasActiveTypes && activeTypes && "opacity-40",
            )}
          >
            {/* Category header */}
            <button
              type="button"
              onClick={() => toggleCategory(group.category)}
              className="w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-muted/40 transition-colors"
            >
              <CategoryIcon color={group.types[0]?.info.color} />
              <div className="flex-1 min-w-0">
                <div className="text-xs font-semibold text-foreground">{group.label}</div>
                {!compact && (
                  <div className="text-[10px] text-muted-foreground leading-tight">
                    {group.types.length} type{group.types.length !== 1 ? "s" : ""}
                  </div>
                )}
              </div>
              {/* Preview chips for collapsed state */}
              {!isExpanded && (
                <div className="flex items-center gap-0.5 mr-2">
                  {group.types.slice(0, 4).map((t) => (
                    <TagChipComponent
                      key={t.name}
                      spanInfo={makeSpanInfo(
                        t.name,
                        t.info.chipLabel.placeholder ? "placeholder" : "opening",
                        "p",
                      )}
                      dimmed={activeTypes ? !activeTypes.includes(t.name) : false}
                    />
                  ))}
                  {group.types.length > 4 && (
                    <span className="text-[10px] text-muted-foreground ml-1">
                      +{group.types.length - 4}
                    </span>
                  )}
                </div>
              )}
              <ChevronDown
                className={cn(
                  "w-3.5 h-3.5 text-muted-foreground transition-transform shrink-0",
                  isExpanded && "rotate-180",
                )}
              />
            </button>

            {/* Expanded type list */}
            {isExpanded && (
              <div className="border-t border-border/30">
                {!compact && categoryDescriptions[group.category] && (
                  <div className="px-3 py-1.5 text-[11px] text-muted-foreground bg-muted/20 border-b border-border/20">
                    {categoryDescriptions[group.category]}
                  </div>
                )}
                {group.types.map((t) => {
                  const isActive = !activeTypes || activeTypes.includes(t.name);
                  const badges = constraintBadges(t.info);
                  return (
                    <div
                      key={t.name}
                      className={cn(
                        "flex items-start gap-3 px-3 py-2 border-b border-border/20 last:border-b-0",
                        "hover:bg-muted/30 transition-colors",
                        onTypeSelect && "cursor-pointer",
                        !isActive && "opacity-40",
                      )}
                      onClick={() => onTypeSelect?.(t.name)}
                    >
                      {/* Chip previews */}
                      <div className="flex items-center gap-1 pt-0.5 shrink-0">
                        {t.info.chipLabel.open && (
                          <TagChipComponent spanInfo={makeSpanInfo(t.name, "opening", "x")} />
                        )}
                        {t.info.chipLabel.close && (
                          <TagChipComponent spanInfo={makeSpanInfo(t.name, "closing", "x")} />
                        )}
                        {t.info.chipLabel.placeholder && !t.info.chipLabel.open && (
                          <TagChipComponent spanInfo={makeSpanInfo(t.name, "placeholder", "x")} />
                        )}
                      </div>
                      {/* Type info */}
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2">
                          <span className="text-xs font-medium text-foreground">
                            {t.info.label}
                          </span>
                          <span className="text-[10px] text-muted-foreground font-mono">
                            {t.name}
                          </span>
                        </div>
                        {/* Constraint badges */}
                        {badges.length > 0 && (
                          <div className="flex items-center gap-1 mt-1">
                            {badges.map((b) => (
                              <span
                                key={b.label}
                                className={cn(
                                  "text-[9px] px-1.5 py-0.5 rounded border font-medium",
                                  b.color,
                                )}
                                title={b.description}
                              >
                                {b.label}
                              </span>
                            ))}
                          </div>
                        )}
                      </div>
                      {/* Formatted preview (how it looks) */}
                      {!compact && (
                        <div className="text-xs text-muted-foreground shrink-0 pt-0.5 font-mono">
                          {t.info.html.open && t.info.html.close
                            ? `${t.info.html.open}…${t.info.html.close}`
                            : t.info.html.placeholder || ""}
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Tiny category icon (colored dot)
// ---------------------------------------------------------------------------

function CategoryIcon({ color }: { color?: ColorScheme }) {
  return (
    <div
      className="w-2.5 h-2.5 rounded-full shrink-0"
      style={{
        backgroundColor: color?.text || "var(--text-muted)",
        opacity: 0.8,
      }}
    />
  );
}

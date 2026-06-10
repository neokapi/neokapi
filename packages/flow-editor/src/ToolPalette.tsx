import { useState, useMemo } from "react";
import { t } from "@neokapi/kapi-react/runtime";
import { Search, ChevronDown, ChevronRight, GripVertical, Layers } from "lucide-react";
import { InputGroup, InputGroupAddon, InputGroupInput, ScrollArea } from "@neokapi/ui-primitives";
import type { ToolInfo } from "./types";
import { ALL_CATEGORIES } from "./category";
import { IoContract } from "./nodes/PortChip";

interface ToolPaletteProps {
  tools: ToolInfo[];
  onAddTool: (toolName: string) => void;
  /** When embedded (e.g. inside the add-tool modal), fill the host width with no
      side border instead of rendering as a fixed-width sidebar. */
  embedded?: boolean;
}

export function ToolPalette({ tools, onAddTool, embedded }: ToolPaletteProps) {
  const [search, setSearch] = useState("");
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});

  // Pre-normalize tool fields for search (avoids per-keystroke toLowerCase)
  const searchIndex = useMemo(
    () =>
      tools.map((t) => ({
        tool: t,
        key: [t.name, t.description, t.category, ...(t.tags ?? [])].join(" ").toLowerCase(),
      })),
    [tools],
  );

  const filtered = useMemo(() => {
    if (!search) return tools;
    const q = search.toLowerCase();
    return searchIndex.filter(({ key }) => key.includes(q)).map(({ tool }) => tool);
  }, [searchIndex, search, tools]);

  const grouped = useMemo(() => {
    const groups: Record<string, ToolInfo[]> = {};
    for (const cat of ALL_CATEGORIES) {
      groups[cat.id] = [];
    }
    for (const tool of filtered) {
      const cat = tool.category || "pipeline";
      if (!groups[cat]) groups[cat] = [];
      groups[cat].push(tool);
    }
    return groups;
  }, [filtered]);

  const toggle = (cat: string) => setCollapsed((prev) => ({ ...prev, [cat]: !prev[cat] }));

  const handleDragStart = (e: React.DragEvent<HTMLButtonElement>, toolName: string) => {
    e.dataTransfer.setData("application/neokapi-tool", toolName);
    e.dataTransfer.effectAllowed = "copy";
  };

  return (
    <div
      className={
        embedded
          ? "flex h-full w-full flex-col overflow-hidden bg-background"
          : "flex flex-col overflow-hidden border-r border-border bg-background"
      }
      style={embedded ? undefined : { width: 240, minWidth: 240, maxWidth: 240 }}
    >
      {/* Search */}
      <PaletteSearchBar value={search} onChange={setSearch} />

      {/* Categories */}
      <ScrollArea className="flex-1">
        <div className="py-1">
          {ALL_CATEGORIES.map((cat) => {
            const items = grouped[cat.id] || [];
            if (items.length === 0 && search) return null;
            const isCollapsed = collapsed[cat.id] ?? false;
            const Icon = cat.icon;

            return (
              <div key={cat.id}>
                {/* Category header */}
                <button
                  type="button"
                  onClick={() => toggle(cat.id)}
                  className="flex w-full items-center gap-1.5 px-2.5 py-1.5 text-left hover:bg-muted transition-colors"
                >
                  {isCollapsed ? (
                    <ChevronRight size={12} className="shrink-0 text-muted-foreground" />
                  ) : (
                    <ChevronDown size={12} className="shrink-0 text-muted-foreground" />
                  )}
                  <Icon size={13} className="shrink-0" style={{ color: cat.color }} />
                  <span
                    className="text-[11px] font-semibold tracking-wide"
                    style={{ color: cat.text }}
                  >
                    {cat.label}
                  </span>
                  <span className="ml-auto text-[10px] text-muted-foreground">{items.length}</span>
                </button>

                {/* Tool items */}
                {!isCollapsed && (
                  <div className="pb-1">
                    {items.map((tool) => (
                      <PaletteItem
                        key={tool.name}
                        tool={tool}
                        onAdd={() => onAddTool(tool.name)}
                        onDragStart={(e) => handleDragStart(e, tool.name)}
                      />
                    ))}
                    {items.length === 0 && (
                      <div className="pl-9 pr-2.5 py-1 text-[11px] italic text-muted-foreground">
                        No tools
                      </div>
                    )}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </ScrollArea>
    </div>
  );
}

function PaletteSearchBar({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <div className="px-2 pt-2 pb-1">
      <InputGroup className="h-7">
        <InputGroupAddon>
          <Search size={13} />
        </InputGroupAddon>
        <InputGroupInput
          placeholder="Search tools..."
          value={value}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) => onChange(e.target.value)}
          className="text-xs"
        />
      </InputGroup>
    </div>
  );
}

function PaletteItem({
  tool,
  onAdd,
  onDragStart,
}: {
  tool: ToolInfo;
  onAdd: () => void;
  onDragStart: (e: React.DragEvent<HTMLButtonElement>) => void;
}) {
  const displayName = tool.display_name || tool.name;

  return (
    <button
      type="button"
      draggable
      onDragStart={onDragStart}
      onClick={onAdd}
      className="flex w-full items-start gap-1.5 py-1.5 pl-5 pr-2.5 text-left cursor-grab hover:bg-muted transition-colors"
      title={tool.description}
    >
      <GripVertical size={11} className="mt-0.5 shrink-0 text-border" />
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-1">
          <div className="text-[11.5px] font-medium text-foreground">{displayName}</div>
          {tool.isSourceTransform && (
            <span
              className="inline-flex shrink-0 items-center gap-0.5 rounded border border-sky-500/40 bg-sky-500/10 px-1 py-px text-[8px] font-semibold text-sky-600 dark:text-sky-400"
              title="Transformer: rewrites the source. Place it before translation and remote-egress steps — the placement check flags an unsafe slot."
            >
              <Layers size={7} />
              rewrites source
            </span>
          )}
        </div>
        <div className="text-[10px] leading-tight text-muted-foreground">{tool.description}</div>
        {/* Tags + IO contract badges */}
        <div className="mt-0.5 flex flex-wrap gap-0.5">
          {tool.tags?.slice(0, 3).map((tag) => (
            <span
              key={tag}
              className="rounded-sm bg-muted px-1 py-px text-[9px] font-medium text-muted-foreground"
            >
              {tag}
            </span>
          ))}
          {tool.cardinality && tool.cardinality !== "monolingual" && (
            <span className="rounded-sm bg-blue-500/10 px-1 py-px text-[9px] font-medium text-blue-600 dark:text-blue-400">
              {tool.cardinality === "bilingual"
                ? t("Bi", "bilingual badge")
                : t("Multi", "multilingual badge")}
            </span>
          )}
          {tool.default_locale && (
            <span className="rounded-sm bg-purple-500/10 px-1 py-px text-[9px] font-medium text-purple-600 dark:text-purple-400">
              {tool.default_locale}
            </span>
          )}
          {tool.side_effects && tool.side_effects.length > 0 && (
            <span className="rounded-sm bg-amber-500/10 px-1 py-px text-[9px] font-medium text-amber-600 dark:text-amber-400">
              {tool.side_effects.map((s) => s.replace(/-/g, " ")).join(", ")}
            </span>
          )}
        </div>
        {/* Compact IO preview: what this tool reads → writes */}
        {((tool.consumes && tool.consumes.length > 0) ||
          (tool.produces && tool.produces.length > 0)) && (
          <div className="mt-0.5">
            <IoContract consumes={tool.consumes} produces={tool.produces} max={3} />
          </div>
        )}
      </div>
    </button>
  );
}

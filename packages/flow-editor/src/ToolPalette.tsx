import { useState, useMemo } from "react";
import { t } from "@neokapi/kapi-react/runtime";
import { Search, ChevronDown, ChevronRight, GripVertical, Layers, Check } from "lucide-react";
import { InputGroup, InputGroupAddon, InputGroupInput, ScrollArea } from "@neokapi/ui-primitives";
import type { ToolInfo } from "./types";
import { ALL_CATEGORIES } from "./category";
import { IoContract, PortChip } from "./nodes/PortChip";
import { toolFit, type SlotContext, type ToolFit } from "./ioGraph";
import { getPortType } from "./portTypes";

interface ToolPaletteProps {
  tools: ToolInfo[];
  onAddTool: (toolName: string) => void;
  /** When embedded (e.g. inside the add-tool modal), fill the host width with no
      side border instead of rendering as a fixed-width sidebar. */
  embedded?: boolean;
  /** What data is available where the tool will land. When supplied, the palette
      shows an "available here" strip, marks each tool ready/needs-input against
      that context, and lists compatible tools first. */
  slotContext?: SlotContext;
}

export function ToolPalette({ tools, onAddTool, embedded, slotContext }: ToolPaletteProps) {
  const [search, setSearch] = useState("");
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});

  // Pre-normalize tool fields for search. Port types (consumed + produced) and
  // their human labels join the index so typing "term", "qa" or "target" finds
  // tools by what they read/write, not just by name.
  const searchIndex = useMemo(
    () =>
      tools.map((tool) => {
        const ports = [...(tool.consumes ?? []), ...(tool.produces ?? [])];
        const portWords = ports.flatMap((p) => [p.type, getPortType(p.type).label]);
        return {
          tool,
          key: [tool.name, tool.description, tool.category, ...(tool.tags ?? []), ...portWords]
            .join(" ")
            .toLowerCase(),
        };
      }),
    [tools],
  );

  const filtered = useMemo(() => {
    if (!search) return tools;
    const q = search.toLowerCase();
    return searchIndex.filter(({ key }) => key.includes(q)).map(({ tool }) => tool);
  }, [searchIndex, search, tools]);

  // Resolve each tool's fit at the insertion slot once (used for marks + sort).
  const fitOf = useMemo(() => {
    const m = new Map<string, ToolFit>();
    if (!slotContext) return m;
    for (const tool of tools) m.set(tool.name, toolFit(tool, slotContext));
    return m;
  }, [tools, slotContext]);

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
    // With a slot context, surface tools that read cleanly here first; keep the
    // original order otherwise so the list stays stable as you browse.
    if (slotContext) {
      for (const cat of Object.keys(groups)) {
        groups[cat] = stableSortByReady(groups[cat], fitOf);
      }
    }
    return groups;
  }, [filtered, slotContext, fitOf]);

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
      {/* What's available where the tool lands — so "where I add it" is legible. */}
      {slotContext && <AvailableHereStrip context={slotContext} />}

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
                        fit={fitOf.get(tool.name)}
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

/** Ready tools first, then those needing inputs — order otherwise preserved. */
function stableSortByReady(items: ToolInfo[], fitOf: Map<string, ToolFit>): ToolInfo[] {
  return items
    .map((tool, i) => ({ tool, i, ready: fitOf.get(tool.name)?.ready !== false }))
    .sort((a, b) => Number(b.ready) - Number(a.ready) || a.i - b.i)
    .map(({ tool }) => tool);
}

function AvailableHereStrip({ context }: { context: SlotContext }) {
  return (
    <div className="flex flex-wrap items-center gap-1 border-b border-border bg-muted/40 px-2.5 py-1.5">
      <span className="text-[10px] font-medium text-muted-foreground">
        {t("Available here", "ports produced upstream of the insertion point")}
      </span>
      {context.available.map((p, i) => (
        <PortChip key={`${p.type}-${p.side ?? ""}-${i}`} type={p.type} side={p.side} showLabel />
      ))}
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
          placeholder="Search tools, inputs, outputs..."
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
  fit,
  onAdd,
  onDragStart,
}: {
  tool: ToolInfo;
  fit?: ToolFit;
  onAdd: () => void;
  onDragStart: (e: React.DragEvent<HTMLButtonElement>) => void;
}) {
  const displayName = tool.display_name || tool.name;
  // Only dim against a real slot context: with no context (`fit` undefined) every
  // tool reads as neutral, never greyed.
  const needsInput = fit ? !fit.ready : false;

  return (
    <button
      type="button"
      draggable
      onDragStart={onDragStart}
      onClick={onAdd}
      className="flex w-full items-start gap-1.5 py-1.5 pl-5 pr-2.5 text-left cursor-grab hover:bg-muted transition-colors"
      title={
        needsInput && fit
          ? `${tool.description}\n\nNeeds input not available here: ${fit.unmet
              .map((p) => getPortType(p.type).label)
              .join(", ")}`
          : tool.description
      }
    >
      <GripVertical size={11} className="mt-0.5 shrink-0 text-border" />
      <div className={`min-w-0 flex-1 ${needsInput ? "opacity-55" : ""}`}>
        <div className="flex items-center gap-1">
          {/* A clean fit reads at a glance — a small check, not a wall of green. */}
          {fit?.ready && (
            <Check
              size={11}
              className="shrink-0 text-emerald-600 dark:text-emerald-400"
              aria-label="reads cleanly here"
            />
          )}
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
        {/* What's missing at this slot — names the unmet inputs, never blocks. */}
        {needsInput && fit && fit.unmet.length > 0 && (
          <div className="mt-0.5 flex flex-wrap items-center gap-0.5">
            <span className="text-[9px] font-medium text-amber-600 dark:text-amber-400">
              {t("needs", "label before the inputs a tool is missing at this slot")}
            </span>
            {fit.unmet.map((p, i) => (
              <PortChip
                key={`need-${p.type}-${p.side ?? ""}-${i}`}
                type={p.type}
                side={p.side}
                verb="consumes"
                showLabel
              />
            ))}
          </div>
        )}
      </div>
    </button>
  );
}

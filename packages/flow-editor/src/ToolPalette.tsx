import { useState, useMemo } from "react";
import { Search, ChevronDown, ChevronRight, GripVertical } from "lucide-react";
import {
  Button,
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
  ScrollArea,
} from "@neokapi/ui-primitives";
import type { ToolInfo } from "./types";
import { ALL_CATEGORIES } from "./category";

interface ToolPaletteProps {
  tools: ToolInfo[];
  onAddTool: (toolName: string) => void;
}

export function ToolPalette({ tools, onAddTool }: ToolPaletteProps) {
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
    <div className="flex w-60 flex-col overflow-hidden border-r border-border bg-background">
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
                <Button
                  variant="ghost"
                  onClick={() => toggle(cat.id)}
                  className="flex w-full items-center gap-1.5 px-2.5 py-1.5 text-left h-auto rounded-none"
                >
                  {isCollapsed ? (
                    <ChevronRight size={12} className="text-muted-foreground" />
                  ) : (
                    <ChevronDown size={12} className="text-muted-foreground" />
                  )}
                  <Icon size={13} style={{ color: cat.color }} />
                  <span
                    className="text-[11px] font-semibold tracking-wide"
                    style={{ color: cat.text }}
                  >
                    {cat.label}
                  </span>
                  <span className="ml-auto text-[10px] text-muted-foreground">{items.length}</span>
                </Button>

                {/* Tool items */}
                {!isCollapsed && (
                  <div className="pb-1">
                    {items.map((tool) => (
                      <PaletteItem
                        key={tool.name}
                        tool={tool}
                        categoryColor={cat.color}
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
  categoryColor,
  onAdd,
  onDragStart,
}: {
  tool: ToolInfo;
  categoryColor: string;
  onAdd: () => void;
  onDragStart: (e: React.DragEvent<HTMLButtonElement>) => void;
}) {
  const displayName = tool.display_name || tool.name;

  return (
    <Button
      variant="ghost"
      draggable
      onDragStart={onDragStart}
      onClick={onAdd}
      className="flex w-full items-start gap-1.5 py-1.5 pl-5 pr-2.5 text-left h-auto rounded-none cursor-grab"
      title={tool.description}
    >
      <GripVertical size={11} className="mt-0.5 shrink-0 text-border" />
      <div className="min-w-0 flex-1">
        <div className="truncate text-[11.5px] font-medium text-foreground">{displayName}</div>
        <div className="truncate text-[10px] leading-tight text-muted-foreground">
          {tool.description}
        </div>
        {tool.tags && tool.tags.length > 0 && (
          <div className="mt-0.5 flex flex-wrap gap-0.5">
            {tool.tags.slice(0, 3).map((tag) => (
              <span
                key={tag}
                className="rounded-sm px-1 py-px text-[9px] font-medium"
                style={{
                  background: `${categoryColor}22`,
                  color: categoryColor,
                }}
              >
                {tag}
              </span>
            ))}
          </div>
        )}
      </div>
    </Button>
  );
}

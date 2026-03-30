import { useState, useMemo } from "react";
import { Search, ChevronDown, ChevronRight, GripVertical } from "lucide-react";
import type { ToolInfo } from "./types";
import { ALL_CATEGORIES } from "./category";
import { theme } from "./theme";

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

  const toggle = (cat: string) =>
    setCollapsed((prev) => ({ ...prev, [cat]: !prev[cat] }));

  const handleDragStart = (
    e: React.DragEvent<HTMLButtonElement>,
    toolName: string,
  ) => {
    e.dataTransfer.setData("application/neokapi-tool", toolName);
    e.dataTransfer.effectAllowed = "copy";
  };

  return (
    <div
      style={{
        width: 240,
        display: "flex",
        flexDirection: "column",
        borderRight: `1px solid ${theme.border}`,
        background: theme.bg,
        overflow: "hidden",
      }}
    >
      {/* Search */}
      <div style={{ padding: "8px 8px 4px" }}>
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 6,
            padding: "5px 8px",
            borderRadius: 6,
            border: `1px solid ${theme.border}`,
            background: theme.bgCard,
          }}
        >
          <Search size={13} style={{ color: theme.fgMuted, flexShrink: 0 }} />
          <input
            type="text"
            placeholder="Search tools..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            style={{
              flex: 1,
              background: "transparent",
              border: "none",
              outline: "none",
              fontSize: 12,
              color: theme.fg,
              fontFamily: "inherit",
            }}
          />
        </div>
      </div>

      {/* Categories */}
      <div style={{ flex: 1, overflow: "auto", padding: "4px 0" }}>
        {ALL_CATEGORIES.map((cat) => {
          const items = grouped[cat.id] || [];
          if (items.length === 0 && search) return null;
          const isCollapsed = collapsed[cat.id] ?? false;
          const Icon = cat.icon;

          return (
            <div key={cat.id}>
              {/* Category header */}
              <button
                onClick={() => toggle(cat.id)}
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 6,
                  width: "100%",
                  padding: "6px 10px",
                  background: "none",
                  border: "none",
                  cursor: "pointer",
                  textAlign: "left",
                }}
              >
                {isCollapsed ? (
                  <ChevronRight size={12} style={{ color: theme.fgMuted }} />
                ) : (
                  <ChevronDown size={12} style={{ color: theme.fgMuted }} />
                )}
                <Icon size={13} style={{ color: cat.color }} />
                <span
                  style={{
                    fontSize: 11,
                    fontWeight: 600,
                    color: cat.text,
                    letterSpacing: "0.02em",
                  }}
                >
                  {cat.label}
                </span>
                <span
                  style={{
                    fontSize: 10,
                    color: theme.fgMuted,
                    marginLeft: "auto",
                  }}
                >
                  {items.length}
                </span>
              </button>

              {/* Tool items */}
              {!isCollapsed && (
                <div style={{ paddingBottom: 4 }}>
                  {items.map((tool) => (
                    <ToolItem
                      key={tool.name}
                      tool={tool}
                      categoryColor={cat.color}
                      onAdd={() => onAddTool(tool.name)}
                      onDragStart={(e) => handleDragStart(e, tool.name)}
                    />
                  ))}
                  {items.length === 0 && (
                    <div
                      style={{
                        padding: "4px 10px 4px 36px",
                        fontSize: 11,
                        color: theme.fgMuted,
                        fontStyle: "italic",
                      }}
                    >
                      No tools
                    </div>
                  )}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

function ToolItem({
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
  const [hovered, setHovered] = useState(false);
  const displayName = tool.name.replace(/^okapi:/, "");

  return (
    <button
      draggable
      onDragStart={onDragStart}
      onClick={onAdd}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      style={{
        display: "flex",
        alignItems: "flex-start",
        gap: 6,
        width: "100%",
        padding: "5px 10px 5px 20px",
        background: hovered ? theme.bgSecondary : "none",
        border: "none",
        cursor: "grab",
        textAlign: "left",
      }}
      title={tool.description}
    >
      <GripVertical
        size={11}
        style={{
          color: theme.border,
          marginTop: 2,
          flexShrink: 0,
        }}
      />
      <div style={{ flex: 1, minWidth: 0 }}>
        <div
          style={{
            fontSize: 11.5,
            fontWeight: 500,
            color: theme.fg,
            whiteSpace: "nowrap",
            overflow: "hidden",
            textOverflow: "ellipsis",
          }}
        >
          {displayName}
        </div>
        <div
          style={{
            fontSize: 10,
            color: theme.fgMuted,
            lineHeight: 1.3,
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
          }}
        >
          {tool.description}
        </div>
        {tool.tags && tool.tags.length > 0 && (
          <div style={{ display: "flex", gap: 3, marginTop: 2, flexWrap: "wrap" }}>
            {tool.tags.slice(0, 3).map((tag) => (
              <span
                key={tag}
                style={{
                  fontSize: 9,
                  padding: "1px 4px",
                  borderRadius: 3,
                  background: `${categoryColor}22`,
                  color: categoryColor,
                  fontWeight: 500,
                }}
              >
                {tag}
              </span>
            ))}
          </div>
        )}
      </div>
    </button>
  );
}

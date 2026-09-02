import { useState, useRef, useEffect, type KeyboardEvent } from "react";
import { X } from "lucide-react";
import { Button, SimpleTooltip } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type { TabInfo } from "../types/api";

interface TabBarProps {
  tabs: TabInfo[];
  activeTabID: string | null;
  onSelect: (tabID: string) => void;
  onClose: (tabID: string) => void;
  onRename: (tabID: string, name: string) => void;
}

export function TabBar({ tabs, activeTabID, onSelect, onClose, onRename }: TabBarProps) {
  const [editingID, setEditingID] = useState<string | null>(null);
  const [editValue, setEditValue] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const tabRefs = useRef(new Map<string, HTMLDivElement>());

  useEffect(() => {
    if (editingID && inputRef.current) {
      inputRef.current.focus();
      inputRef.current.select();
    }
  }, [editingID]);

  const startEditing = (tab: TabInfo) => {
    setEditingID(tab.id);
    setEditValue(tab.name);
  };

  const commitRename = () => {
    if (editingID && editValue.trim()) {
      onRename(editingID, editValue.trim());
    }
    setEditingID(null);
  };

  const focusTab = (id: string) => {
    tabRefs.current.get(id)?.focus();
  };

  // Automatic-activation tablist: arrow keys both move focus and select, Enter/
  // Space select the focused tab, F2 renames.
  const handleTabKeyDown = (event: KeyboardEvent<HTMLDivElement>, tab: TabInfo, index: number) => {
    if (editingID === tab.id) return; // the rename input owns keys while editing
    const move = (next: TabInfo) => {
      event.preventDefault();
      onSelect(next.id);
      focusTab(next.id);
    };
    switch (event.key) {
      case "Enter":
      case " ":
        event.preventDefault();
        onSelect(tab.id);
        break;
      case "ArrowRight":
      case "ArrowDown":
        move(tabs[(index + 1) % tabs.length]);
        break;
      case "ArrowLeft":
      case "ArrowUp":
        move(tabs[(index - 1 + tabs.length) % tabs.length]);
        break;
      case "Home":
        move(tabs[0]);
        break;
      case "End":
        move(tabs[tabs.length - 1]);
        break;
      case "F2":
        event.preventDefault();
        startEditing(tab);
        break;
    }
  };

  if (tabs.length === 0) return null;

  // Roving tabindex: exactly one tab is in the tab order at a time.
  const focusableID = activeTabID ?? tabs[0].id;

  return (
    <div
      role="tablist"
      aria-label={t("Open projects")}
      className="flex items-end gap-px overflow-x-auto px-1"
    >
      {tabs.map((tab, index) => {
        const isActive = activeTabID === tab.id;
        return (
          <div
            key={tab.id}
            ref={(el) => {
              if (el) tabRefs.current.set(tab.id, el);
              else tabRefs.current.delete(tab.id);
            }}
            role="tab"
            aria-selected={isActive}
            tabIndex={tab.id === focusableID ? 0 : -1}
            onClick={() => onSelect(tab.id)}
            onKeyDown={(e) => handleTabKeyDown(e, tab, index)}
            className={`group flex min-w-[160px] max-w-[240px] cursor-pointer items-center justify-between gap-1.5 rounded-t-lg px-3 py-1.5 text-xs transition-all outline-none focus-visible:ring-2 focus-visible:ring-ring ${
              isActive
                ? "bg-border text-foreground font-semibold"
                : "text-muted-foreground hover:bg-accent/40 hover:text-foreground"
            }`}
          >
            {editingID === tab.id ? (
              <input
                ref={inputRef}
                value={editValue}
                onChange={(e) => setEditValue(e.target.value)}
                onBlur={commitRename}
                onKeyDown={(e) => {
                  if (e.key === "Enter") commitRename();
                  if (e.key === "Escape") setEditingID(null);
                }}
                className="w-24 rounded bg-transparent px-0.5 text-xs outline-none ring-1 ring-ring"
                aria-label="Rename project"
              />
            ) : (
              <SimpleTooltip content={tab.path ? `${tab.name} · ${tab.path}` : tab.name}>
                <span onDoubleClick={() => startEditing(tab)} className="max-w-[140px] truncate">
                  {tab.name}
                </span>
              </SimpleTooltip>
            )}
            <Button
              variant="ghost"
              size="icon-xs"
              onClick={(e: React.MouseEvent) => {
                e.stopPropagation();
                onClose(tab.id);
              }}
              className="h-4 w-4 opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
              aria-label={t("Close {name}", { name: tab.name })}
            >
              <X size={10} />
            </Button>
          </div>
        );
      })}
    </div>
  );
}

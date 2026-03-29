import { useState, useRef, useEffect } from "react";
import { X } from "lucide-react";
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

  if (tabs.length === 0) return null;

  return (
    <div className="flex items-center gap-0.5 px-2 overflow-x-auto">
      {tabs.map((tab) => (
        <div
          key={tab.id}
          className={`group flex items-center gap-1.5 rounded-t-md px-3 py-1.5 text-xs transition-colors ${
            activeTabID === tab.id
              ? "bg-background text-foreground border-b-2 border-primary"
              : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
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
            <button
              onClick={() => onSelect(tab.id)}
              onDoubleClick={() => startEditing(tab)}
              className="max-w-[140px] truncate"
              title={tab.path ? `${tab.name} — ${tab.path}` : tab.name}
            >
              {tab.name}
            </button>
          )}
          <button
            onClick={(e) => {
              e.stopPropagation();
              onClose(tab.id);
            }}
            className="rounded p-0.5 opacity-0 transition-opacity hover:bg-accent group-hover:opacity-100"
            aria-label={`Close ${tab.name}`}
          >
            <X size={10} />
          </button>
        </div>
      ))}
    </div>
  );
}

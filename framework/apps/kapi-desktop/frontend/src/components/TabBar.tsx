import { X } from "lucide-react";
import type { TabInfo } from "../types/api";

interface TabBarProps {
  tabs: TabInfo[];
  activeTabID: string | null;
  onSelect: (tabID: string) => void;
  onClose: (tabID: string) => void;
}

export function TabBar({ tabs, activeTabID, onSelect, onClose }: TabBarProps) {
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
          <button
            onClick={() => onSelect(tab.id)}
            className="max-w-[140px] truncate"
            title={tab.path || tab.name}
          >
            {tab.name}
          </button>
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

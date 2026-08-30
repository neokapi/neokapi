// The Context pillar, and the surfaces that read the project's context graph.
//
// One rail, one model: the explorer answers a question at a point, and Voice
// reads the profile governing each point whole. Both resolve through the same
// host layer a run and a `kapi check` go through.

import { useState } from "react";
import { Compass, MessageSquareQuote } from "lucide-react";
import { cn } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { ContextExplorerView } from "./ContextExplorerView";
import { VoicePage } from "./VoicePage";

/** The surfaces filed under Context. */
export type ContextSection = "explorer" | "voice";

export interface ContextHubProps {
  tabID: string;
  /** The open project's name — the ladder's project rung. */
  projectName: string;
  /** The section to open on. */
  section?: ContextSection;
}

const SECTIONS: Array<{ id: ContextSection; label: string; icon: React.ReactNode }> = [
  { id: "explorer", label: "Explorer", icon: <Compass size={14} /> },
  { id: "voice", label: "Voice", icon: <MessageSquareQuote size={14} /> },
];

export function ContextHub({ tabID, projectName, section }: ContextHubProps) {
  const [active, setActive] = useState<ContextSection>(section ?? "explorer");

  return (
    <div className="flex h-full min-h-0 flex-col">
      <nav
        aria-label={t("Context sections")}
        className="flex shrink-0 items-center gap-1 border-b border-border px-6 py-2"
      >
        {SECTIONS.map((s) => (
          <button
            key={s.id}
            type="button"
            onClick={() => setActive(s.id)}
            aria-current={active === s.id ? "page" : undefined}
            className={cn(
              "flex items-center gap-1.5 rounded-md px-2.5 py-1 text-sm transition-colors",
              active === s.id
                ? "bg-accent font-medium text-foreground"
                : "text-muted-foreground hover:bg-accent/60 hover:text-foreground",
            )}
          >
            {s.icon}
            {s.label}
          </button>
        ))}
      </nav>
      <div className="min-h-0 flex-1">
        {active === "explorer" ? (
          <ContextExplorerView tabID={tabID} projectName={projectName} />
        ) : (
          <VoicePage tabID={tabID} />
        )}
      </div>
    </div>
  );
}

// The Context pillar, and the surfaces that read the project's context graph.
//
// One rail, one model: the explorer answers a question at a point, Voice reads
// the profile governing each point whole, and the two stores hold what that
// governance is made of. Filing the stores here says what they are — the terms
// a project has agreed and the wording it has already approved are OF its
// context, not separate cabinets beside it.
//
// Memory stays behind the gate the sidebar applies: a project with no target
// languages is not shown a surface it has nothing to put in.

import { useState } from "react";
import { BookOpen, Compass, Database, MessageSquareQuote } from "lucide-react";
import { cn } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { ContextExplorerView } from "./ContextExplorerView";
import { VoicePage } from "./VoicePage";
import { TermsPage } from "./TermsPage";
import { MemoriesPage } from "./MemoriesPage";

/** The surfaces filed under Context. */
export type ContextSection = "explorer" | "voice" | "terms" | "memory";

/** A point to open the explorer standing at, and what sent it there. */
export interface ContextPin {
  coordinate?: string;
  collection?: string;
  path?: string;
  /** The rule that fired, when a check finding opened this. */
  rule?: string;
}

export interface ContextHubProps {
  tabID: string;
  /** The open project's name — the ladder's project rung. */
  projectName: string;
  /** The section to open on. */
  section?: ContextSection;
  /** Open the explorer pinned at a point, e.g. from a check finding. */
  pin?: ContextPin;
  /** Whether the project declares targets. Memory appears when it does. */
  hasTargetLanguages?: boolean;
}

const SECTIONS: Array<{
  id: ContextSection;
  label: string;
  icon: React.ReactNode;
  localeGated?: boolean;
}> = [
  { id: "explorer", label: "Explorer", icon: <Compass size={14} /> },
  { id: "voice", label: "Voice", icon: <MessageSquareQuote size={14} /> },
  { id: "terms", label: "Terms", icon: <BookOpen size={14} /> },
  { id: "memory", label: "Content Memory", icon: <Database size={14} />, localeGated: true },
];

export function ContextHub({
  tabID,
  projectName,
  section,
  pin,
  hasTargetLanguages,
}: ContextHubProps) {
  const [active, setActive] = useState<ContextSection>(section ?? "explorer");
  // A pin set from inside the hub (the memory browser opening a unit) rather
  // than handed in by the router.
  const [unitPin, setUnitPin] = useState<ContextPin | null>(null);
  const sections = SECTIONS.filter((s) => !s.localeGated || hasTargetLanguages);
  // A section the project's languages gated away must not stay selected.
  const current = sections.some((s) => s.id === active) ? active : "explorer";

  return (
    <div className="flex h-full min-h-0 flex-col">
      <nav
        aria-label={t("Context sections")}
        className="flex shrink-0 items-center gap-1 border-b border-border px-6 py-2"
      >
        {sections.map((s) => (
          <button
            key={s.id}
            type="button"
            onClick={() => setActive(s.id)}
            aria-current={current === s.id ? "page" : undefined}
            className={cn(
              "flex items-center gap-1.5 rounded-md px-2.5 py-1 text-sm transition-colors",
              current === s.id
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
        {current === "explorer" && (
          <ContextExplorerView tabID={tabID} projectName={projectName} pin={unitPin ?? pin} />
        )}
        {current === "voice" && <VoicePage tabID={tabID} />}
        {current === "terms" && <TermsPage tabID={tabID} />}
        {current === "memory" && (
          <MemoriesPage
            tabID={tabID}
            onOpenUnit={(unitPath) => {
              // An approved answer names the unit it was approved for; the
              // explorer is where that unit's governance is read.
              setUnitPin({ path: unitPath });
              setActive("explorer");
            }}
          />
        )}
      </div>
    </div>
  );
}

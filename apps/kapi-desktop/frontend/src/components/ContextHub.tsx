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
import { BookOpen, Bot, Compass, Database, MessageSquareQuote, Sparkles } from "lucide-react";
import { ScrollArea, cn } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { AgentContextPane } from "./AgentContextPane";
import { ContextDigestPanel } from "./ContextDigestPanel";
import { ContextExplorerView } from "./ContextExplorerView";
import { VoicePage } from "./VoicePage";
import { TermsPage } from "./TermsPage";
import { MemoriesPage } from "./MemoriesPage";

/** The surfaces filed under Context. */
export type ContextSection = "learned" | "explorer" | "agent" | "voice" | "terms" | "memory";

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
  /**
   * A tab over a project's context alone: the workspace holds the project and
   * no checkout on this machine carries it. The sections that read files stay
   * out of a tab that has none.
   */
  contextOnly?: boolean;
}

const SECTIONS: Array<{
  id: ContextSection;
  label: string;
  icon: React.ReactNode;
  localeGated?: boolean;
  /**
   * Reads the recipe or the files beside it, so a context-only tab does not
   * offer it: there is no checkout behind that tab to read either from.
   */
  needsCheckout?: boolean;
}> = [
  { id: "learned", label: "Learned", icon: <Sparkles size={14} /> },
  { id: "explorer", label: "Explorer", icon: <Compass size={14} />, needsCheckout: true },
  { id: "agent", label: "Agent View", icon: <Bot size={14} />, needsCheckout: true },
  {
    id: "voice",
    label: "Voice",
    icon: <MessageSquareQuote size={14} />,
    needsCheckout: true,
  },
  { id: "terms", label: "Terms", icon: <BookOpen size={14} /> },
  { id: "memory", label: "Content Memory", icon: <Database size={14} />, localeGated: true },
];

export function ContextHub({
  tabID,
  projectName,
  section,
  pin,
  hasTargetLanguages,
  contextOnly,
}: ContextHubProps) {
  const [active, setActive] = useState<ContextSection>(section ?? (pin ? "explorer" : "learned"));
  // A pin set from inside the hub (the memory browser opening a unit) rather
  // than handed in by the router.
  const [unitPin, setUnitPin] = useState<ContextPin | null>(null);
  // A context-only tab holds the whole of a project's context and none of its
  // files, so the language gate does not apply there: the content memory is
  // one of the two things such a tab exists to show.
  const sections = SECTIONS.filter(
    (s) =>
      (!s.localeGated || hasTargetLanguages || contextOnly) && (!s.needsCheckout || !contextOnly),
  );
  // A section the project's languages or its missing checkout gated away must
  // not stay selected.
  const current = sections.some((s) => s.id === active) ? active : sections[0].id;

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
        {current === "agent" && <AgentContextPane tabID={tabID} path={pin?.path} />}
        {current === "learned" && (
          <ScrollArea className="h-full">
            <div className="mx-auto max-w-3xl px-6 py-5">
              <h2 className="text-lg font-semibold">What kapi learned about {projectName}</h2>
              <div className="mt-3">
                <ContextDigestPanel
                  tabID={tabID}
                  canDecide={!contextOnly}
                  onOpenFile={
                    contextOnly
                      ? undefined
                      : (path) => {
                          setUnitPin({ path });
                          setActive("explorer");
                        }
                  }
                />
              </div>
            </div>
          </ScrollArea>
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

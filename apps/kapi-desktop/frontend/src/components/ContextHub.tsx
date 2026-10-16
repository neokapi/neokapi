// The Context pillar, and the surfaces that read the project's context graph.
//
// One rail, in the order a person meets it: What's new holds the rules and
// suggestions kapi keeps for the project and is where a person reviews them;
// Ask answers a question at a point; Assistant shows what an assistant reads;
// Voice reads the profile governing each point whole; the two stores hold
// what that governance is made of; History lists every operation, with the
// resets; Share says how far the context reaches. Filing the stores here says
// what they are: the terms a project has agreed and the wording it has
// already approved are part of its context.
//
// Memory stays behind the gate the sidebar applies: a project with no target
// languages is not shown a surface it has nothing to put in.

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  BookOpen,
  Bot,
  Compass,
  Database,
  History,
  MessageSquareQuote,
  Sparkles,
} from "lucide-react";
import { LoadingSpinner, PageHeader, ScrollArea, cn } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { AgentContextPane } from "./AgentContextPane";
import { ContextDigestPanel } from "./ContextDigestPanel";
import { ContextExplorerView } from "./ContextExplorerView";
import { ContextFeedPanel } from "./ContextFeedPanel";
import { VoicePage } from "./VoicePage";
import { TermsPage } from "./TermsPage";
import { MemoriesPage } from "./MemoriesPage";

/** The surfaces filed under Context. */
export type ContextSection =
  | "rules"
  | "explorer"
  | "agent"
  | "voice"
  | "terms"
  | "memory"
  | "history";

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
  icon: React.ReactNode;
  localeGated?: boolean;
  /**
   * Reads the recipe or the files beside it, so a context-only tab does not
   * offer it: there is no checkout behind that tab to read either from.
   */
  needsCheckout?: boolean;
}> = [
  { id: "rules", icon: <Sparkles size={14} /> },
  { id: "explorer", icon: <Compass size={14} />, needsCheckout: true },
  { id: "agent", icon: <Bot size={14} />, needsCheckout: true },
  {
    id: "voice",
    icon: <MessageSquareQuote size={14} />,
    needsCheckout: true,
  },
  { id: "terms", icon: <BookOpen size={14} /> },
  { id: "memory", icon: <Database size={14} />, localeGated: true },
  { id: "history", icon: <History size={14} /> },
];

/**
 * The section a view opens the hub on: "context:<section>" names one, and the
 * stores' own view ids stay routable so a persisted view lands where it did.
 */
export function contextSectionOf(view: string): ContextSection | undefined {
  if (view === "termbases") return "terms";
  if (view === "memories") return "memory";
  const [base, section] = view.split(":");
  if (base !== "context" || !section) return undefined;
  return SECTIONS.find((s) => s.id === section)?.id;
}

export function ContextHub({
  tabID,
  projectName,
  section,
  pin,
  hasTargetLanguages,
  contextOnly,
}: ContextHubProps) {
  const [active, setActive] = useState<ContextSection>(section ?? (pin ? "explorer" : "rules"));
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
  const offers = (id: ContextSection) => sections.some((s) => s.id === id);

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
            <SectionLabel id={s.id} />
          </button>
        ))}
        <span
          data-slot="context-section-command"
          className="ml-auto shrink-0 pl-3 text-[11px] text-muted-foreground empty:hidden"
        >
          <SectionCommand id={current} />
        </span>
      </nav>
      <div className="min-h-0 flex-1">
        {current === "explorer" && (
          <ContextExplorerView tabID={tabID} projectName={projectName} pin={unitPin ?? pin} />
        )}
        {current === "agent" && <AgentContextPane tabID={tabID} path={pin?.path} />}
        {current === "rules" && (
          <ScrollArea className="h-full">
            <div className="mx-auto max-w-3xl px-6 py-5">
              <PageHeader
                className="mb-3"
                title={t("What's new for {project}", { project: projectName })}
              />
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
                onConnectAssistant={offers("agent") ? () => setActive("agent") : undefined}
              />
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
        {current === "history" && (
          <ScrollArea className="h-full">
            <div className="mx-auto max-w-3xl px-6 py-5">
              <PageHeader className="mb-3" title="History" />
              <ProjectHistory tabID={tabID} />
            </div>
          </ScrollArea>
        )}
      </div>
    </div>
  );
}

/**
 * The project's own feed. The feed is narrowed by the workspace's key for the
 * project, which the digest's first read carries; this shares that read.
 */
function ProjectHistory({ tabID }: { tabID: string }) {
  const digest = useQuery({
    queryKey: qk.projectContextDigest(tabID, ""),
    queryFn: () => api.projectContextDigest(tabID, ""),
  });
  const key = digest.data?.project;
  if (!key) return digest.isLoading ? <LoadingSpinner /> : null;
  return <ContextFeedPanel projectKey={key} />;
}

/** The command a section does the work of, for the reader who has a terminal. */
function SectionCommand({ id }: { id: ContextSection }) {
  const code = (command: string) => (
    <code className="font-mono" translate="no">
      {command}
    </code>
  );
  switch (id) {
    case "rules":
      return <>The same as {code("kapi context review")}</>;
    case "agent":
      return code("kapi init");
    case "history":
      return code("kapi context log");
  }
  return null;
}

/** A section's name on the rail, written here so it is translated. */
function SectionLabel({ id }: { id: ContextSection }) {
  switch (id) {
    case "rules":
      return <>What&apos;s new</>;
    case "explorer":
      return <>Ask</>;
    case "agent":
      return <>Assistant</>;
    case "voice":
      return <>Voice</>;
    case "terms":
      return <>Terms</>;
    case "memory":
      return <>Content Memory</>;
    case "history":
      return <>History</>;
  }
}

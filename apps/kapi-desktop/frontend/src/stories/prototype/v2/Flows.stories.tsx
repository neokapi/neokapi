import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import {
  ArrowRight,
  BookOpen,
  ChevronDown,
  FileText,
  FolderKanban,
  Home,
  Languages,
  Pencil,
  Play,
  Plus,
  ShieldCheck,
  SlidersHorizontal,
  Trash2,
  Workflow,
  Wrench,
} from "lucide-react";
import { Badge, Button } from "@neokapi/ui-primitives";
import { type FlowDef, configuredFlows, DesktopFrame } from "../_shared";

/**
 * Prototype v2 (source-first): where do FLOWS live?
 *
 * Flows are no longer a sidebar pillar (no rail icon). Everyone has them, as a
 * shared library you create / edit / delete and run — from a project or ad-hoc.
 * The open question is the conceptual HOME of that library. These three
 * concepts put the same flows in three different places so we can compare:
 *
 *   A — In the Project  (flows are project config, in the recipe)
 *   B — In the Toolbox  (a flow is a saved pipeline of tools)
 *   C — Contextual      (flows aren't a place — a "Run a flow" action opens them)
 *
 * The flow data and the flow-library cards are shared; only the container and
 * the entry point change.
 */
const meta = {
  title: "Prototype v2/Flows",
  parameters: { layout: "fullscreen" },
} satisfies Meta;

export default meta;
type Story = StoryObj;

/* ---- shared flow-library pieces ---- */

function FlowKindBadge({ kind }: { kind: FlowDef["kind"] }) {
  return kind === "localization" ? (
    <Badge variant="secondary" className="gap-1 text-[10px]">
      <Languages size={10} />
      Localization
    </Badge>
  ) : (
    <Badge variant="outline" className="gap-1 text-[10px] text-muted-foreground">
      <FileText size={10} />
      Content
    </Badge>
  );
}

function StepPill({ children }: { children: ReactNode }) {
  return (
    <span className="rounded-md bg-muted px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground">
      {children}
    </span>
  );
}

function FlowCard({ flow, compact }: { flow: FlowDef; compact?: boolean }) {
  return (
    <div className="rounded-xl border border-border bg-card p-3.5">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <Workflow size={14} className="shrink-0 text-muted-foreground" />
            <span className="truncate text-sm font-medium">{flow.name}</span>
            <FlowKindBadge kind={flow.kind} />
          </div>
          {!compact && (
            <p className="mt-1 text-xs leading-relaxed text-muted-foreground">{flow.description}</p>
          )}
          <div className="mt-2 flex flex-wrap items-center gap-1">
            {flow.steps.map((s, i) => (
              <span key={s} className="flex items-center gap-1">
                {i > 0 && <ArrowRight size={11} className="text-muted-foreground/50" />}
                <StepPill>{s}</StepPill>
              </span>
            ))}
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          <Button size="xs" variant="outline">
            <Play size={12} /> Run
          </Button>
          {!compact && (
            <>
              <Button size="icon-sm" variant="ghost" aria-label="Edit flow">
                <Pencil size={14} />
              </Button>
              <Button size="icon-sm" variant="ghost" aria-label="Delete flow">
                <Trash2 size={14} />
              </Button>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

function FlowList({ compact }: { compact?: boolean }) {
  return (
    <div className="space-y-2">
      {configuredFlows.map((f) => (
        <FlowCard key={f.name} flow={f} compact={compact} />
      ))}
    </div>
  );
}

/* ---- a labeled rail (illustrative) — note: NO Flows item ---- */

const RAIL_ITEMS: { view: string; label: string; icon: typeof Home }[] = [
  { view: "home", label: "Home", icon: Home },
  { view: "project", label: "Project", icon: FolderKanban },
  { view: "content", label: "Content", icon: FileText },
  { view: "checks", label: "Checks", icon: ShieldCheck },
  { view: "toolbox", label: "Toolbox", icon: Wrench },
  { view: "termbases", label: "Termbases", icon: BookOpen },
];

function Rail({ active }: { active: string }) {
  return (
    <aside className="flex w-44 shrink-0 flex-col border-r border-border bg-muted/20 p-2">
      <nav className="flex flex-1 flex-col gap-0.5">
        {RAIL_ITEMS.map(({ view, label, icon: Icon }) => (
          <div
            key={view}
            className={`flex items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm ${
              active === view ? "bg-primary text-primary-foreground" : "text-muted-foreground"
            }`}
          >
            <Icon size={17} strokeWidth={1.5} />
            <span>{label}</span>
          </div>
        ))}
      </nav>
      <div className="flex items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm text-muted-foreground">
        <SlidersHorizontal size={17} strokeWidth={1.5} />
        <span>Project Settings</span>
      </div>
    </aside>
  );
}

function ConceptShell({
  title,
  blurb,
  active,
  children,
}: {
  title: string;
  blurb: ReactNode;
  active: string;
  children: ReactNode;
}) {
  return (
    <div className="min-h-screen bg-background p-8 text-foreground">
      <div className="mx-auto max-w-5xl">
        <h1 className="text-xl font-semibold">{title}</h1>
        <p className="mt-1 max-w-3xl text-sm leading-relaxed text-muted-foreground">{blurb}</p>
        <div className="mt-5">
          <DesktopFrame title="Acme Marketing Site">
            <div className="flex h-[560px]">
              <Rail active={active} />
              {children}
            </div>
          </DesktopFrame>
        </div>
      </div>
    </div>
  );
}

/* ---- Concept A — flows live in the Project ---- */

export const A_InProject: Story = {
  name: "A — In the Project",
  render: () => (
    <ConceptShell
      title="Concept A — Flows live in the Project"
      blurb="Flows are project configuration — they live in the project's recipe. You create, edit, and run them from the project; the toolbox can still run a saved flow ad-hoc. Reached via Project — no Flows rail icon."
      active="project"
    >
      <div className="flex-1 overflow-y-auto bg-background p-6">
        <h2 className="text-lg font-semibold">Project</h2>
        <p className="mt-0.5 text-sm text-muted-foreground">en-US → fr-FR, de-DE · 248 blocks</p>

        <div className="mt-6 flex items-center justify-between">
          <h3 className="text-sm font-semibold">Flows</h3>
          <Button size="xs" variant="outline">
            <Plus size={13} /> New flow
          </Button>
        </div>
        <p className="mb-3 mt-0.5 text-xs text-muted-foreground">
          Saved pipelines for this project. Run here, from Content, or ad-hoc in the Toolbox.
        </p>
        <FlowList />
      </div>
    </ConceptShell>
  ),
};

/* ---- Concept B — flows live in the Toolbox ---- */

export const B_InToolbox: Story = {
  name: "B — In the Toolbox",
  render: () => (
    <ConceptShell
      title="Concept B — Flows live in the Toolbox"
      blurb="A flow is a saved pipeline of tools, so it lives beside the single tools. Create / edit / delete here; run ad-hoc on a file or against the open project. Reached via Toolbox — no Flows rail icon."
      active="toolbox"
    >
      <div className="flex-1 overflow-y-auto bg-background p-6">
        <h2 className="text-lg font-semibold">Toolbox</h2>
        <p className="mt-0.5 text-sm text-muted-foreground">Single tools and saved flows.</p>

        <div className="mt-4 flex gap-1 border-b border-border">
          <div className="border-b-2 border-transparent px-3 py-1.5 text-sm text-muted-foreground">
            Tools
          </div>
          <div className="-mb-px border-b-2 border-primary px-3 py-1.5 text-sm font-medium text-foreground">
            Flows
          </div>
        </div>

        <div className="mt-4 flex items-center justify-between">
          <p className="text-xs text-muted-foreground">A flow chains tools into one saved step.</p>
          <Button size="xs" variant="outline">
            <Plus size={13} /> New flow
          </Button>
        </div>
        <div className="mt-3">
          <FlowList />
        </div>
      </div>
    </ConceptShell>
  ),
};

/* ---- Concept C — flows are a contextual action ---- */

export const C_Contextual: Story = {
  name: "C — Contextual action",
  render: () => (
    <ConceptShell
      title="Concept C — Flows are an action, not a place"
      blurb="There is no Flows destination at all. A 'Run a flow' control wherever you work opens the flow library — to run on what you're looking at, or to create and edit. Shown here as a panel opened from Content."
      active="content"
    >
      <div className="flex flex-1">
        <div className="flex-1 overflow-y-auto bg-background p-6">
          <div className="flex items-center justify-between">
            <h2 className="text-lg font-semibold">Content</h2>
            <Button size="xs" className="gap-1">
              <Workflow size={12} /> Run a flow
              <ChevronDown size={12} />
            </Button>
          </div>
          <div className="mt-4 space-y-1.5">
            {["home.en-US.json", "about.en-US.json", "pricing.en-US.json", "faq.en-US.json"].map(
              (f) => (
                <div
                  key={f}
                  className="flex items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm"
                >
                  <FileText size={14} className="text-muted-foreground" />
                  <span className="font-mono text-xs">{f}</span>
                </div>
              ),
            )}
          </div>
        </div>

        {/* Flows library, opened as a panel from the "Run a flow" control. */}
        <aside className="w-72 shrink-0 border-l border-border bg-muted/10 p-4">
          <div className="flex items-center justify-between">
            <h3 className="flex items-center gap-1.5 text-sm font-semibold">
              <Workflow size={14} /> Flows
            </h3>
            <Button size="xs" variant="ghost">
              <Plus size={13} /> New
            </Button>
          </div>
          <p className="mt-0.5 text-xs text-muted-foreground">
            Run on the current content, or edit.
          </p>
          <div className="mt-3">
            <FlowList compact />
          </div>
          <button type="button" className="mt-3 text-xs font-medium text-primary hover:underline">
            Manage flows…
          </button>
        </aside>
      </div>
    </ConceptShell>
  ),
};

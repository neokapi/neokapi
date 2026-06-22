/**
 * Shared building blocks for the P10 desktop UX prototype.
 *
 * These are throwaway prototype components for visual design review only —
 * they are NOT wired to the backend and must not be imported by production
 * code. They deliberately reuse @neokapi/ui-primitives and the app's design
 * tokens so the prototype reads like the real app.
 *
 * This file is intentionally NOT a *.stories.tsx file so Storybook does not
 * try to index its exports as stories.
 */

import type { ReactNode } from "react";
import {
  BarChart3,
  BookOpen,
  Database,
  FileText,
  FolderKanban,
  Home,
  Languages,
  Palette,
  PenLine,
  Plus,
  Settings,
  ShieldCheck,
  SlidersHorizontal,
} from "lucide-react";
import { Badge, LocalePill } from "@neokapi/ui-primitives";

const SW = 1.5;

/* ------------------------------------------------------------------ *
 * Window chrome
 * ------------------------------------------------------------------ */

export interface DesktopFrameProps {
  title: string;
  /** Optional badge shown next to the title (e.g. the project type). */
  badge?: ReactNode;
  children: ReactNode;
  className?: string;
}

/** A light desktop-window chrome so prototype panes read as the real app. */
export function DesktopFrame({ title, badge, children, className }: DesktopFrameProps) {
  return (
    <div
      className={`overflow-hidden rounded-xl border border-border bg-background shadow-sm ${className ?? ""}`}
    >
      <div className="flex h-9 items-center gap-2 border-b border-border bg-muted/30 px-3">
        <div className="flex items-center gap-1.5">
          <span className="size-2.5 rounded-full bg-muted-foreground/25" />
          <span className="size-2.5 rounded-full bg-muted-foreground/25" />
          <span className="size-2.5 rounded-full bg-muted-foreground/25" />
        </div>
        <div className="flex flex-1 items-center justify-center gap-2 text-xs text-muted-foreground">
          <span className="font-medium text-foreground/80">{title}</span>
          {badge}
        </div>
        <div className="w-12" />
      </div>
      {children}
    </div>
  );
}

/* ------------------------------------------------------------------ *
 * Adaptive sidebar
 * ------------------------------------------------------------------ */

interface NavRow {
  icon: ReactNode;
  label: string;
  view: string;
}

const contentRows: NavRow[] = [
  { view: "content", label: "Content", icon: <FileText size={17} strokeWidth={SW} /> },
  { view: "check", label: "Check", icon: <ShieldCheck size={17} strokeWidth={SW} /> },
  { view: "rewrite", label: "Rewrite", icon: <PenLine size={17} strokeWidth={SW} /> },
  { view: "stats", label: "Stats", icon: <BarChart3 size={17} strokeWidth={SW} /> },
  { view: "brand", label: "Brand", icon: <Palette size={17} strokeWidth={SW} /> },
];

const localizationRows: NavRow[] = [
  { view: "translate", label: "Translate", icon: <Languages size={17} strokeWidth={SW} /> },
  {
    view: "memories",
    label: "Translation Memories",
    icon: <Database size={17} strokeWidth={SW} />,
  },
  { view: "termbases", label: "Termbases", icon: <BookOpen size={17} strokeWidth={SW} /> },
];

function NavButton({ row, active }: { row: NavRow; active: boolean }) {
  return (
    <button
      type="button"
      title={row.label}
      className={`flex w-full items-center gap-2.5 rounded-lg px-2.5 py-1.5 text-sm transition-colors ${
        active
          ? "bg-primary text-primary-foreground"
          : "text-muted-foreground hover:bg-accent hover:text-foreground"
      }`}
    >
      <span className="shrink-0">{row.icon}</span>
      <span className="truncate">{row.label}</span>
    </button>
  );
}

function GroupLabel({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <div
      className={`px-2.5 pb-1 pt-1 text-[10px] font-semibold uppercase tracking-wider ${className ?? "text-muted-foreground/70"}`}
    >
      {children}
    </div>
  );
}

export interface AdaptiveSidebarProps {
  /** Project display name shown in the sidebar header. */
  project: string;
  /** Whether the project has enabled the localization feature/plugin. */
  localization: boolean;
  /** Currently active view. */
  active?: string;
}

/**
 * The adaptive project sidebar. Content items always show; the grouped
 * Localization set only appears when the project enabled the localization
 * feature — and renders inside a faintly lit container so it reads as a
 * module that "switched on".
 */
export function AdaptiveSidebar({
  project,
  localization,
  active = "content",
}: AdaptiveSidebarProps) {
  return (
    <aside className="flex w-56 shrink-0 flex-col border-r border-border bg-muted/20">
      {/* Project identity */}
      <div className="flex items-center gap-2.5 border-b border-border px-3 py-3">
        <div className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
          <FolderKanban size={16} strokeWidth={SW} />
        </div>
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-semibold">{project}</div>
          <div className="text-[11px] text-muted-foreground">
            {localization ? "Localization project" : "Content project"}
          </div>
        </div>
      </div>

      <nav className="flex flex-1 flex-col gap-0.5 overflow-y-auto p-2">
        <NavButton
          row={{ view: "home", label: "Home", icon: <Home size={17} strokeWidth={SW} /> }}
          active={active === "home"}
        />

        <GroupLabel className="mt-2 text-muted-foreground/70">Workspace</GroupLabel>
        {contentRows.map((row) => (
          <NavButton key={row.view} row={row} active={active === row.view} />
        ))}

        {localization && (
          <div className="mt-3 rounded-xl border border-primary/15 bg-primary/[0.04] p-1.5">
            <GroupLabel className="flex items-center gap-1.5 text-primary/80">
              <Languages size={11} strokeWidth={2} />
              Localization
            </GroupLabel>
            {localizationRows.map((row) => (
              <NavButton key={row.view} row={row} active={active === row.view} />
            ))}
          </div>
        )}
      </nav>

      <div className="flex flex-col gap-0.5 border-t border-border p-2">
        <NavButton
          row={{
            view: "project-settings",
            label: "Project Settings",
            icon: <SlidersHorizontal size={17} strokeWidth={SW} />,
          }}
          active={active === "project-settings"}
        />
        <NavButton
          row={{
            view: "app-settings",
            label: "App Settings",
            icon: <Settings size={17} strokeWidth={SW} />,
          }}
          active={active === "app-settings"}
        />
      </div>
    </aside>
  );
}

/* ------------------------------------------------------------------ *
 * v2 — source-first (one project shape, languages as a dial)
 *
 * The v2 prototype drops the "content project vs localization project" fork.
 * There is one project; localization is a "Languages" dial you turn up. The
 * Localization group is ALWAYS present in the sidebar — an empty-state
 * "Add a language" CTA until a language is added, then the active surface.
 * ------------------------------------------------------------------ */

export interface SourceFirstSidebarProps {
  /** Project display name shown in the sidebar header. */
  project: string;
  /** Target languages on the project. Empty = source-only (monolingual). */
  languages: string[];
  /** Currently active view. */
  active?: string;
  /** Empty-state CTA handler (prototype: adds a language). */
  onAddLanguage?: () => void;
}

/**
 * The v2 source-first sidebar. One project shape: the content workspace always
 * shows, and the Localization group is always rendered — as an "Add a language"
 * CTA when the project has no languages, or the active Translate / TM / Termbase
 * surface once one is added. No project-kind label; the header carries a
 * languages-count state instead.
 */
export function SourceFirstSidebar({
  project,
  languages,
  active = "content",
  onAddLanguage,
}: SourceFirstSidebarProps) {
  const hasLangs = languages.length > 0;
  return (
    <aside className="flex w-56 shrink-0 flex-col border-r border-border bg-muted/20">
      {/* Project identity — a languages state, never a "kind". */}
      <div className="flex items-center gap-2.5 border-b border-border px-3 py-3">
        <div className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
          <FolderKanban size={16} strokeWidth={SW} />
        </div>
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-semibold">{project}</div>
          <div className="text-[11px] text-muted-foreground">
            {hasLangs
              ? `${languages.length} language${languages.length > 1 ? "s" : ""}`
              : "No languages yet"}
          </div>
        </div>
      </div>

      <nav className="flex flex-1 flex-col gap-0.5 overflow-y-auto p-2">
        <NavButton
          row={{ view: "home", label: "Home", icon: <Home size={17} strokeWidth={SW} /> }}
          active={active === "home"}
        />

        <GroupLabel className="mt-2 text-muted-foreground/70">Workspace</GroupLabel>
        {contentRows.map((row) => (
          <NavButton key={row.view} row={row} active={active === row.view} />
        ))}

        {/* Localization is ALWAYS present — CTA when empty, active surface when not. */}
        <div className="mt-3 rounded-xl border border-primary/15 bg-primary/[0.04] p-1.5">
          <GroupLabel className="flex items-center gap-1.5 text-primary/80">
            <Languages size={11} strokeWidth={2} />
            Localization
          </GroupLabel>
          {hasLangs ? (
            <>
              <div className="flex flex-wrap gap-1 px-2.5 pb-1.5 pt-0.5">
                {languages.map((l) => (
                  <LocalePill key={l} locale={l} />
                ))}
              </div>
              {localizationRows.map((row) => (
                <NavButton key={row.view} row={row} active={active === row.view} />
              ))}
            </>
          ) : (
            <div className="px-1 pb-0.5 pt-0.5">
              <button
                type="button"
                onClick={onAddLanguage}
                className="flex w-full items-center gap-2 rounded-lg border border-dashed border-primary/30 px-2.5 py-1.5 text-sm text-primary transition-colors hover:bg-primary/[0.06]"
              >
                <Plus size={15} strokeWidth={SW} />
                Add a language
              </button>
              <p className="px-1.5 pt-1.5 text-[10px] leading-snug text-muted-foreground/80">
                Turns on Translate, Translation Memory, and Termbases.
              </p>
            </div>
          )}
        </div>
      </nav>

      <div className="flex flex-col gap-0.5 border-t border-border p-2">
        <NavButton
          row={{
            view: "project-settings",
            label: "Project Settings",
            icon: <SlidersHorizontal size={17} strokeWidth={SW} />,
          }}
          active={active === "project-settings"}
        />
        <NavButton
          row={{
            view: "app-settings",
            label: "App Settings",
            icon: <Settings size={17} strokeWidth={SW} />,
          }}
          active={active === "app-settings"}
        />
      </div>
    </aside>
  );
}

/**
 * A languages-state chip for recents and window chrome — a derived state, not a
 * project category. Source-only reads affirmatively ("Source only"), not as an
 * absence.
 */
export function LanguagesChip({ targets }: { targets: string[] }) {
  return targets.length > 0 ? (
    <Badge variant="secondary" className="gap-1 text-[10px]">
      <Languages size={10} />
      {targets.length} language{targets.length > 1 ? "s" : ""}
    </Badge>
  ) : (
    <Badge variant="outline" className="gap-1 text-[10px] text-muted-foreground">
      Source only
    </Badge>
  );
}

/* ------------------------------------------------------------------ *
 * Journey card (used by Launcher + NewProject)
 * ------------------------------------------------------------------ */

export interface JourneyCardProps {
  icon: ReactNode;
  eyebrow: string;
  title: string;
  description: string;
  chips: ReactNode;
  /** Footer content — capability list or a continue affordance. */
  footer?: ReactNode;
  selected?: boolean;
  onClick?: () => void;
}

/** A large, co-equal onboarding journey card. */
export function JourneyCard({
  icon,
  eyebrow,
  title,
  description,
  chips,
  footer,
  selected = false,
  onClick,
}: JourneyCardProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`group flex h-full flex-col rounded-2xl border p-6 text-left transition-colors ${
        selected
          ? "border-primary/50 bg-primary/[0.06] ring-1 ring-primary/30"
          : "border-border bg-card hover:border-primary/40 hover:bg-accent/30"
      }`}
    >
      <div className="mb-4 flex size-12 items-center justify-center rounded-xl bg-primary/10 text-primary">
        {icon}
      </div>
      <div className="mb-1 text-[11px] font-semibold uppercase tracking-wider text-primary/70">
        {eyebrow}
      </div>
      <div className="text-lg font-semibold">{title}</div>
      <p className="mt-1.5 text-sm leading-relaxed text-muted-foreground">{description}</p>
      <div className="mt-4 flex flex-wrap gap-1.5">{chips}</div>
      {footer && <div className="mt-5 border-t border-border/70 pt-4">{footer}</div>}
    </button>
  );
}

/** A small capability pill used inside journey cards and tool tiles. */
export function Chip({ children }: { children: ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1 rounded-full border border-border bg-muted/40 px-2 py-0.5 text-[11px] text-muted-foreground">
      {children}
    </span>
  );
}

/* ------------------------------------------------------------------ *
 * Mock content
 * ------------------------------------------------------------------ */

export interface RecentProject {
  name: string;
  path: string;
  kind: "content" | "localization";
  langs?: { source: string; targets: string[] };
}

export const recentProjects: RecentProject[] = [
  {
    name: "Acme Marketing Site",
    path: "~/work/acme-site",
    kind: "localization",
    langs: { source: "en-US", targets: ["fr-FR", "de-DE", "ja-JP"] },
  },
  {
    name: "Help Center Articles",
    path: "~/work/help-center",
    kind: "content",
  },
  {
    name: "Mobile App Strings",
    path: "~/work/mobile-strings",
    kind: "localization",
    langs: { source: "en-US", targets: ["es-ES", "pt-BR"] },
  },
  {
    name: "Release Notes Q2",
    path: "~/work/release-notes",
    kind: "content",
  },
];

/** A small project-type tag for recents lists. */
export function ProjectKindBadge({ kind }: { kind: RecentProject["kind"] }) {
  return kind === "localization" ? (
    <Badge variant="secondary" className="gap-1 text-[10px]">
      <Languages size={10} />
      Localization
    </Badge>
  ) : (
    <Badge variant="outline" className="gap-1 text-[10px]">
      <FileText size={10} />
      Content
    </Badge>
  );
}

/** Render a source → targets locale row. */
export function LocaleRoute({ source, targets }: { source: string; targets: string[] }) {
  return (
    <span className="flex flex-wrap items-center gap-1">
      <LocalePill locale={source} />
      <span className="text-muted-foreground">&rarr;</span>
      {targets.map((l) => (
        <LocalePill key={l} locale={l} />
      ))}
    </span>
  );
}

export interface FlowDef {
  name: string;
  description: string;
  steps: string[];
  kind: "content" | "localization";
}

export const configuredFlows: FlowDef[] = [
  {
    name: "Brand check & rewrite",
    description: "Score every draft against the brand voice and rewrite anything off-message.",
    steps: ["brand-check", "rewrite", "report"],
    kind: "content",
  },
  {
    name: "Translate & QA",
    description: "Translate from TM + AI, then run terminology and placeholder QA on the targets.",
    steps: ["recycle", "translate", "qa-check"],
    kind: "localization",
  },
  {
    name: "Pseudo-localize",
    description: "Expand and accent the source to surface hard-coded strings and layout breaks.",
    steps: ["pseudo-translate"],
    kind: "localization",
  },
];

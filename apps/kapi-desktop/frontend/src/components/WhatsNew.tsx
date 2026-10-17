// What is new across the workspace, for the home screen.
//
// One row per project with something to decide about, newest session first:
// the mark of whoever noticed something most recently, the project and when,
// a chip for each kind of thing noticed, and one button that opens the project
// on its own What's new. j and k (or the arrows) move over the rows and Enter
// opens the one under the cursor.
//
// Above it, WorkspaceNumbers draws the workspace as four labelled counts.

import { useEffect, useMemo, useState } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  ActorMark,
  Badge,
  Button,
  KindIcon,
  SectionHeading,
  StatRow,
  THEME_KINDS,
  When,
  cn,
  type DigestKind,
  type StatItem,
} from "@neokapi/ui-primitives";
import type { ContextActivity, ContextProjectStanding } from "../types/api";

export interface WhatsNewProps {
  /** Every project's standing; null while the workspace is being read. */
  standings: ContextProjectStanding[] | null;
  /** Open a project on its What's new. */
  onOpen: (key: string) => void;
  /** Take the keyboard. */
  keyboard?: boolean;
}

/** The digest's theme order, which the chips follow. */
const THEMES = ["names", "words", "writing", "memory"] as const;

type Theme = (typeof THEMES)[number];

/**
 * The projects with something to decide about, newest session first: what the
 * person has not seen, a disagreement, suggestions still advising, or an
 * assistant working there now. A project opened on its What's new moves the
 * marker at once, so "new" alone would drop a project whose suggestions still
 * wait.
 */
export function projectsWithNews(standings: ContextProjectStanding[]): ContextProjectStanding[] {
  return standings
    .filter((s) => s.new > 0 || s.conflicts > 0 || s.suggested > 0 || !!s.latest?.working)
    .sort((a, b) => (b.latest?.at ?? "").localeCompare(a.latest?.at ?? ""));
}

/** Whether any project holds anything at all: a rule, a suggestion, a disagreement. */
function anythingRecorded(standings: ContextProjectStanding[]): boolean {
  return standings.some((s) => s.rules > 0 || s.suggested > 0 || s.conflicts > 0 || s.new > 0);
}

export function WhatsNew({ standings, onOpen, keyboard = true }: WhatsNewProps) {
  const rows = useMemo(() => projectsWithNews(standings ?? []), [standings]);
  const [cursor, setCursor] = useState(0);
  const active = rows[Math.min(cursor, rows.length - 1)];

  useEffect(() => {
    if (!keyboard || rows.length === 0) return;
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      const tag = el?.tagName ?? "";
      if (tag === "TEXTAREA" || tag === "INPUT" || tag === "SELECT" || el?.isContentEditable) {
        return;
      }
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      switch (e.key) {
        case "j":
        case "ArrowDown":
          e.preventDefault();
          setCursor((at) => Math.min(at + 1, rows.length - 1));
          break;
        case "k":
        case "ArrowUp":
          e.preventDefault();
          setCursor((at) => Math.max(at - 1, 0));
          break;
        case "Enter":
          // A focused button or link answers Enter itself.
          if (tag === "BUTTON" || tag === "A" || !active) return;
          e.preventDefault();
          onOpen(active.project);
          break;
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [active, keyboard, onOpen, rows.length]);

  if (!standings) return null;

  return (
    <section data-slot="whats-new" aria-labelledby="whats-new-heading">
      <div id="whats-new-heading">
        <SectionHeading>What&apos;s new</SectionHeading>
      </div>
      {rows.length > 0 ? (
        <div className="mt-1.5 divide-y divide-border/50">
          {rows.map((row, i) => (
            <WhatsNewRow
              key={row.project}
              standing={row}
              active={active?.project === row.project}
              onFocus={() => setCursor(i)}
              onOpen={() => onOpen(row.project)}
            />
          ))}
        </div>
      ) : (
        <p data-slot="whats-new-empty" className="mt-2 text-sm text-muted-foreground">
          {anythingRecorded(standings) ? (
            <>Nothing new since you last looked.</>
          ) : (
            <>Nothing is recorded yet. What an assistant notices in a project appears here.</>
          )}
        </p>
      )}
    </section>
  );
}

function WhatsNewRow({
  standing,
  active,
  onFocus,
  onOpen,
}: {
  standing: ContextProjectStanding;
  active: boolean;
  onFocus: () => void;
  onOpen: () => void;
}) {
  const latest = standing.latest;
  const chips = newsChips(standing);
  return (
    <div
      data-slot="whats-new-row"
      data-project={standing.project}
      data-active={active || undefined}
      onClick={onFocus}
      className={cn("flex h-11 items-center gap-3 rounded-md px-2", active && "bg-accent/50")}
    >
      {latest ? (
        <ActorMark data-slot="whats-new-actor" {...markActor(latest.actor)} />
      ) : (
        <span aria-hidden className="size-[22px] shrink-0" />
      )}
      <div className="flex min-w-0 flex-1 items-baseline gap-1.5">
        <span className="truncate text-sm" translate="no">
          {standing.project_name || standing.project}
        </span>
        {latest && (
          <>
            <span aria-hidden className="text-[11px] text-muted-foreground">
              ·
            </span>
            <When
              iso={latest.at}
              relative
              className="shrink-0 text-[11px] whitespace-nowrap text-muted-foreground"
            />
          </>
        )}
      </div>
      {latest?.working && (
        <Badge
          variant="outline"
          data-slot="workspace-working"
          className="text-[11px] font-normal text-muted-foreground"
        >
          <span
            aria-hidden
            className="size-1.5 rounded-full bg-success motion-safe:animate-pulse"
          />
          {t("{actor} is working", "a chip: an assistant is working in the project now", {
            actor: actorName(latest.actor),
          })}
        </Badge>
      )}
      {chips.length > 0 && (
        <div data-slot="whats-new-chips" className="flex shrink-0 items-center gap-1.5">
          {/* The label is translated already, by newsChips. */}
          {chips.map(({ kind, label, warning }) => (
            <Badge
              key={kind}
              variant={warning ? "warning" : "outline"}
              data-slot="whats-new-chip"
              data-kind={kind}
              className={cn(
                "text-[11px] font-normal tabular-nums",
                !warning && "text-muted-foreground",
              )}
            >
              {isDigestKind(kind) && <KindIcon kind={kind} decorative />}
              {label}
            </Badge>
          ))}
        </div>
      )}
      <Button
        size="sm"
        variant="outline"
        data-slot="whats-new-open"
        className="min-w-[4.5rem] shrink-0"
        onClick={(e) => {
          e.stopPropagation();
          onOpen();
        }}
      >
        {standing.conflicts > 0 ? <>Decide</> : <>Review</>}
      </Button>
    </div>
  );
}

/** One chip on a What's new row: a count of one kind of thing. */
export interface NewsChip {
  /**
   * The digest kind it counts. `new` and `suggested` count items whose kind
   * the latest session does not say.
   */
  kind: DigestKind | "new" | "suggested";
  count: number;
  /** The count and its noun, in the UI language: "3 names". */
  label: string;
  /** It waits for a person. */
  warning?: boolean;
}

function isDigestKind(kind: NewsChip["kind"]): kind is DigestKind {
  return kind !== "new" && kind !== "suggested";
}

/**
 * What a project's row counts, in the digest's theme order: what the latest
 * session noticed by kind, then the disagreements that need a person. A
 * project whose latest session noticed nothing new still says how much is new,
 * or how many suggestions are advising, in one chip.
 */
export function newsChips(standing: ContextProjectStanding): NewsChip[] {
  const chips: NewsChip[] = [];
  const counts = standing.latest?.new_by_theme ?? {};
  for (const theme of THEMES) {
    const count = counts[theme] ?? 0;
    if (count > 0) chips.push({ kind: THEME_KINDS[theme], count, label: themePart(theme, count) });
  }
  if (chips.length === 0) {
    if (standing.new > 0) {
      chips.push({
        kind: "new",
        count: standing.new,
        label: t("{count} new", { count: standing.new }),
      });
    } else if (standing.suggested > 0) {
      chips.push({
        kind: "suggested",
        count: standing.suggested,
        label: t("{count, plural, one {# suggestion} other {# suggestions}}", {
          count: standing.suggested,
        }),
      });
    }
  }
  if (standing.conflicts > 0) {
    chips.push({
      kind: "conflict",
      count: standing.conflicts,
      label: t("{count, plural, one {# needs you} other {# need you}}", {
        count: standing.conflicts,
      }),
      warning: true,
    });
  }
  return chips;
}

function themePart(theme: Theme, count: number): string {
  switch (theme) {
    case "names":
      return t("{count, plural, one {# name} other {# names}}", { count });
    case "words":
      return t("{count, plural, one {# word} other {# words}}", { count });
    case "writing":
      return t("{count, plural, one {# note} other {# notes}}", { count });
    case "memory":
      return t("{count, plural, one {# wording} other {# wordings}}", { count });
  }
}

/** Who a session belongs to, as a chip names them. */
export function actorName(actor: ContextActivity["actor"]): string {
  if (actor.kind === "agent") return actor.name || t("An agent");
  if (actor.kind === "tool") return actor.name || t("A tool");
  return actor.name || t("You");
}

/**
 * The mark a session's actor is drawn with: a person by their name, and an
 * agent or a tool by its own name, which gives a known assistant its letters
 * and hue.
 */
export function markActor(actor: ContextActivity["actor"]): { actor: string; name?: string } {
  if (actor.kind === "person") return { actor: "person", name: actor.name };
  return { actor: actor.name || actor.kind };
}

/**
 * The workspace as labelled counts: projects, rules in force, the concepts in
 * the projects' terms stores, suggestions advising, and the disagreements that
 * need a person, in the warning ink while there are any.
 */
export function WorkspaceNumbers({
  standings,
  projects,
}: {
  standings: ContextProjectStanding[] | null;
  /** How many projects the workspace holds. Absent counts the standings. */
  projects?: number;
}) {
  if (!standings) return null;
  const rules = standings.reduce((n, s) => n + s.rules, 0);
  const terms = standings.reduce((n, s) => n + s.terms, 0);
  const suggested = standings.reduce((n, s) => n + s.suggested, 0);
  const conflicts = standings.reduce((n, s) => n + s.conflicts, 0);
  const count = projects ?? standings.length;
  const items: StatItem[] = [
    {
      id: "projects",
      value: count,
      label: t("{count, plural, one {project} other {projects}}", "a count on the home", {
        count,
      }),
    },
    {
      id: "rules",
      value: rules,
      label: t("{count, plural, one {rule} other {rules}}", "a count on the home", {
        count: rules,
      }),
    },
    {
      id: "terms",
      value: terms,
      label: t("{count, plural, one {term} other {terms}}", "a count on the home", {
        count: terms,
      }),
    },
    {
      id: "suggestions",
      value: suggested,
      label: t("{count, plural, one {suggestion} other {suggestions}}", "a count on the home", {
        count: suggested,
      }),
      hint: t(
        "A suggestion advises until you keep it or drop it. A check reports it and fails nothing.",
      ),
    },
    {
      id: "needs-you",
      value: conflicts,
      label: t("{count, plural, one {needs you} other {need you}}", "a count on the home", {
        count: conflicts,
      }),
      tone: conflicts > 0 ? "warning" : "muted",
      hint: t("Disagreements wait for you to choose the side that holds."),
    },
  ];
  return <StatRow data-slot="workspace-numbers" items={items} />;
}

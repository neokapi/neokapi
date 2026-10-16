// What is new across the workspace, for the home screen.
//
// One row per project with suggestions still advising, something the person
// has not seen, or a disagreement that needs them, newest session first. The row says how much
// is new, who noticed it and what kind of thing it is, and its button opens
// the project on its own What's new. j and k (or the arrows) move over the
// rows and Enter opens the one under the cursor.
//
// Above it, WorkspaceNumbers states the workspace in one sentence, and a
// second when an agent session is working in a project now.

import { useEffect, useMemo, useState } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { Button, SectionHeading, cn } from "@neokapi/ui-primitives";
import { formatLook } from "./ContextDigest";
import type { ContextActivity, ContextProjectStanding } from "../types/api";

export interface WhatsNewProps {
  /** Every project's standing; null while the workspace is being read. */
  standings: ContextProjectStanding[] | null;
  /** Open a project on its What's new. */
  onOpen: (key: string) => void;
  /** Take the keyboard. */
  keyboard?: boolean;
}

/** The digest's theme order, which the sentence about a session follows. */
const THEMES = ["names", "words", "writing", "memory"] as const;

/**
 * The projects with something to decide about, newest session first: what the
 * person has not seen, a disagreement, or suggestions still advising. A
 * project opened on its What's new moves the marker at once, so "new" alone
 * would drop a project whose suggestions still wait.
 */
export function projectsWithNews(standings: ContextProjectStanding[]): ContextProjectStanding[] {
  return standings
    .filter((s) => s.new > 0 || s.conflicts > 0 || s.suggested > 0)
    .sort((a, b) => (b.latest?.at ?? "").localeCompare(a.latest?.at ?? ""));
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

  if (rows.length === 0) return null;

  return (
    <section data-slot="whats-new" aria-labelledby="whats-new-heading">
      <div id="whats-new-heading">
        <SectionHeading>What&apos;s new</SectionHeading>
      </div>
      <div className="mt-2 space-y-0.5">
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
  const working = !!standing.latest?.working;
  const themes = themesOf(standing.latest);
  return (
    <div
      data-slot="whats-new-row"
      data-project={standing.project}
      data-active={active || undefined}
      onClick={onFocus}
      className={cn("flex items-start gap-3 rounded-md px-2 py-2.5", active && "bg-accent/50")}
    >
      <span
        data-slot="whats-new-dot"
        data-working={working || undefined}
        aria-hidden
        className={cn(
          "mt-[7px] size-1.5 shrink-0 rounded-full bg-primary",
          working && "motion-safe:animate-pulse",
        )}
      />
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium" translate="no">
          {standing.project_name || standing.project}
        </p>
        <p data-slot="whats-new-sentence" className="text-sm text-muted-foreground">
          {standing.new > 0 &&
            (standing.since
              ? t("{count} new since {when}.", {
                  count: standing.new,
                  when: formatLook(standing.since),
                })
              : t("{count} new.", { count: standing.new }))}
          {standing.new === 0 &&
            standing.suggested > 0 &&
            t("{count, plural, one {# suggestion is} other {# suggestions are}} advising.", {
              count: standing.suggested,
            })}
          {standing.latest && (themes || standing.latest.recorded > 0) && (
            <>
              {" "}
              {themes
                ? t("{actor} noticed {themes}.", {
                    actor: actorName(standing.latest.actor),
                    themes,
                  })
                : t("{actor} recorded {count, plural, one {# note} other {# notes}}.", {
                    actor: actorName(standing.latest.actor),
                    count: standing.latest.recorded,
                  })}
            </>
          )}
          {standing.conflicts > 0 && (
            <>
              {(standing.new > 0 || standing.suggested > 0) && " "}
              <span className="text-amber-700 dark:text-amber-500">
                {t("{count, plural, one {# needs you} other {# need you}}.", {
                  count: standing.conflicts,
                })}
              </span>
            </>
          )}
        </p>
      </div>
      <Button
        size="sm"
        variant="outline"
        data-slot="whats-new-open"
        className="shrink-0"
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

/** Who a session belongs to, as the sentence names them. */
export function actorName(actor: ContextActivity["actor"]): string {
  if (actor.kind === "agent") return actor.name || t("An agent");
  if (actor.kind === "tool") return actor.name || t("A tool");
  return actor.name || t("You");
}

/**
 * What a session noticed, in the digest's theme order: "3 names and 1 word",
 * "2 names, 1 word and 1 note". Empty when it noticed nothing new.
 */
export function themesOf(latest?: ContextActivity): string {
  const counts = latest?.new_by_theme ?? {};
  const parts: string[] = [];
  for (const theme of THEMES) {
    const count = counts[theme] ?? 0;
    if (count > 0) parts.push(themePart(theme, count));
  }
  if (parts.length === 0) return "";
  if (parts.length === 1) return parts[0];
  return t("{list} and {last}", {
    list: parts.slice(0, -1).join(", "),
    last: parts[parts.length - 1],
  });
}

function themePart(theme: (typeof THEMES)[number], count: number): string {
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

/**
 * The workspace in one sentence: "kapi knows 61 rules across 5 projects.
 * 7 suggestions are advising." and, while an agent session is working, who
 * is working where.
 */
export function WorkspaceNumbers({ standings }: { standings: ContextProjectStanding[] | null }) {
  if (!standings) return null;
  const rules = standings.reduce((n, s) => n + s.rules, 0);
  const suggested = standings.reduce((n, s) => n + s.suggested, 0);
  const working = standings.filter((s) => s.latest?.working);
  return (
    <div data-slot="workspace-numbers" className="space-y-0.5 text-sm">
      {rules === 0 && suggested === 0 ? (
        <p>Nothing is recorded yet.</p>
      ) : (
        <p>
          {t(
            "kapi knows {rules, plural, one {# rule} other {# rules}} across {projects, plural, one {# project} other {# projects}}.",
            { rules, projects: standings.length },
          )}
          {suggested > 0 && (
            <>
              {" "}
              {t("{suggested, plural, one {# suggestion is} other {# suggestions are}} advising.", {
                suggested,
              })}
            </>
          )}
        </p>
      )}
      {working.map((s) => (
        <p key={s.project} data-slot="workspace-working" className="text-muted-foreground">
          {t("{actor} is working in {project} now.", {
            actor: actorName(s.latest!.actor),
            project: s.project_name || s.project,
          })}
        </p>
      ))}
    </div>
  );
}

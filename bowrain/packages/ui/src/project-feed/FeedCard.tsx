// One card of the project feed on the web, the twin of the mobile app's: the
// actor's avatar, their name in bold with what they did and when, and a
// preview of the change. One actor's changes of one kind close together fold
// into one card that expands. A coverage milestone is a card of its own. The
// card leaves room for a footer action row below the preview.
import { useState } from "react";
import { t, useNeokapi } from "@neokapi/i18n-react/runtime";
import {
  initialsOf,
  paletteIndex,
  targetChanges,
  type FeedActor,
  type FeedCard as Card,
  type FeedEntry,
} from "@neokapi/bowrain-client";
import { cn } from "@neokapi/ui-primitives";
import { ChevronDown, ChevronUp, GitBranch, Settings, Sparkles } from "../components/icons";
import {
  actionText,
  actorName,
  ageText,
  countText,
  foldedActionText,
  languageName,
  stateLabel,
} from "./labels";

// Spelled out in full so Tailwind sees every class.
const PERSON_COLOURS = [
  "bg-prism-1",
  "bg-prism-2",
  "bg-prism-3",
  "bg-prism-4",
  "bg-prism-5",
  "bg-primary",
  "bg-chart-1",
  "bg-chart-3",
];

const PRISM_FILLS = ["bg-prism-4", "bg-prism-3", "bg-prism-5", "bg-prism-2", "bg-prism-1"];

/** A person's initials in their colour, an agent's spark, a tool's gear. */
export function FeedAvatar({
  actor,
  name,
  size = 40,
}: {
  actor: FeedActor;
  name: string;
  size?: number;
}) {
  const style = { width: size, height: size };
  if (actor.kind === "agent") {
    return (
      <span
        aria-hidden
        style={style}
        className="inline-flex shrink-0 items-center justify-center rounded-xl bg-linear-to-br from-prism-4 to-prism-5 text-white"
      >
        <Sparkles className="size-1/2" />
      </span>
    );
  }
  if (actor.kind === "tool") {
    return (
      <span
        aria-hidden
        style={style}
        className="inline-flex shrink-0 items-center justify-center rounded-full border border-border bg-muted text-muted-foreground"
      >
        <Settings className="size-1/2" />
      </span>
    );
  }
  const colour = PERSON_COLOURS[paletteIndex(actor.id || name, PERSON_COLOURS.length)];
  return (
    <span
      aria-hidden
      style={style}
      className={cn(
        "inline-flex shrink-0 items-center justify-center rounded-full text-sm font-semibold text-white",
        colour,
      )}
    >
      {initialsOf(name)}
    </span>
  );
}

type ChipTone = "neutral" | "added" | "removed" | "accent";

const CHIP_TONES: Record<ChipTone, string> = {
  neutral: "bg-muted text-foreground",
  added: "bg-success/15 text-foreground",
  removed: "bg-destructive/15 text-foreground",
  accent: "bg-primary/10 text-foreground",
};

function Chip({
  children,
  tone = "neutral",
  onClick,
}: {
  children: React.ReactNode;
  tone?: ChipTone;
  onClick?: () => void;
}) {
  const cls = cn(
    "inline-flex max-w-full items-center gap-1 truncate rounded-full px-3 py-1 text-[13px] tabular-nums",
    CHIP_TONES[tone],
  );
  if (!onClick) return <span className={cls}>{children}</span>;
  return (
    <button
      type="button"
      data-slot="feed-chip-link"
      className={cn(
        cls,
        "hover:ring-1 hover:ring-ring focus-visible:ring-2 focus-visible:ring-ring",
      )}
      onClick={(ev) => {
        ev.stopPropagation();
        onClick();
      }}
    >
      {children}
    </button>
  );
}

function SplitBar({
  segments,
}: {
  segments: { key: string; value: number; colour: string; label: string }[];
}) {
  const shown = segments.filter((s) => s.value > 0);
  if (shown.length === 0) return null;
  return (
    <div className="space-y-2">
      <div className="flex h-2.5 overflow-hidden rounded-full bg-muted">
        {shown.map((s) => (
          <div key={s.key} className={cn("h-full", s.colour)} style={{ flexGrow: s.value }} />
        ))}
      </div>
      <div className="flex flex-wrap gap-x-4 gap-y-1 text-[13px] text-muted-foreground tabular-nums">
        {shown.map((s) => (
          <span key={s.key} className="inline-flex items-center gap-1.5">
            <span className={cn("size-2 rounded-full", s.colour)} />
            {s.label} {s.value}
          </span>
        ))}
      </div>
    </div>
  );
}

function axes(c: Record<string, string> | undefined): string {
  return Object.values(c ?? {}).join(" · ");
}

const CONTEXT_OPS = ["add", "edit", "keep", "drop", "retire"];

function standingLabel(state: string): string {
  switch (state) {
    case "shippable":
      return t("Ready", "language standing");
    case "parked":
      return t("Parked", "language standing");
    case "pending":
      return t("In progress", "language standing");
    default:
      return state;
  }
}

function opLabel(op: string): string {
  switch (op) {
    case "concept.create":
      return t("concepts added");
    case "concept.update":
      return t("concepts edited");
    case "concept.delete":
      return t("concepts removed");
    case "term.add":
      return t("terms added");
    case "term.update":
      return t("terms edited");
    case "term.remove":
      return t("terms removed");
    case "term.status":
      return t("status changes");
    case "relation.add":
      return t("relations added");
    case "relation.remove":
      return t("relations removed");
    default:
      return op;
  }
}

/** The body of a card: what the change looked like. */
export function FeedPreview({
  entry,
  onOpenLanguage,
}: {
  entry: FeedEntry;
  onOpenLanguage?: (locale: string) => void;
}) {
  const { locale } = useNeokapi();
  const c = entry.counts ?? {};
  const openLang = (l: string) => (onOpenLanguage ? () => onOpenLanguage(l) : undefined);
  switch (entry.kind) {
    case "push":
      return (
        <div className="flex flex-wrap gap-2">
          {entry.stream && (
            <Chip>
              <GitBranch className="size-3.5 text-muted-foreground" aria-hidden />
              <span translate="no">{entry.stream}</span>
            </Chip>
          )}
          {c.collections ? (
            <Chip>{countText({ key: "collections", n: c.collections }, locale)}</Chip>
          ) : null}
          {c.files !== undefined && <Chip>{countText({ key: "files", n: c.files }, locale)}</Chip>}
          {c.blocks !== undefined && (
            <Chip tone="accent">{countText({ key: "blocks", n: c.blocks }, locale)}</Chip>
          )}
          {c.files_removed ? (
            <Chip tone="removed">
              {t("{count, plural, one {# file removed} other {# files removed}}", {
                count: c.files_removed,
              })}
            </Chip>
          ) : null}
        </div>
      );
    case "convergence_run": {
      const standing = entry.standing ?? [];
      if (standing.length === 0) {
        return (
          <div className="flex flex-wrap gap-2">
            {(entry.locales ?? []).map((l) => (
              <Chip key={l} onClick={openLang(l)}>
                {languageName(l, locale)}
              </Chip>
            ))}
          </div>
        );
      }
      return (
        <div className="space-y-3">
          {standing.map((s, i) => {
            const units = Math.max(s.units ?? 0, s.produced ?? 0);
            const fraction =
              units > 0 ? (s.produced ?? 0) / units : s.state === "shippable" ? 1 : 0;
            return (
              <button
                key={s.locale}
                type="button"
                data-slot="feed-standing"
                className="block w-full space-y-1.5 text-left"
                onClick={(ev) => {
                  ev.stopPropagation();
                  onOpenLanguage?.(s.locale);
                }}
              >
                <span className="flex items-baseline justify-between gap-3 text-sm">
                  <span className="truncate text-foreground">{languageName(s.locale, locale)}</span>
                  <span className="text-[13px] text-muted-foreground tabular-nums">
                    {countText({ key: "produced", n: s.produced ?? 0 }, locale)} ·{" "}
                    {standingLabel(s.state)}
                  </span>
                </span>
                <span className="block h-2 overflow-hidden rounded-full bg-muted">
                  <span
                    className={cn("block h-full rounded-full", PRISM_FILLS[i % PRISM_FILLS.length])}
                    style={{ width: `${Math.max(4, Math.min(100, fraction * 100))}%` }}
                  />
                </span>
              </button>
            );
          })}
        </div>
      );
    }
    case "review_summary": {
      const l = entry.locales?.[0];
      return (
        <div className="space-y-3">
          {l && (
            <div className="flex flex-wrap gap-2">
              <Chip tone="accent" onClick={openLang(l)}>
                {languageName(l, locale)}
              </Chip>
            </div>
          )}
          <SplitBar
            segments={[
              {
                key: "approved",
                value: (c.approved ?? 0) + (c.bulk_approved ?? 0),
                colour: "bg-primary",
                label: t("Approved", "review split"),
              },
              {
                key: "fixed",
                value: c.fixed ?? 0,
                colour: "bg-prism-3",
                label: t("Fixed", "review split"),
              },
              {
                key: "rejected",
                value: c.rejected ?? 0,
                colour: "bg-destructive",
                label: t("Rejected", "review split"),
              },
              {
                key: "unreviewed",
                value: c.unreviewed ?? 0,
                colour: "bg-prism-2",
                label: t("Sent back", "review split"),
              },
            ]}
          />
        </div>
      );
    }
    case "recipe_change": {
      const { added, removed } = targetChanges(entry);
      return (
        <div className="flex flex-wrap gap-2">
          {added.map((l) => (
            <Chip key={`+${l}`} tone="added" onClick={openLang(l)}>
              + {languageName(l, locale)}
            </Chip>
          ))}
          {removed.map((l) => (
            <Chip key={`-${l}`} tone="removed">
              − {languageName(l, locale)}
            </Chip>
          ))}
          {(entry.diff?.collections ?? []).map((col) => (
            <Chip key={`c-${col.name}`} tone="accent">
              <span translate="no">
                {col.before && Object.keys(col.before).length > 0
                  ? `${col.name}: ${axes(col.before)} → ${axes(col.after)}`
                  : `${col.name}: ${axes(col.after)}`}
              </span>
            </Chip>
          ))}
          {(entry.diff?.settings ?? []).map((s) => (
            <Chip key={`s-${s.name}`}>
              <span translate="no">
                {s.name}: {s.after || "–"}
              </span>
            </Chip>
          ))}
        </div>
      );
    }
    case "context_change": {
      const samples = entry.samples ?? [];
      const total = CONTEXT_OPS.reduce((n, op) => n + (c[op] ?? 0), 0);
      const more = Math.max(0, total - samples.length);
      if (samples.length === 0) {
        return (
          <div className="flex flex-wrap gap-2">
            {Object.entries(c)
              .filter(([k, n]) => !CONTEXT_OPS.includes(k) && n > 0)
              .map(([k, n]) => (
                <Chip key={k} tone="accent">
                  {countText({ key: k, n }, locale)}
                </Chip>
              ))}
          </div>
        );
      }
      return (
        <div className="space-y-2">
          {samples.map((s, i) =>
            s.kind === "note" ? (
              <blockquote
                key={`n-${i}`}
                className="border-l-2 border-prism-4 pl-3 text-sm italic text-foreground"
              >
                {s.text}
              </blockquote>
            ) : (
              <div key={`t-${i}`}>
                <span className="inline-flex flex-wrap items-center gap-1.5 rounded-xl bg-primary/10 px-3 py-1.5 text-sm">
                  {s.replacement ? (
                    <>
                      <span className="font-semibold" translate="no">
                        {s.replacement}
                      </span>
                      <span aria-hidden>←</span>
                      <span className="text-muted-foreground line-through" translate="no">
                        {s.term}
                      </span>
                    </>
                  ) : (
                    <span className="font-semibold" translate="no">
                      {s.term}
                    </span>
                  )}
                </span>
              </div>
            ),
          )}
          {more > 0 && (
            <p className="text-[13px] text-muted-foreground">
              {t("+{count} more", { count: more })}
            </p>
          )}
        </div>
      );
    }
    case "change_set": {
      const ops = Object.entries(c).filter(([, n]) => n > 0);
      return (
        <div className="space-y-2">
          <div className="flex items-center gap-2">
            {entry.state && (
              <span
                className={cn(
                  "rounded-md px-2 py-0.5 text-[11px] font-semibold tracking-wide uppercase",
                  entry.state === "merged" || entry.state === "approved"
                    ? "bg-success/15"
                    : entry.state === "rejected"
                      ? "bg-destructive/15"
                      : "bg-muted",
                )}
              >
                {stateLabel(entry.state)}
              </span>
            )}
            {entry.name && (
              <span className="truncate text-sm" translate="no">
                {entry.name}
              </span>
            )}
          </div>
          {ops.length > 0 && (
            <div className="flex flex-wrap gap-2">
              {ops.map(([op, n]) => (
                <Chip key={op}>
                  {n} {opLabel(op)}
                </Chip>
              ))}
            </div>
          )}
        </div>
      );
    }
    default:
      return null;
  }
}

const CARD =
  "w-full rounded-3xl border border-border bg-card p-4 text-left shadow-sm transition-shadow hover:shadow-md focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none";

function MilestoneCard({
  entry,
  onOpenLanguage,
}: {
  entry: FeedEntry;
  onOpenLanguage?: (l: string) => void;
}) {
  const { locale } = useNeokapi();
  const percent = entry.counts.percent ?? 0;
  const l = entry.locales?.[0];
  return (
    <button
      type="button"
      data-slot="feed-card"
      data-kind={entry.kind}
      className={cn(CARD, "overflow-hidden p-0")}
      onClick={() => l && onOpenLanguage?.(l)}
    >
      <div className="h-1.5 bg-linear-to-r from-prism-1 via-prism-3 to-prism-5" />
      <div className="flex flex-col items-center gap-1 px-6 py-6">
        <span className="text-[11px] font-semibold tracking-wide text-muted-foreground uppercase">
          {t("Coverage milestone")}
        </span>
        <span className="text-6xl font-bold tabular-nums">{percent}%</span>
        <span className="text-base font-semibold">{l ? languageName(l, locale) : ""}</span>
        <span className="mt-3 block h-2 w-full overflow-hidden rounded-full bg-muted">
          <span
            className="block h-full rounded-full bg-linear-to-r from-prism-1 via-prism-3 to-prism-5"
            style={{ width: `${Math.min(100, percent)}%` }}
          />
        </span>
        <span className="mt-2 text-[13px] text-muted-foreground">{ageText(entry.at, locale)}</span>
      </div>
    </button>
  );
}

export interface FeedCardProps {
  card: Card;
  onOpen?: (entry: FeedEntry) => void;
  onOpenLanguage?: (locale: string) => void;
}

export function FeedCard({ card, onOpen, onOpenLanguage }: FeedCardProps) {
  const { locale } = useNeokapi();
  const [expanded, setExpanded] = useState(false);
  const lead = card.entries[0];
  if (lead.kind === "coverage_milestone") {
    return <MilestoneCard entry={lead} onOpenLanguage={onOpenLanguage} />;
  }
  const name = actorName(lead);
  const folded = card.entries.length > 1;
  const forMember =
    lead.actor.kind === "agent" && lead.on_behalf_of?.name
      ? t("for {name}", { name: lead.on_behalf_of.name })
      : undefined;

  return (
    <article data-slot="feed-card" data-kind={lead.kind} className={CARD}>
      <div
        role="button"
        tabIndex={0}
        className="cursor-pointer"
        onClick={() => onOpen?.(lead)}
        onKeyDown={(ev) => {
          if (ev.key === "Enter" || ev.key === " ") {
            ev.preventDefault();
            onOpen?.(lead);
          }
        }}
      >
        <div className="flex gap-3">
          <FeedAvatar actor={lead.actor} name={name} />
          <div className="min-w-0 flex-1">
            <p className="text-[15px] leading-5 text-foreground">
              <span className="font-semibold" translate="no">
                {name}
              </span>{" "}
              {foldedActionText(card, locale)}
            </p>
            <p className="mt-0.5 text-[13px] text-muted-foreground">
              {forMember && (
                <>
                  <span translate="no">{forMember}</span> ·{" "}
                </>
              )}
              <time
                dateTime={lead.at}
                title={new Date(lead.at).toLocaleString(locale || undefined)}
              >
                {ageText(lead.at, locale)}
              </time>
            </p>
          </div>
        </div>
        <div className="mt-3 pl-[52px]">
          <FeedPreview entry={lead} onOpenLanguage={onOpenLanguage} />
        </div>
      </div>
      {folded && (
        <div className="mt-3 pl-[52px]">
          {expanded && (
            <ul className="mb-3 space-y-1.5 border-t border-border pt-3 text-sm">
              {card.entries.slice(1).map((e) => (
                <li key={e.id} className="flex items-baseline gap-2">
                  <span className="size-1.5 shrink-0 translate-y-[-2px] rounded-full bg-muted-foreground" />
                  <span className="flex-1">{actionText(e, locale)}</span>
                  <span className="text-[13px] text-muted-foreground">{ageText(e.at, locale)}</span>
                </li>
              ))}
            </ul>
          )}
          <button
            type="button"
            data-slot="feed-card-expand"
            className="inline-flex items-center gap-1 text-sm font-medium text-primary hover:underline"
            onClick={() => setExpanded((v) => !v)}
          >
            {expanded
              ? t("Show less")
              : t("{count, plural, other {Show # more}}", { count: card.entries.length - 1 })}
            {expanded ? <ChevronUp className="size-3.5" /> : <ChevronDown className="size-3.5" />}
          </button>
        </div>
      )}
    </article>
  );
}

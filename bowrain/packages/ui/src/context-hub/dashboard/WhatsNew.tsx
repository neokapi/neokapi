// What's new on the workspace's context dashboard, in the shape Kapi Desktop's
// home draws it: one row per thing that waits or moved, each with the mark of
// who acted, its name and when, a chip per kind of thing it holds, and one
// button to the place it is decided.
//
// A change-set's row carries the blast radius the server stored when it was
// submitted, never a live walk. A voice profile's row counts the rules the
// team's corrections suggest, by kind, and opens the profile's review. j and k
// (or the arrows) move over the rows and Enter opens the one under the cursor.
import { useEffect, useMemo, useState } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  ActorMark,
  Badge,
  Button,
  KindIcon,
  SectionHeading,
  SimpleTooltip,
  Skeleton,
  When,
  cn,
  type DigestKind,
} from "@neokapi/ui-primitives";
import { Palette } from "../../components/icons";
import type { ImpactSummary } from "../../types/brand-graph";
import { ChangeSetStatusBadge } from "../shell/atoms";
import type { ChangeSetNews, NewsRow, ProfileNews } from "./newsRows";

export interface WhatsNewProps {
  rows: readonly NewsRow[];
  loading?: boolean;
  /** Open a change-set on its own page, where it is reviewed and merged. */
  onOpenChangeset?: (changesetId: string) => void;
  /** Open a voice profile's suggested-rule review. */
  onOpenReview?: (profileId: string) => void;
  /** Resolves a member's user id to the name the workspace knows them by. */
  nameOf?: (id: string | null | undefined) => string;
  /** Take the keyboard. */
  keyboard?: boolean;
}

export function WhatsNew({
  rows,
  loading,
  onOpenChangeset,
  onOpenReview,
  nameOf,
  keyboard = true,
}: WhatsNewProps) {
  const [cursor, setCursor] = useState(0);
  const active = rows[Math.min(cursor, rows.length - 1)];
  const open = useMemo(
    () => (row: NewsRow) => {
      if (row.type === "changeset") onOpenChangeset?.(row.changeset.id);
      else onOpenReview?.(row.profileId);
    },
    [onOpenChangeset, onOpenReview],
  );

  useEffect(() => {
    if (!keyboard || rows.length === 0) return;
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      const tag = el?.tagName ?? "";
      if (tag === "TEXTAREA" || tag === "INPUT" || tag === "SELECT" || el?.isContentEditable) {
        return;
      }
      if (el?.closest('[role="dialog"], [role="menu"], [role="listbox"]')) return;
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
          open(active);
          break;
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [active, keyboard, open, rows.length]);

  return (
    <section data-slot="whats-new" aria-labelledby="dashboard-whats-new">
      <div id="dashboard-whats-new">
        <SectionHeading>What&apos;s new</SectionHeading>
      </div>
      {loading ? (
        <div className="mt-1.5 space-y-1">
          <Skeleton className="h-11 w-full" />
          <Skeleton className="h-11 w-full" />
        </div>
      ) : rows.length > 0 ? (
        <div className="mt-1.5 divide-y divide-border/50">
          {rows.map((row, i) => (
            <NewsRowView
              key={row.key}
              row={row}
              active={active?.key === row.key}
              nameOf={nameOf}
              onFocus={() => setCursor(i)}
              onOpen={() => open(row)}
              canOpen={row.type === "changeset" ? !!onOpenChangeset : !!onOpenReview}
            />
          ))}
        </div>
      ) : (
        <p data-slot="whats-new-empty" className="mt-2 text-sm text-muted-foreground">
          Nothing waits for a decision. A proposal sent for review, or a phrasing your team keeps
          correcting, appears here.
        </p>
      )}
    </section>
  );
}

function NewsRowView({
  row,
  active,
  nameOf,
  onFocus,
  onOpen,
  canOpen,
}: {
  row: NewsRow;
  active: boolean;
  nameOf?: (id: string | null | undefined) => string;
  onFocus: () => void;
  onOpen: () => void;
  canOpen: boolean;
}) {
  const changeset = row.type === "changeset" ? row.changeset : undefined;
  return (
    <div
      data-slot="whats-new-row"
      data-kind={row.type}
      data-waiting={(row.type === "changeset" ? row.waiting : row.conflicts > 0) || undefined}
      data-active={active || undefined}
      onClick={onFocus}
      className={cn("flex h-11 items-center gap-3 rounded-md px-2", active && "bg-accent/50")}
    >
      {changeset ? (
        <ActorMark
          data-slot="whats-new-actor"
          actor="person"
          name={nameOf?.(changeset.created_by) || changeset.created_by}
        />
      ) : (
        <SimpleTooltip content={t("Voice profile")}>
          <span
            data-slot="whats-new-actor"
            role="img"
            aria-label={t("Voice profile")}
            className="inline-grid size-[22px] shrink-0 place-items-center rounded-[6px] border border-border bg-muted text-muted-foreground [&_svg]:size-3.5"
          >
            <Palette aria-hidden />
          </span>
        </SimpleTooltip>
      )}
      <div className="flex min-w-0 flex-1 items-baseline gap-1.5">
        <span className="truncate text-sm" translate="no">
          {changeset ? changeset.name : (row as ProfileNews).name}
        </span>
        {row.type === "changeset" && (
          <>
            <span aria-hidden className="text-[11px] text-muted-foreground">
              ·
            </span>
            <When
              iso={row.at}
              relative
              className="shrink-0 text-[11px] whitespace-nowrap text-muted-foreground"
            />
          </>
        )}
      </div>
      <div data-slot="whats-new-chips" className="hidden shrink-0 items-center gap-1.5 sm:flex">
        {row.type === "changeset" ? <ChangeSetChips row={row} /> : <ProfileChips row={row} />}
      </div>
      {canOpen && (
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
          {openLabel(row)}
        </Button>
      )}
    </div>
  );
}

/** The act the row's button leads to, named for it. */
function openLabel(row: NewsRow): React.ReactNode {
  if (row.type === "profile") return row.conflicts > 0 ? <>Decide</> : <>Review</>;
  switch (row.changeset.status) {
    case "in_review":
      return <>Review</>;
    case "approved":
      return <>Merge</>;
    default:
      return <>Open</>;
  }
}

const CHIP = "text-[11px] font-normal tabular-nums";

function Chip({
  kind,
  warning,
  hint,
  children,
}: {
  kind?: DigestKind;
  warning?: boolean;
  hint?: string;
  children: React.ReactNode;
}) {
  const chip = (
    <Badge
      variant={warning ? "warning" : "outline"}
      data-slot="whats-new-chip"
      data-kind={kind}
      tabIndex={hint ? 0 : undefined}
      className={cn(CHIP, !warning && "text-muted-foreground")}
    >
      {kind && <KindIcon kind={kind} decorative />}
      {children}
    </Badge>
  );
  return hint ? <SimpleTooltip content={hint}>{chip}</SimpleTooltip> : chip;
}

/** A profile's suggested rules by kind, then the disagreements that need a person. */
function ProfileChips({ row }: { row: ProfileNews }) {
  return (
    <>
      {row.words > 0 && (
        <Chip kind="word">
          {t("{count, plural, one {# word} other {# words}}", { count: row.words })}
        </Chip>
      )}
      {row.notes > 0 && (
        <Chip kind="note">
          {t("{count, plural, one {# note} other {# notes}}", { count: row.notes })}
        </Chip>
      )}
      {row.approved > 0 && (
        <Chip hint={t("Accepted by a reviewer, not yet promoted")}>
          {t("{count} approved", { count: row.approved })}
        </Chip>
      )}
      {row.conflicts > 0 && (
        <Chip kind="conflict" warning>
          {t("{count, plural, one {# needs you} other {# need you}}", { count: row.conflicts })}
        </Chip>
      )}
    </>
  );
}

/** A change-set's stored blast radius while it waits, and its status. */
function ChangeSetChips({ row }: { row: ChangeSetNews }) {
  return (
    <>
      {row.waiting && <ImpactChips impact={row.changeset.impact_summary} />}
      <ChangeSetStatusBadge status={row.changeset.status} className="text-[11px]" />
    </>
  );
}

/**
 * The totals the server stored when the change-set was submitted. A change-set
 * that was never submitted has none, and a walk that stopped early reports a
 * floor rather than a total.
 */
function ImpactChips({ impact }: { impact?: ImpactSummary }) {
  if (!impact) {
    return (
      <Chip hint={t("Blast radius is measured when a proposal is submitted.")}>
        {t("not measured")}
      </Chip>
    );
  }
  if (impact.affected_blocks === 0) {
    return <Chip>{t("no content affected")}</Chip>;
  }
  const floor = impact.partial ? "≥" : "";
  const blocks = `${floor}${impact.affected_blocks.toLocaleString()}`;
  const words = `${floor}${impact.words.toLocaleString()}`;
  return (
    <>
      <Chip
        hint={
          impact.partial
            ? t("{words} words. The walk stopped early: {reason}", {
                words,
                reason: impact.partial_reason ?? "",
              })
            : t("{words} words", { words })
        }
      >
        {t("{blocks} blocks", { blocks })}
      </Chip>
      {impact.new_violations > 0 && (
        <Chip warning>
          {t("+{count} new", "violations a change-set would add", {
            count: impact.new_violations.toLocaleString(),
          })}
        </Chip>
      )}
      {impact.resolved > 0 && (
        <Chip>
          {t("−{count} resolved", "violations a change-set would resolve", {
            count: impact.resolved.toLocaleString(),
          })}
        </Chip>
      )}
    </>
  );
}

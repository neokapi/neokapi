import * as React from "react";
import { t } from "@neokapi/i18n-react/runtime";

import { cn } from "../../lib/utils";

/**
 * StatusBadge: one scale for the two ladders content climbs.
 *
 * A target moves draft → translated → established; a source moves written →
 * established. Both are drawn on one scale: muted at the bottom rung, neutral
 * in the middle, and a filled green once a person has established the unit.
 * The source ladder skips the bottom stop, so `written` sits where
 * `translated` does.
 *
 * `blocked` and `attention` belong to neither ladder and both take warning:
 * something is waiting for a person. `not-started` is the bucket a locale with
 * no target text falls in, below the content ladder's bottom rung.
 *
 * The status strings are the wire values (`not-started`, not `not_started`), so a
 * caller can hand a badge whatever the API returned.
 */

/** Which ladder a status belongs to. */
export type StatusLadder = "content" | "source";

/** The target ladder, lowest rung first. Matches core/model TargetStatusLadder. */
export const CONTENT_STATUS_LADDER = ["draft", "translated", "established"] as const;

/** The source ladder, lowest rung first. Matches core/model SourceStatusLadder. */
export const SOURCE_STATUS_LADDER = ["written", "established"] as const;

/** Statuses that sit off both ladders and mean a person is needed. */
export const ATTENTION_STATUSES = ["blocked", "attention"] as const;

export type ContentStatus = (typeof CONTENT_STATUS_LADDER)[number];
export type SourceStatus = (typeof SOURCE_STATUS_LADDER)[number];
export type AttentionStatus = (typeof ATTENTION_STATUSES)[number];
export type LadderStatus = ContentStatus | SourceStatus | AttentionStatus;

/** The ladders by name, so an adopter can iterate one without re-typing it. */
export const STATUS_LADDERS: Record<StatusLadder, readonly string[]> = {
  content: CONTENT_STATUS_LADDER,
  source: SOURCE_STATUS_LADDER,
};

/**
 * The four stops of the shared scale, plus the off-ladder one.
 *
 * `earned` is the soft green, which no rung of either ladder takes.
 * `settled` is the filled green: a unit a person established.
 */
export type StatusTone = "start" | "middle" | "earned" | "settled" | "attention";

const TONE_CLASS: Record<StatusTone, string> = {
  start: "bg-muted text-muted-foreground",
  // A tint of the foreground rather than `secondary`, which sits so close to the
  // card in the Kapi palette that the middle rung read as bare text.
  middle: "bg-foreground/10 text-foreground",
  earned: "bg-success/12 text-success dark:bg-success/20",
  settled: "bg-success text-success-foreground",
  attention: "bg-warning text-warning-foreground",
};

/** What a badge needs to draw one status. */
export interface StatusMeta {
  /** The status as it travels on the wire. */
  status: string;
  /** Where it sits on the shared scale. */
  tone: StatusTone;
  /** Human-readable name in the active UI language. */
  readonly label: string;
}

// `label` is a getter throughout, so the dictionary is read at render time and
// a locale switch repaints a badge mounted before the catalog arrived.
const CONTENT_META: Record<string, StatusMeta> = {
  // The bucket the platform counts a locale in when it has no target text at
  // all. It sits below the ladder rather than on it, so it shares the bottom
  // tone and says so in words.
  "not-started": {
    status: "not-started",
    tone: "start",
    get label() {
      return t("Not started", "content status");
    },
  },
  draft: {
    status: "draft",
    tone: "start",
    get label() {
      return t("Draft", "content status");
    },
  },
  translated: {
    status: "translated",
    tone: "middle",
    get label() {
      return t("Translated", "content status");
    },
  },
  established: {
    status: "established",
    tone: "settled",
    get label() {
      return t("Established", "content status");
    },
  },
};

const SOURCE_META: Record<string, StatusMeta> = {
  written: {
    status: "written",
    tone: "middle",
    get label() {
      return t("Written", "source status");
    },
  },
  established: {
    status: "established",
    tone: "settled",
    get label() {
      return t("Established", "source status");
    },
  },
};

const ATTENTION_META: Record<string, StatusMeta> = {
  blocked: {
    status: "blocked",
    tone: "attention",
    get label() {
      return t("Blocked", "content status");
    },
  },
  attention: {
    status: "attention",
    tone: "attention",
    get label() {
      return t("Needs attention", "content status");
    },
  },
};

/**
 * Resolve a status on a ladder. An unrecognised status keeps its own text and
 * takes the neutral middle stop: a server that grows a rung should read as a
 * status nobody has styled yet, not as an empty cell.
 */
export function statusMeta(ladder: StatusLadder, status: string): StatusMeta {
  const table = ladder === "source" ? SOURCE_META : CONTENT_META;
  return (
    table[status] ??
    ATTENTION_META[status] ?? {
      status,
      tone: "middle" as StatusTone,
      label: status,
    }
  );
}

export interface StatusBadgeProps extends React.ComponentProps<"span"> {
  /** Which ladder to read `status` against. */
  ladder: StatusLadder;
  /** The status as it travels on the wire, e.g. `established`. */
  status: string;
  /** Denser badge for a table cell or a dense list. */
  compact?: boolean;
  /** Override the rendered name. */
  label?: string;
}

/** A status on the content or source ladder, drawn on the shared scale. */
export function StatusBadge({
  ladder,
  status,
  compact = false,
  label,
  className,
  ...props
}: StatusBadgeProps) {
  const meta = statusMeta(ladder, status);

  return (
    <span
      data-slot="status-badge"
      data-ladder={ladder}
      data-status={status}
      data-tone={meta.tone}
      className={cn(
        "inline-flex w-fit shrink-0 items-center justify-center rounded-4xl font-medium whitespace-nowrap",
        compact ? "h-4 px-1.5 text-[10px]" : "h-5 px-2 text-xs",
        TONE_CLASS[meta.tone],
        className,
      )}
      {...props}
    >
      {label ?? meta.label}
    </span>
  );
}

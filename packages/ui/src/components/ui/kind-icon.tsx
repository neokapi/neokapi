import * as React from "react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  CaseLower,
  CaseSensitive,
  Check,
  Languages,
  Pilcrow,
  TrendingDown,
  TriangleAlert,
} from "lucide-react";

import { cn } from "../../lib/utils";
import { SimpleTooltip } from "./tooltip";

/**
 * KindIcon: what sort of thing a row of a project's digest is, as one small
 * icon, so a list of suggestions scans by kind before it is read.
 *
 * The kinds are the digest's own (`host/contextdigest.go`). Suggestions sit in
 * four themes, one kind each: names and spellings, words to avoid, how the
 * project writes, and wording in other languages. Three more say where an item
 * stands rather than what it is about: a disagreement waiting for a person,
 * an established rule, and content drifting away from one. `digestKind` turns
 * a digest item's `theme` and `status` into one of these.
 *
 * Muted by default. A disagreement takes the warning ink because it waits for
 * a person; an established rule takes the success ink because a person settled
 * it. The kind's name is the icon's accessible name and its tooltip.
 */

/**
 * Every kind of thing a digest shows. A kind added here does not compile until
 * KIND_ICONS gives it an icon, a tone and a name.
 */
export const DIGEST_KINDS = [
  "name",
  "word",
  "note",
  "memory",
  "conflict",
  "established",
  "drift",
] as const;

export type DigestKind = (typeof DIGEST_KINDS)[number];

/** The digest's suggestion themes, as `DigestItem.theme` carries them. */
export const DIGEST_THEMES = ["names", "words", "writing", "memory"] as const;

export type DigestThemeId = (typeof DIGEST_THEMES)[number];

/** Which colour a kind's icon is drawn in. */
export type KindTone = "muted" | "warning" | "success";

/** What a KindIcon needs to draw one kind. */
export interface KindIconMeta {
  icon: React.ComponentType<{ className?: string; "aria-hidden"?: boolean }>;
  tone: KindTone;
  /** The kind's name in the active UI language. */
  readonly label: string;
}

/**
 * The icon, tone and name of every kind. `label` is a getter so a locale switch
 * repaints an icon mounted before the dictionary arrived.
 */
export const KIND_ICONS: Record<DigestKind, KindIconMeta> = {
  name: {
    icon: CaseSensitive,
    tone: "muted",
    get label() {
      return t("Name or spelling", "digest kind");
    },
  },
  word: {
    icon: CaseLower,
    tone: "muted",
    get label() {
      return t("Word to avoid", "digest kind");
    },
  },
  note: {
    icon: Pilcrow,
    tone: "muted",
    get label() {
      return t("How the project writes", "digest kind");
    },
  },
  memory: {
    icon: Languages,
    tone: "muted",
    get label() {
      return t("Wording in another language", "digest kind");
    },
  },
  conflict: {
    icon: TriangleAlert,
    tone: "warning",
    get label() {
      return t("Disagreement", "digest kind");
    },
  },
  established: {
    icon: Check,
    tone: "success",
    get label() {
      return t("Established rule", "digest kind");
    },
  },
  drift: {
    icon: TrendingDown,
    tone: "muted",
    get label() {
      return t("Drifting from a rule", "digest kind");
    },
  },
};

/** The kind each suggestion theme is drawn as. */
export const THEME_KINDS: Record<DigestThemeId, DigestKind> = {
  names: "name",
  words: "word",
  writing: "note",
  memory: "memory",
};

// `--warning` is a fill colour that holds dark ink, and as ink on a light
// ground it reads at about 2.3:1. Mixed with its own ink it clears 4.9:1, and
// on a dark ground the fill colour is legible as it is.
const TONE_CLASS: Record<KindTone, string> = {
  muted: "text-muted-foreground",
  warning:
    "text-[color-mix(in_oklch,var(--warning)_60%,var(--warning-foreground))] dark:text-warning",
  success: "text-success",
};

/** The name of a kind, in the active UI language. */
export function kindLabel(kind: DigestKind): string {
  return KIND_ICONS[kind].label;
}

/** What `digestKind` reads off a digest item or a feed entry. */
export interface DigestKindSource {
  /** `names`, `words`, `writing` or `memory`. */
  theme?: string;
  /** `suggested`, `established`, `contested`, … */
  status?: string;
  /** The subject, for an entry that carries no theme (a feed entry). */
  subject?: { kind?: string };
}

/**
 * The kind a digest item or a feed entry is drawn as. A contested item is a
 * disagreement and an established one is a rule, whatever it is about. The
 * rest take their theme's kind, or their subject's when they carry no theme.
 * Anything else is a word rule, which is where the digest itself puts a
 * subject it cannot place.
 */
export function digestKind(item: DigestKindSource): DigestKind {
  if (item.status === "contested") return "conflict";
  if (item.status === "established") return "established";
  if (item.theme && (DIGEST_THEMES as readonly string[]).includes(item.theme)) {
    return THEME_KINDS[item.theme as DigestThemeId];
  }
  if (item.subject?.kind === "memory") return "memory";
  if (item.subject?.kind === "note") return "note";
  return "word";
}

export interface KindIconProps extends Omit<React.ComponentProps<"span">, "children"> {
  kind: DigestKind;
  /** `sm` (14px, default) for a list row, `md` (16px) beside body text. */
  size?: "sm" | "md";
  /**
   * The kind is spelt out beside the icon already: hide the icon from
   * assistive technology and drop the tooltip.
   */
  decorative?: boolean;
}

/** One kind of digest item, as a small icon in its tone. */
export function KindIcon({
  kind,
  size = "sm",
  decorative = false,
  className,
  ...props
}: KindIconProps) {
  const meta = KIND_ICONS[kind];
  const Icon = meta.icon;
  const label = meta.label;

  const icon = (
    <span
      data-slot="kind-icon"
      data-kind={kind}
      data-tone={meta.tone}
      role={decorative ? undefined : "img"}
      aria-label={decorative ? undefined : label}
      aria-hidden={decorative || undefined}
      className={cn(
        "inline-flex shrink-0 items-center justify-center",
        TONE_CLASS[meta.tone],
        className,
      )}
      {...props}
    >
      <Icon aria-hidden className={size === "sm" ? "size-3.5" : "size-4"} />
    </span>
  );

  return decorative ? icon : <SimpleTooltip content={label}>{icon}</SimpleTooltip>;
}

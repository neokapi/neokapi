import * as React from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { RefreshCw, User } from "lucide-react";

import { cn } from "../../lib/utils";
import { SimpleTooltip } from "./tooltip";

/**
 * ActorMark: who did something, as a small square a reader recognises before
 * reading a name.
 *
 * Each assistant host kapi connects to has two letters and a hue, so "CC" in
 * violet is Claude Code on every row of every screen. A person and a sync are
 * neutral: a person shows their initials, or a person icon when the record
 * carries no name, and a sync shows the sync icon. Any other actor shows
 * initials taken from its own name on the neutral tint, so an assistant this
 * build has not heard of still gets a mark.
 *
 * The hues come from the prism ramp `CoordinateChip` draws its axes from, at
 * the same tint and ink lightness, so a mark beside a chip reads as one
 * drawing. The full name is the accessible name and the tooltip, because the
 * letters and the hue are a shorthand.
 */

/**
 * Every actor the mark knows by name: the assistant hosts in
 * `host/agentwiring.go`, Claude Desktop (wired by Kapi Desktop itself), a
 * person and a sync.
 */
export const KNOWN_ACTORS = [
  "claude-code",
  "claude-desktop",
  "codex",
  "cursor",
  "vscode",
  "person",
  "sync",
] as const;

export type KnownActor = (typeof KNOWN_ACTORS)[number];

/** What the mark needs to draw one known actor. */
interface ActorMeta {
  /** Two letters, for an actor drawn with initials. */
  initials?: string;
  /** The icon, for an actor drawn without initials. */
  icon?: React.ComponentType<{ className?: string; "aria-hidden"?: boolean }>;
  /** A prism-ramp hue in degrees. Absent means the neutral tint. */
  hue?: number;
  /** The display name in the active UI language. */
  readonly label: string;
}

// Product names are never translated. "You" and "Sync" are UI copy, so their
// labels are getters, read at render time like StatusBadge's.
const ACTORS: Record<KnownActor, ActorMeta> = {
  "claude-code": { initials: "CC", hue: 300, label: "Claude Code" },
  "claude-desktop": { initials: "CD", hue: 15, label: "Claude Desktop" },
  codex: { initials: "CX", hue: 250, label: "Codex" },
  cursor: { initials: "CU", hue: 150, label: "Cursor" },
  vscode: { initials: "VS", hue: 195, label: "VS Code" },
  person: {
    icon: User,
    get label() {
      return t("You", "actor: the person reading");
    },
  },
  sync: {
    icon: RefreshCw,
    get label() {
      return t("Sync", "actor: a sync with a shared copy of the project");
    },
  },
};

function knownActor(actor: string): KnownActor | undefined {
  const key = actor.trim().toLowerCase();
  return (KNOWN_ACTORS as readonly string[]).includes(key) ? (key as KnownActor) : undefined;
}

/**
 * The display name for an actor: "Claude Code", "Codex", "You" for a person,
 * "Sync". An actor outside KNOWN_ACTORS is named by its own string.
 */
export function actorLabel(actor: string): string {
  const known = knownActor(actor);
  return known ? ACTORS[known].label : actor;
}

/**
 * Two letters from a name: the first letters of the first and last words, or
 * the first two letters of a single word. Words split on spaces and on the
 * punctuation that joins an identifier (`lab-agent`, `claude@studio`).
 */
function initialsFrom(name: string): string {
  const words = name
    .trim()
    .split(/[\s\-_.@/:]+/u)
    .filter(Boolean);
  if (words.length === 0) return "";
  // Array.from splits by code point, so a letter outside the BMP stays whole.
  const first = Array.from(words[0]);
  const letters =
    words.length > 1 ? [first[0], Array.from(words[words.length - 1])[0]] : first.slice(0, 2);
  return letters.join("").toLocaleUpperCase();
}

/**
 * The letters a mark shows, or null when it shows an icon instead: a known
 * assistant's own two letters, a person's initials from their name (null with
 * no name), null for a sync, and initials from the string for anything else.
 */
export function actorInitials(actor: string, name?: string): string | null {
  const known = knownActor(actor);
  if (known === "person") {
    const fromName = name ? initialsFrom(name) : "";
    return fromName || null;
  }
  if (known) return ACTORS[known].initials ?? null;
  return initialsFrom(name || actor) || null;
}

/** The size steps: `sm` is 18px, `md` 22px, `lg` 28px. */
export type ActorMarkSize = "sm" | "md" | "lg";

const SIZE_CLASS: Record<ActorMarkSize, { box: string; icon: string }> = {
  sm: { box: "size-[18px] rounded-[5px] text-[11px] tracking-tighter", icon: "size-3" },
  md: { box: "size-[22px] rounded-[6px] text-[11px] tracking-tight", icon: "size-3.5" },
  lg: { box: "size-7 rounded-[7px] text-xs", icon: "size-4" },
};

// The hued tint, ink and border at the lightness and chroma of the coordinate
// axis pairs in semantic-colors.css, with the hue left to `--mark-hue`. One
// class string for every hue keeps the pairs from drifting apart, and the dark
// values keep the ink above 8:1 on its tint in both themes.
const HUED_CLASS =
  "border-[oklch(0.89_0.055_var(--mark-hue))] bg-[oklch(0.945_0.035_var(--mark-hue))] text-[oklch(0.44_0.13_var(--mark-hue))] dark:border-[oklch(0.38_0.06_var(--mark-hue))] dark:bg-[oklch(0.3_0.045_var(--mark-hue))] dark:text-[oklch(0.84_0.09_var(--mark-hue))]";

// The neutral is CoordinateChip's: the tint an axis nobody named falls back to.
const NEUTRAL_CLASS = "border-border bg-axis-unknown text-axis-unknown-foreground";

export interface ActorMarkProps extends Omit<React.ComponentProps<"span">, "children"> {
  /**
   * Who: a host id from KNOWN_ACTORS (`claude-code`, `codex`, `person`,
   * `sync`, …) or any other actor name, which renders neutral.
   */
  actor: string;
  /**
   * A person's name, which gives the initials and the accessible name. An
   * actor outside KNOWN_ACTORS takes it as its display name too. A known
   * assistant keeps its product name, so a caller can pass the name a record
   * carries for every actor alike.
   */
  name?: string;
  /** `md` (22px, default) for a row, `sm` (18px) beside dense text, `lg` (28px) for a header. */
  size?: ActorMarkSize;
  /**
   * The name is printed beside the mark already: hide the mark from assistive
   * technology and drop the tooltip, so the name is read once.
   */
  decorative?: boolean;
}

/** A small square naming who did something, by initials and hue. */
export function ActorMark({
  actor,
  name,
  size = "md",
  decorative = false,
  className,
  style,
  ...props
}: ActorMarkProps) {
  const known = knownActor(actor);
  const meta = known ? ACTORS[known] : undefined;
  const assistant = meta !== undefined && known !== "person" && known !== "sync";
  const label = (!assistant && name?.trim()) || actorLabel(actor);
  const initials = actorInitials(actor, name);
  const Icon = initials ? undefined : (meta?.icon ?? User);
  const hue = meta?.hue;
  const sizes = SIZE_CLASS[size];

  const mark = (
    <span
      data-slot="actor-mark"
      data-actor={known ?? actor}
      role={decorative ? undefined : "img"}
      aria-label={decorative ? undefined : label}
      aria-hidden={decorative || undefined}
      className={cn(
        "inline-grid shrink-0 place-items-center overflow-hidden border leading-none font-semibold select-none",
        sizes.box,
        hue === undefined ? NEUTRAL_CLASS : HUED_CLASS,
        className,
      )}
      style={hue === undefined ? style : ({ "--mark-hue": hue, ...style } as React.CSSProperties)}
      {...props}
    >
      {Icon ? <Icon aria-hidden className={sizes.icon} /> : initials}
    </span>
  );

  return decorative ? mark : <SimpleTooltip content={label}>{mark}</SimpleTooltip>;
}

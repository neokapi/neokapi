// Day arithmetic for the atlas. A query names whole UTC days ("2026-09-18"):
// a window runs from the start of its first day to the end of its last, and a
// snapshot is the end of its day. Every instant on the wire is compared in
// milliseconds since the epoch.

export const DAY_MS = 86_400_000;

const DAY_RE = /^\d{4}-\d{2}-\d{2}$/;

/** Milliseconds for an RFC 3339 instant; NaN when it does not parse. */
export function instantMs(iso: string | undefined | null): number {
  if (!iso) return Number.NaN;
  return Date.parse(iso);
}

/**
 * Whether an instant is the Go zero time or missing: the contract's way of
 * saying "unknown".
 */
export function isUnknownInstant(iso: string | undefined | null): boolean {
  if (!iso) return true;
  const ms = Date.parse(iso);
  return Number.isNaN(ms) || ms <= Date.parse("0001-01-02T00:00:00Z");
}

/** Whether a string is a well-formed UTC day, `YYYY-MM-DD`. */
export function isDay(s: unknown): s is string {
  if (typeof s !== "string" || !DAY_RE.test(s)) return false;
  return !Number.isNaN(Date.parse(`${s}T00:00:00Z`));
}

/** The UTC day an instant falls on. */
export function dayOf(ms: number): string {
  return new Date(Math.floor(ms / DAY_MS) * DAY_MS).toISOString().slice(0, 10);
}

/** Milliseconds at the start (00:00 UTC) of a day. */
export function dayStart(day: string): number {
  return Date.parse(`${day}T00:00:00Z`);
}

/** Milliseconds at the end of a day: the last instant that belongs to it. */
export function dayEnd(day: string): number {
  return dayStart(day) + DAY_MS - 1;
}

/** The day `n` days after `day` (before, when negative). */
export function addDays(day: string, n: number): string {
  return dayOf(dayStart(day) + n * DAY_MS);
}

/** Whole days from `a` to `b`. */
export function daysBetween(a: string, b: string): number {
  return Math.round((dayStart(b) - dayStart(a)) / DAY_MS);
}

/** The later of two days. */
export function maxDay(a: string, b: string): string {
  return a >= b ? a : b;
}

/** The earlier of two days. */
export function minDay(a: string, b: string): string {
  return a <= b ? a : b;
}

/** Clamp a day into `[lo, hi]`. */
export function clampDay(day: string, lo: string, hi: string): string {
  return minDay(maxDay(day, lo), hi);
}

/** Every day from `from` to `to`, inclusive. */
export function dayRange(from: string, to: string): string[] {
  const out: string[] = [];
  for (let d = from; d <= to; d = addDays(d, 1)) out.push(d);
  return out;
}

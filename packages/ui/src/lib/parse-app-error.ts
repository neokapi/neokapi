// parseAppError — normalise the many shapes an application error can arrive in
// (Wails binding envelopes, nested JSON strings, Error instances, plain
// strings) into one friendly-first contract:
//
//   { title, detail?, raw? }
//
// - `title`  — the friendly one-liner to show the user.
// - `detail` — an optional secondary hint (e.g. an underlying cause message).
// - `raw`    — the original structured payload, pretty-printed, for a
//              details-on-demand disclosure. Only present when the input was
//              structured (JSON envelope / object / Error with a stack).
//
// The same contract exists in bowrain's kit; keep names, states, and behavior
// in sync (no cross-kit imports).

export interface ParsedAppError {
  /** Friendly one-line summary, safe to show as the primary message. */
  title: string;
  /** Optional secondary hint (e.g. the underlying cause). */
  detail?: string;
  /** Pretty-printed structured payload for a "Details" disclosure. */
  raw?: string;
}

/** Fallback title when an error carries no usable message at all. */
const UNKNOWN = "Something went wrong";

function looksLikeJson(s: string): boolean {
  const t = s.trim();
  return (t.startsWith("{") && t.endsWith("}")) || (t.startsWith("[") && t.endsWith("]"));
}

function tryParseJson(s: string): unknown | undefined {
  if (!looksLikeJson(s)) return undefined;
  try {
    return JSON.parse(s) as unknown;
  } catch {
    return undefined;
  }
}

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

function pretty(v: unknown): string {
  try {
    return JSON.stringify(v, null, 2);
  } catch {
    return String(v);
  }
}

/** Extract a human message from an envelope-ish object, if it has one. */
function envelopeMessage(obj: Record<string, unknown>): string | undefined {
  for (const key of ["message", "error", "detail", "reason"]) {
    const v = obj[key];
    if (typeof v === "string" && v.trim() !== "") return v.trim();
  }
  return undefined;
}

/** Extract a secondary hint from an envelope's cause, if meaningful. */
function causeDetail(obj: Record<string, unknown>): string | undefined {
  const cause = obj["cause"];
  if (typeof cause === "string" && cause.trim() !== "") return cause.trim();
  if (isRecord(cause)) {
    const msg = envelopeMessage(cause);
    if (msg) return msg;
  }
  return undefined;
}

/**
 * Normalise an unknown error value into a friendly `{ title, detail?, raw? }`.
 *
 * Handles, in order:
 * - Wails binding envelopes `{ message, kind, cause }` (message = friendly line)
 * - JSON strings containing such envelopes, with one nested-parse attempt
 *   when the envelope's message is itself a JSON envelope string
 * - `Error` instances (message = title; stack retained as `raw`)
 * - plain strings and everything else via `String(...)`
 */
export function parseAppError(err: unknown): ParsedAppError {
  if (err === undefined || err === null) return { title: UNKNOWN };

  if (err instanceof Error) {
    const fromMessage = tryParseJson(err.message);
    if (isRecord(fromMessage)) {
      return fromEnvelope(fromMessage);
    }
    const title = err.message.trim() !== "" ? err.message : err.name || UNKNOWN;
    const raw = err.stack && err.stack.trim() !== "" ? err.stack : undefined;
    return raw ? { title, raw } : { title };
  }

  if (typeof err === "string") {
    const trimmed = err.trim();
    if (trimmed === "") return { title: UNKNOWN };
    const parsed = tryParseJson(trimmed);
    if (isRecord(parsed)) return fromEnvelope(parsed);
    if (parsed !== undefined) {
      // Structured but not an envelope (e.g. an array) — keep it inspectable.
      return { title: UNKNOWN, raw: pretty(parsed) };
    }
    return { title: trimmed };
  }

  if (isRecord(err)) return fromEnvelope(err);

  return { title: String(err) };
}

function fromEnvelope(obj: Record<string, unknown>): ParsedAppError {
  const raw = pretty(obj);
  let title = envelopeMessage(obj);
  let detail = causeDetail(obj);

  // One nested-parse attempt: the friendly line may itself be a JSON envelope
  // string (double-encoded errors are common across binding layers).
  if (title !== undefined) {
    const nested = tryParseJson(title);
    if (isRecord(nested)) {
      const inner = envelopeMessage(nested);
      if (inner) {
        title = inner;
        detail = detail ?? causeDetail(nested);
      }
    }
  }

  if (title === undefined) {
    return { title: UNKNOWN, raw };
  }
  return detail ? { title, detail, raw } : { title, raw };
}

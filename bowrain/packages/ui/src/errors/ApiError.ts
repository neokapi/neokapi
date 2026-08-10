/**
 * ApiError — the typed error thrown by the REST adapter on any non-OK
 * response. It parses the server's standard error envelope
 * `{ error, message?, reference?, ...details }` (bowrain/apierror) into
 * structured fields while tolerating every legacy shape: plain-text bodies,
 * envelopes without `message`/`reference`, and gateway HTML.
 *
 * `Error.message` prefers the server's human sentence so even call sites that
 * render `(e as Error).message` raw show a readable line instead of JSON;
 * parseAppError recognises ApiError first-class and reconstructs the full
 * friendly presentation (copy map, hint, reference chip) from the structured
 * fields.
 */

import type { GovernedRefusal } from "../types/api";

/** Envelope keys that are part of the error contract, not detail payload. */
const ENVELOPE_KEYS = new Set(["error", "message", "reference", "reference_id"]);

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export class ApiError extends Error {
  /** Stable machine code — the envelope's `error` field (e.g. `project_limit_reached`). */
  readonly code?: string;
  /**
   * Per-request correlation ID: the envelope's `reference` (or legacy
   * `reference_id`) field, falling back to the `X-Request-ID` response header.
   * The one ID a user can quote to drill down to server logs and Sentry.
   */
  readonly referenceId?: string;
  /** HTTP status of the response. */
  readonly status: number;
  /** Structured top-level detail fields from the envelope (e.g. `current`, `limit`). */
  readonly details?: Record<string, unknown>;
  /** The parsed JSON body, when the response carried one. */
  readonly body?: unknown;
  /** The raw response body text (legacy `"<status>: <body>"` renders from this). */
  readonly bodyText: string;

  constructor(args: {
    message: string;
    status: number;
    code?: string;
    referenceId?: string;
    details?: Record<string, unknown>;
    body?: unknown;
    bodyText: string;
  }) {
    super(args.message);
    this.name = "ApiError";
    this.status = args.status;
    this.code = args.code;
    this.referenceId = args.referenceId;
    this.details = args.details;
    this.body = args.body;
    this.bodyText = args.bodyText;
  }

  /**
   * Historical alias: earlier adapter errors attached the correlation ID as
   * `reference`; keep it readable for call sites that duck-type that field.
   */
  get reference(): string | undefined {
    return this.referenceId;
  }
}

/**
 * Build an ApiError from a non-OK response. `headerReference` is the
 * `X-Request-ID` response header, which carries the correlation ID even when
 * the body is not our JSON envelope (gateway 502s, empty bodies).
 */
export function apiErrorFromResponse(
  status: number,
  bodyText: string,
  headerReference?: string | null,
): ApiError {
  let body: unknown;
  const trimmed = bodyText.trim();
  if (trimmed.startsWith("{") || trimmed.startsWith("[")) {
    try {
      body = JSON.parse(trimmed);
    } catch {
      body = undefined;
    }
  }

  let code: string | undefined;
  let serverMessage: string | undefined;
  let referenceId = headerReference || undefined;
  let details: Record<string, unknown> | undefined;

  if (isRecord(body) && typeof body.error === "string") {
    code = body.error;
    if (typeof body.message === "string" && body.message.trim() !== "") {
      serverMessage = body.message;
    }
    const bodyRef =
      typeof body.reference === "string" && body.reference !== ""
        ? body.reference
        : typeof body.reference_id === "string" && body.reference_id !== ""
          ? body.reference_id
          : undefined;
    if (bodyRef) referenceId = bodyRef;
    const extra = Object.entries(body).filter(([k]) => !ENVELOPE_KEYS.has(k));
    if (extra.length > 0) details = Object.fromEntries(extra);
  }

  // Prefer the server's human sentence, then the stable code, then the
  // historical "<status>: <body>" string — so `.message` is always readable.
  const message = serverMessage ?? code ?? `${status}: ${bodyText}`;

  return new ApiError({ message, status, code, referenceId, details, body, bodyText });
}

/**
 * The 409 envelope the server refuses a governed edit with, verbatim from
 * `conceptGovernedConflict` (bowrain/server/handlers_concepts.go). `detail`
 * names the specific edit and is supplied per call site.
 *
 * It is declared here because an adapter with no server to ask still has to
 * refuse — the desktop's concept bulk-delete is governed whether or not a
 * request is made — and a bare `Error` there gave the desktop a vaguer refusal
 * than the web for the same action. A contract test holds these strings
 * against the Go source.
 */
export const GOVERNED_REFUSAL_ERROR = "governed change requires a change-set";
export const GOVERNED_REFUSAL_HINT =
  "open a change-set (POST /:ws/changesets), add the governed op, and submit it for review";

/**
 * The refusal an adapter raises locally, shaped so `governedRefusal` reads it
 * exactly as it reads the server's. `detail` is the phrase naming the edit
 * ("deleting concepts"), matching the argument the Go handler passes.
 */
export function governedRefusalError(detail: string): ApiError {
  const body = { error: GOVERNED_REFUSAL_ERROR, detail, hint: GOVERNED_REFUSAL_HINT };
  return apiErrorFromResponse(409, JSON.stringify(body));
}

/**
 * The governed-refusal envelope when `err` is the 409 a governed batch is
 * refused with — deleting concepts, which belongs in a change-set. Undefined
 * for every other error, so a caller can fall through to ordinary handling.
 */
export function governedRefusal(err: unknown): GovernedRefusal | undefined {
  if (!(err instanceof ApiError) || err.status !== 409) return undefined;
  const detail = err.details?.detail;
  const hint = err.details?.hint;
  if (typeof detail !== "string" || typeof hint !== "string") return undefined;
  return { error: err.code ?? err.message, detail, hint };
}

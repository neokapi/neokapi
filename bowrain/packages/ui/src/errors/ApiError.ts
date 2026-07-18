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

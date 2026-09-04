/**
 * parseAppError — one place that turns any thrown/returned error value into a
 * human-readable summary, while preserving the raw structured payload for an
 * optional "Details" disclosure.
 *
 * Known shapes handled:
 * - `ApiError` thrown by the REST adapter (structured code/status/reference/details)
 * - REST envelope from bowrain-server:
 *   `{ error: string, message?: string, reference?: string, details?: string, code?: string, ...detail fields }`
 * - Client-side envelope: `{ message: string, kind?: string, cause?: unknown }`
 * - `Error` instances, including the historical RestApiAdapter pattern
 *   `Error("<httpStatus>: <raw body>")` where the body may itself be JSON
 * - Plain strings (one JSON parse attempt for nested envelopes)
 */

import { ApiError } from "./ApiError";

export interface AppError {
  /** Short human-readable one-liner. */
  title: string;
  /** Secondary human detail (server `details` field, cause, original message). */
  detail?: string;
  /** Recovery hint mapped from known codes / HTTP statuses. */
  hint?: string;
  /** The structured raw payload, when the error carried one. */
  raw?: unknown;
  /** Machine-readable code (`code` field), when present. */
  code?: string;
  /** HTTP status, when it could be derived. */
  status?: number;
  /**
   * Per-request correlation ID (server `reference` field / `X-Request-ID`
   * header). The one ID a user can quote so support can drill down to the
   * exact server logs and Sentry issue. Shown by ErrorNotice.
   */
  reference?: string;
}

const GENERIC_TITLE = "Something went wrong";
const MAX_TITLE_LENGTH = 160;

interface Phrase {
  title: string;
  hint?: string;
}

/** Friendly phrasing for machine `code` values used by API envelopes. */
const CODE_PHRASES: Record<string, Phrase> = {
  unauthorized: {
    title: "You need to sign in",
    hint: "Your session may have expired. Sign in and try again.",
  },
  forbidden: {
    title: "You don't have permission to do that",
    hint: "Ask a workspace admin to grant you access.",
  },
  not_found: {
    title: "That item could not be found",
    hint: "It may have been moved or deleted. Try refreshing the page.",
  },
  conflict: {
    title: "That change conflicts with the current state",
    hint: "Refresh to pick up the latest changes, then try again.",
  },
  rate_limited: {
    title: "Too many requests",
    hint: "Wait a moment before trying again.",
  },
  invalid: {
    title: "The request was not valid",
    hint: "Check the values you entered and try again.",
  },
};

/**
 * Friendly copy for the stable envelope codes the server puts in the `error`
 * field. This is the central code → copy map: each entry can read the
 * envelope's structured detail fields (current, limit, minimum_plan, …) to
 * parametrize the sentence. Codes without an entry fall back to the server's
 * own `message` sentence.
 */
const ENVELOPE_CODE_PHRASES: Record<string, (obj: Record<string, unknown>) => Phrase> = {
  project_limit_reached: (obj) => ({
    title: limitTitle("project", obj),
    // A free workspace's project ceiling guards against scripted signups
    // rather than metering anything, so the hint does not sell an upgrade —
    // no paid plan lists a project count.
    hint: "Create a new workspace, choose another, or get in touch.",
  }),
  custodian_limit_reached: (obj) => ({
    title: limitTitle("custodian", obj),
    hint: "A custodian governs a brand, product or channel. Every other member is free: add them without a custodian's region, or upgrade for another custodian.",
  }),
  upgrade_required: (obj) => ({
    title: "This feature is not included in the workspace's current plan",
    hint:
      typeof obj.minimum_plan === "string" && obj.minimum_plan !== ""
        ? `Upgrade to the ${obj.minimum_plan} plan to enable it.`
        : "Upgrade the workspace's plan to enable it.",
  }),
  credits_exhausted: creditsPhrase,
  insufficient_credits: creditsPhrase,
  elevation_required: () => ({
    title: "This action needs a recent security check",
    hint: "Verify with your passkey or password to continue.",
  }),
  account_console_required: () => ({
    title: "Passkeys for this account are managed by your identity provider",
    hint: "Open the account console to manage them.",
  }),
};

function creditsPhrase(obj: Record<string, unknown>): Phrase {
  const resetsAt = typeof obj.resets_at === "string" ? formatResetDay(obj.resets_at) : undefined;
  return {
    title: "This workspace has used its weekly credits",
    hint: resetsAt
      ? `Credits reset on ${resetsAt}. Upgrade the plan for a higher weekly allowance.`
      : "Upgrade the plan for a higher weekly allowance, or wait for the weekly reset.",
  };
}

function limitTitle(noun: string, obj: Record<string, unknown>): string {
  const limit = typeof obj.limit === "number" ? obj.limit : undefined;
  if (limit === undefined) return `This workspace has reached its plan's ${noun} limit`;
  const current = typeof obj.current === "number" ? obj.current : limit;
  return `This workspace's plan allows ${limit} ${noun}${limit === 1 ? "" : "s"} and already has ${current}`;
}

function formatResetDay(iso: string): string | undefined {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return undefined;
  return d.toLocaleDateString(undefined, { weekday: "long" });
}

function envelopePhrase(code: string, obj: Record<string, unknown>): Phrase | undefined {
  return ENVELOPE_CODE_PHRASES[code]?.(obj);
}

/**
 * Friendly phrasing for HTTP statuses. It stands in for a response that
 * carries no explanation of its own: an empty, HTML or plain-text body, or an
 * envelope with nothing readable in it. A response that explains itself
 * renders that explanation, and a remedy is added only when a recognised code
 * or message maps to one (CODE_PHRASES, MESSAGE_PHRASES). The status alone
 * says nothing about the cause: a 403 for separation of duties refuses the
 * reviewer for who they are, and no grant fixes it.
 */
const STATUS_PHRASES: Record<number, Phrase> = {
  400: CODE_PHRASES.invalid,
  401: CODE_PHRASES.unauthorized,
  403: CODE_PHRASES.forbidden,
  404: CODE_PHRASES.not_found,
  409: CODE_PHRASES.conflict,
  413: {
    title: "That upload is too large",
    hint: "Try a smaller file.",
  },
  422: CODE_PHRASES.invalid,
  429: CODE_PHRASES.rate_limited,
  500: {
    title: "The server hit an unexpected error",
    hint: "Try again in a moment. If it keeps happening, contact support.",
  },
  502: {
    title: "The server is temporarily unreachable",
    hint: "Try again in a moment.",
  },
  503: {
    title: "The server is temporarily unavailable",
    hint: "Try again in a moment.",
  },
  504: {
    title: "The server took too long to respond",
    hint: "Try again in a moment.",
  },
};

/**
 * The refusals the server writes when the caller lacks a grant: `deny()` in
 * `bowrain/server/middleware_auth.go` and the review permission checks. These
 * are the one 403 family a workspace admin can fix, so they alone earn the
 * "ask an admin" remedy; every other 403 states its own reason. An entry
 * ending in a space is a prefix: the server appends what was refused (a
 * locale). A contract test holds each entry against the Go source.
 */
export const PERMISSION_REFUSALS: readonly string[] = [
  "insufficient permissions",
  "insufficient project permissions",
  "demoting a signed-off target requires review permission",
  "no access to language: ",
  "no review permission for ",
];

function isPermissionRefusal(message: string): boolean {
  return PERMISSION_REFUSALS.some((refusal) =>
    refusal.endsWith(" ") ? message.startsWith(refusal) : message === refusal,
  );
}

/**
 * Friendly phrasing for well-known server message strings (the bowrain server
 * returns lowercase message text in the `error` field rather than codes).
 * Matched case-insensitively; prefix matches where noted.
 */
const MESSAGE_PHRASES: Array<{ match: (msg: string) => boolean; phrase: Phrase }> = [
  {
    match: (m) => m.startsWith("rate limit exceeded"),
    phrase: CODE_PHRASES.rate_limited,
  },
  {
    match: (m) =>
      m === "invalid or expired token" ||
      m === "invalid or expired refresh token" ||
      m === "token has expired" ||
      m === "not authenticated" ||
      m === "missing authorization",
    phrase: CODE_PHRASES.unauthorized,
  },
  {
    match: (m) => m.startsWith("not a member of"),
    phrase: {
      title: "You're not a member of this workspace",
      hint: "Ask a workspace admin to invite you.",
    },
  },
  {
    match: isPermissionRefusal,
    phrase: CODE_PHRASES.forbidden,
  },
  {
    match: (m) => m === "not found",
    phrase: CODE_PHRASES.not_found,
  },
  {
    match: (m) => m === "slug is already in use" || m === "slug is reserved from a recent rename",
    phrase: {
      title: "That handle is already taken",
      hint: "Choose a different handle.",
    },
  },
  {
    match: (m) => m === "that email is already in use",
    phrase: {
      title: "That email address is already in use",
      hint: "Sign in with that address, or use a different one.",
    },
  },
];

/** One-shot JSON parse attempt for strings that look like JSON. */
function tryParseJson(text: string): unknown {
  const t = text.trim();
  if (!t.startsWith("{") && !t.startsWith("[")) return undefined;
  try {
    return JSON.parse(t) as unknown;
  } catch {
    return undefined;
  }
}

function sentenceCase(text: string): string {
  const t = text.trim();
  if (!t) return t;
  return t.charAt(0).toUpperCase() + t.slice(1);
}

function truncate(text: string): string {
  if (text.length <= MAX_TITLE_LENGTH) return text;
  return `${text.slice(0, MAX_TITLE_LENGTH - 1).trimEnd()}…`;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function statusPhrase(status: number | undefined): Phrase | undefined {
  if (status === undefined) return undefined;
  if (STATUS_PHRASES[status]) return STATUS_PHRASES[status];
  if (status >= 500) return STATUS_PHRASES[500];
  return undefined;
}

function messagePhrase(message: string): Phrase | undefined {
  const m = message.trim().toLowerCase();
  for (const entry of MESSAGE_PHRASES) {
    if (entry.match(m)) return entry.phrase;
  }
  return undefined;
}

/** Parse the `{error, message?, reference?, details?, code?, …}` REST envelope. */
function fromRestEnvelope(obj: Record<string, unknown>, status?: number, depth = 0): AppError {
  const message = String(obj.error);
  const code = typeof obj.code === "string" ? obj.code : undefined;
  const details = typeof obj.details === "string" ? obj.details : undefined;
  // The additive server envelope fields: a human sentence in `message` and the
  // correlation ID in `reference` (tolerate the `reference_id` spelling).
  const serverMessage =
    typeof obj.message === "string" && obj.message.trim() !== "" && obj.message !== message
      ? obj.message
      : undefined;
  const reference =
    typeof obj.reference === "string" && obj.reference !== ""
      ? obj.reference
      : typeof obj.reference_id === "string" && obj.reference_id !== ""
        ? obj.reference_id
        : undefined;
  // Nested envelopes get exactly one parse attempt overall.
  if (depth === 0) {
    const nested = tryParseJson(message);
    if (
      isRecord(nested) &&
      (typeof nested.error === "string" || typeof nested.message === "string")
    ) {
      const inner = fromObject(nested, status, depth + 1);
      return { ...inner, raw: obj, code: inner.code ?? code };
    }
  }
  const envPhrase = envelopePhrase(message, obj);
  const phrase = envPhrase ?? (code ? CODE_PHRASES[code] : undefined) ?? messagePhrase(message);
  // Without local copy, the server's own sentence beats sentence-casing a code.
  const readable = truncate(sentenceCase(serverMessage ?? message));
  if (phrase) {
    return {
      title: phrase.title,
      // Keep the original server message visible when we replaced it — except
      // for envelope-code copy, which already restates the server's sentence
      // with the same structured details.
      detail: details ?? (envPhrase || phrase.title === readable ? undefined : readable),
      hint: phrase.hint,
      raw: obj,
      code,
      status,
      reference,
    };
  }
  // The server's own sentence, unrecognised: it is the explanation, and a
  // per-status remedy would presume a cause the server did not name.
  return {
    title: readable,
    detail: details,
    raw: obj,
    code,
    status,
    reference,
  };
}

/** Parse the `{message, kind?, cause?}` client envelope. */
function fromClientEnvelope(obj: Record<string, unknown>, status?: number, depth = 0): AppError {
  const message = String(obj.message);
  // Nested envelopes get exactly one parse attempt overall.
  if (depth === 0) {
    const nested = tryParseJson(message);
    if (
      isRecord(nested) &&
      (typeof nested.error === "string" || typeof nested.message === "string")
    ) {
      const inner = fromObject(nested, status, depth + 1);
      return { ...inner, raw: obj };
    }
  }
  const kind = typeof obj.kind === "string" ? obj.kind : undefined;
  const cause = obj.cause;
  const causeText =
    cause instanceof Error ? cause.message : typeof cause === "string" ? cause : undefined;
  const phrase = messagePhrase(message);
  return {
    title: phrase ? phrase.title : truncate(sentenceCase(message)),
    detail: causeText ?? (phrase && kind === undefined ? truncate(sentenceCase(message)) : kind),
    hint: phrase?.hint,
    raw: obj,
    status,
  };
}

function fromObject(obj: Record<string, unknown>, status?: number, depth = 0): AppError {
  if (typeof obj.error === "string" && obj.error.trim() !== "") {
    return fromRestEnvelope(obj, status, depth);
  }
  if (typeof obj.message === "string" && obj.message.trim() !== "") {
    return fromClientEnvelope(obj, status, depth);
  }
  const phrase = statusPhrase(status);
  return {
    title: phrase?.title ?? GENERIC_TITLE,
    hint: phrase?.hint,
    raw: obj,
    status,
  };
}

/** Parse a message string, handling the `"<status>: <body>"` adapter pattern. */
function fromString(text: string, fallbackTitle: string): AppError {
  const trimmed = text.trim();
  if (trimmed === "") return { title: fallbackTitle };

  // RestApiAdapter throws `Error("<status>: <raw body>")`.
  const statusMatch = /^(\d{3}):\s*([\s\S]*)$/.exec(trimmed);
  const status = statusMatch ? Number(statusMatch[1]) : undefined;
  const body = statusMatch ? statusMatch[2].trim() : trimmed;

  // Nested JSON bodies get exactly one parse attempt.
  const parsed = tryParseJson(body);
  if (isRecord(parsed)) return fromObject(parsed, status, 1);
  if (Array.isArray(parsed)) {
    const phrase = statusPhrase(status);
    return {
      title: phrase?.title ?? fallbackTitle,
      hint: phrase?.hint,
      raw: parsed,
      status,
    };
  }

  const phrase = statusPhrase(status);
  if (status !== undefined) {
    // HTML or empty bodies (proxies, gateways) are noise — keep the phrasing only.
    const isNoise = body === "" || body.startsWith("<");
    if (phrase) {
      return {
        title: phrase.title,
        detail: isNoise ? undefined : truncate(sentenceCase(body)),
        hint: phrase.hint,
        status,
      };
    }
    return {
      title: isNoise ? fallbackTitle : truncate(sentenceCase(body)),
      status,
    };
  }

  const msgPhrase = messagePhrase(body);
  if (msgPhrase) {
    const readable = truncate(sentenceCase(body));
    return {
      title: msgPhrase.title,
      detail: msgPhrase.title === readable ? undefined : readable,
      hint: msgPhrase.hint,
    };
  }
  return { title: truncate(sentenceCase(body)) };
}

/**
 * Turn any error value into a human-readable `{title, detail?, hint?, raw?}`.
 *
 * @param input anything that was thrown, rejected, or returned as an error
 * @param fallbackTitle title to use when nothing readable can be extracted
 */
export function parseAppError(input: unknown, fallbackTitle: string = GENERIC_TITLE): AppError {
  if (input === null || input === undefined) return { title: fallbackTitle };

  // The REST adapter's typed error: rebuild the presentation from the parsed
  // envelope body (one code path with raw-object envelopes), then fill status
  // and reference from the structured fields — the header-derived reference is
  // available even when the body carried none.
  if (input instanceof ApiError) {
    const base = isRecord(input.body)
      ? fromObject(input.body, input.status, 0)
      : fromString(`${input.status}: ${input.bodyText}`, fallbackTitle);
    if (base.status === undefined) base.status = input.status;
    if (base.reference === undefined && input.referenceId !== undefined) {
      base.reference = input.referenceId;
    }
    return base;
  }

  if (input instanceof Error) {
    const result = fromString(input.message, fallbackTitle);
    // Pick up structured fields some call sites attach to Error instances.
    const carrier = input as Error & {
      status?: unknown;
      code?: unknown;
      cause?: unknown;
      reference?: unknown;
    };
    if (result.reference === undefined && typeof carrier.reference === "string") {
      result.reference = carrier.reference;
    }
    if (result.status === undefined && typeof carrier.status === "number") {
      result.status = carrier.status;
      // The status phrase stands in only when the message itself said nothing.
      const phrase = statusPhrase(carrier.status);
      if (phrase && result.title === fallbackTitle) {
        result.title = phrase.title;
        result.hint = phrase.hint;
      }
    }
    if (result.code === undefined && typeof carrier.code === "string") {
      result.code = carrier.code;
      const phrase = CODE_PHRASES[carrier.code];
      if (phrase) {
        if (result.title === fallbackTitle) result.title = phrase.title;
        if (!result.hint) result.hint = phrase.hint;
      }
    }
    if (!result.detail && carrier.cause instanceof Error) {
      result.detail = truncate(sentenceCase(carrier.cause.message));
    }
    return result;
  }

  if (typeof input === "string") return fromString(input, fallbackTitle);

  if (isRecord(input)) return fromObject(input);

  if (Array.isArray(input)) return { title: fallbackTitle, raw: input };

  return { title: fallbackTitle };
}

/** True when the parsed error carries a structured payload worth a "Details" view. */
export function hasStructuredRaw(parsed: AppError): boolean {
  return typeof parsed.raw === "object" && parsed.raw !== null;
}

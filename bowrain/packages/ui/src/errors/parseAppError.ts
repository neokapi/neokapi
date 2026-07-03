/**
 * parseAppError — one place that turns any thrown/returned error value into a
 * human-readable summary, while preserving the raw structured payload for an
 * optional "Details" disclosure.
 *
 * Known shapes handled:
 * - REST envelope from bowrain-server: `{ error: string, details?: string, code?: string }`
 * - Client-side envelope: `{ message: string, kind?: string, cause?: unknown }`
 * - `Error` instances, including the RestApiAdapter pattern
 *   `Error("<httpStatus>: <raw body>")` where the body may itself be JSON
 * - Plain strings (one JSON parse attempt for nested envelopes)
 */

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
    hint: "Your session may have expired — sign in and try again.",
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

/** Friendly phrasing for HTTP statuses (used when the body carries no better message). */
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
    match: (m) => m === "not found",
    phrase: CODE_PHRASES.not_found,
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

/** Parse the `{error, details?, code?}` REST envelope. */
function fromRestEnvelope(obj: Record<string, unknown>, status?: number, depth = 0): AppError {
  const message = String(obj.error);
  const code = typeof obj.code === "string" ? obj.code : undefined;
  const details = typeof obj.details === "string" ? obj.details : undefined;
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
  const phrase = (code ? CODE_PHRASES[code] : undefined) ?? messagePhrase(message);
  const readable = truncate(sentenceCase(message));
  if (phrase) {
    return {
      title: phrase.title,
      // Keep the original server message visible when we replaced it.
      detail: details ?? (phrase.title === readable ? undefined : readable),
      hint: phrase.hint,
      raw: obj,
      code,
      status,
    };
  }
  return {
    title: readable,
    detail: details,
    hint: statusPhrase(status)?.hint,
    raw: obj,
    code,
    status,
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
    hint: phrase?.hint ?? statusPhrase(status)?.hint,
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

  if (input instanceof Error) {
    const result = fromString(input.message, fallbackTitle);
    // Pick up structured fields some call sites attach to Error instances.
    const carrier = input as Error & { status?: unknown; code?: unknown; cause?: unknown };
    if (result.status === undefined && typeof carrier.status === "number") {
      result.status = carrier.status;
      const phrase = statusPhrase(carrier.status);
      if (phrase && !result.hint) result.hint = phrase.hint;
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

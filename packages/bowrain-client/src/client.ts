/**
 * The Bowrain server API as typed calls. The client attaches the access token,
 * refreshes it once on a 401, and returns the server's answers as they are:
 * which cards make a deck, in which order, which fixes apply and who may decide
 * are all computed by the server.
 */

import type { ChangeResult, ChangeSet } from "@neokapi/contract-types";
import type { Run } from "@neokapi/kapi-format";

import type {
  AuthTokens,
  BlockInfo,
  CallerPermissions,
  Deck,
  DeckCard,
  DeckCounts,
  DeckGroup,
  DeckNote,
  FeedPage,
  FeedQuery,
  FixCandidate,
  FixCandidates,
  FixReason,
  PendingReviewEntry,
  PendingReviewPage,
  PreReview,
  Project,
  ReviewContext,
  User,
  Workspace,
} from "./types.ts";

/** A response the server answered with an error status. */
export class ApiError extends Error {
  readonly status: number;
  readonly body: unknown;

  constructor(status: number, message: string, body: unknown) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.body = body;
  }
}

/** Whether an error means the server or the network could not be reached. */
export function isNetworkError(err: unknown): boolean {
  return err instanceof TypeError || (err instanceof ApiError && err.status >= 500);
}

export interface BowrainClientOptions {
  /** The server's origin, such as `https://bowrain.cloud` or `http://localhost:8080`. */
  baseUrl: string;
  /** The current access token, or null when signed out. */
  getAccessToken: () => string | null | Promise<string | null>;
  /**
   * Obtain a fresh access token after a 401, or null when the session is over.
   * Called at most once per request.
   */
  onUnauthorized?: () => Promise<string | null>;
  fetch?: typeof fetch;
}

/** Where a project's review content lives: a workspace, a project and a stream. */
export interface ProjectScope {
  workspace: string;
  project: string;
  /** The stream (ref); `main` when omitted. */
  stream?: string;
}

export interface BowrainClient {
  readonly baseUrl: string;
  /**
   * The URL that starts sign-in in a browser. The server brokers the OIDC flow
   * with the workspace's identity provider and redirects back with a one-time
   * code and the client's `state`, never with a token.
   */
  loginUrl(redirectUri: string, codeChallenge: string, state: string): string;
  /** Redeem the one-time code with the PKCE verifier; the tokens arrive in the response body. */
  exchangeCode(code: string, codeVerifier: string, redirectUri: string): Promise<AuthTokens>;
  refresh(refreshToken: string): Promise<AuthTokens>;
  /**
   * End the session on the server. The answer carries the identity provider's
   * sign-out URL when it has one; opening it ends the provider's own session,
   * so the next sign-in asks for an account again. A native app passes the
   * redirect its browser session waits for.
   */
  logout(opts?: { postLogoutRedirectUri?: string }): Promise<{ endSessionUrl?: string }>;
  me(): Promise<User>;
  workspaces(): Promise<Workspace[]>;
  projects(workspace: string): Promise<Project[]>;
  permissions(workspace: string, project: string): Promise<CallerPermissions>;
  pendingReview(
    scope: ProjectScope,
    opts?: { locales?: string[]; limit?: number; offset?: number },
  ): Promise<PendingReviewPage>;
  /** The number of translations waiting for review in one language. */
  pendingCount(scope: ProjectScope, locale: string): Promise<number>;
  /** One page of the review deck for a language, in the server's group order. */
  deck(
    scope: ProjectScope,
    locale: string,
    opts?: { limit?: number; cursor?: string; group?: DeckGroup },
  ): Promise<Deck>;
  reviewContext(scope: ProjectScope, blockId: string, locale: string): Promise<ReviewContext>;
  /** Ready-made fixes for one card, or null when the server has no such route. */
  fixCandidates(
    scope: ProjectScope,
    card: Pick<DeckCard, "blockId" | "locale">,
    opts?: { reason?: FixReason; ai?: boolean },
  ): Promise<FixCandidates | null>;
  /** Send a change set. A refusal answers with its result, not an exception. */
  applyChanges(scope: ProjectScope, set: ChangeSet): Promise<ChangeResult>;
  /** One page of a project's feed, newest first. */
  feed(workspace: string, project: string, query?: FeedQuery): Promise<FeedPage>;
}

const DEFAULT_STREAM = "main";

/** Paths of the review endpoints (the "API for the first version" section of the design note). */
export const ROUTES = {
  deck: (s: ProjectScope) => `/${enc(s.workspace)}/${enc(s.project)}/review-deck/${stream(s)}`,
  fixCandidates: (s: ProjectScope, blockId: string) =>
    `/${enc(s.workspace)}/${enc(s.project)}/blocks/${stream(s)}/${enc(blockId)}/fix-candidates`,
  changes: (s: ProjectScope) =>
    `/${enc(s.workspace)}/projects/${enc(s.project)}/streams/${stream(s)}/changes`,
  feed: (workspace: string, project: string) => `/${enc(workspace)}/${enc(project)}/feed`,
};

/** The query string of a feed request; `coordinate` repeats once per axis. */
export function feedQueryString(q: FeedQuery = {}): string {
  const parts: string[] = [];
  const add = (k: string, v: string | number | undefined) => {
    if (v !== undefined && v !== "") parts.push(`${enc(k)}=${enc(String(v))}`);
  };
  add("kind", q.kinds?.join(","));
  add("locale", q.locale);
  for (const [axis, value] of Object.entries(q.coordinates ?? {}))
    add("coordinate", `${axis}:${value}`);
  add("actor", q.actor);
  add("actor_kind", q.actorKind);
  add("since", q.since);
  add("cursor", q.cursor);
  add("limit", q.limit);
  return parts.length ? `?${parts.join("&")}` : "";
}

function enc(s: string): string {
  return encodeURIComponent(s);
}

function stream(s: ProjectScope): string {
  return enc(s.stream || DEFAULT_STREAM);
}

function query(params: Record<string, string | number | undefined>): string {
  const q = Object.entries(params)
    .filter(([, v]) => v !== undefined && v !== "")
    .map(([k, v]) => `${enc(k)}=${enc(String(v))}`)
    .join("&");
  return q ? `?${q}` : "";
}

export function createBowrainClient(opts: BowrainClientOptions): BowrainClient {
  const baseUrl = opts.baseUrl.replace(/\/+$/, "");
  const api = `${baseUrl}/api/v1`;
  const doFetch = opts.fetch ?? fetch;

  async function send(
    path: string,
    init: RequestInit = {},
    accept: (status: number) => boolean = (s) => s >= 200 && s < 300,
  ): Promise<{ status: number; body: unknown }> {
    const attempt = async (token: string | null) => {
      const headers: Record<string, string> = { Accept: "application/json" };
      if (init.body) headers["Content-Type"] = "application/json";
      if (token) headers.Authorization = `Bearer ${token}`;
      return doFetch(`${api}${path}`, { ...init, headers });
    };
    let res = await attempt(await opts.getAccessToken());
    if (res.status === 401 && opts.onUnauthorized) {
      const fresh = await opts.onUnauthorized();
      if (fresh) res = await attempt(fresh);
    }
    const text = await res.text();
    let body: unknown = undefined;
    if (text) {
      try {
        body = JSON.parse(text);
      } catch {
        body = text;
      }
    }
    if (!accept(res.status)) {
      const message =
        (body as { error?: string } | undefined)?.error ??
        `request failed with status ${res.status}`;
      throw new ApiError(res.status, message, body);
    }
    return { status: res.status, body };
  }

  async function postTokens(path: string, payload: object, failure: string): Promise<AuthTokens> {
    const res = await doFetch(`${api}${path}`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Accept: "application/json" },
      body: JSON.stringify(payload),
    });
    const body = (await res.json().catch(() => undefined)) as AuthTokens | undefined;
    if (!res.ok || !body?.access_token) throw new ApiError(res.status, failure, body);
    return body;
  }

  async function get<T>(path: string): Promise<T> {
    return (await send(path)).body as T;
  }

  const client: BowrainClient = {
    baseUrl,

    loginUrl(redirectUri, codeChallenge, state) {
      return `${api}/auth/desktop/login${query({
        redirect_uri: redirectUri,
        code_challenge: codeChallenge,
        code_challenge_method: "S256",
        state,
        response_mode: "code",
      })}`;
    },

    exchangeCode(code, codeVerifier, redirectUri) {
      return postTokens(
        "/auth/desktop/token",
        { code, code_verifier: codeVerifier, redirect_uri: redirectUri },
        "the sign-in code was not accepted",
      );
    },

    refresh(refreshToken) {
      return postTokens(
        "/auth/refresh",
        { refresh_token: refreshToken },
        "the session could not be refreshed",
      );
    },

    async logout(o = {}) {
      const init: RequestInit = { method: "POST" };
      if (o.postLogoutRedirectUri) {
        init.body = JSON.stringify({ post_logout_redirect_uri: o.postLogoutRedirectUri });
      }
      const { status, body } = await send("/auth/logout", init, () => true);
      const url = (body as { end_session_url?: unknown } | undefined)?.end_session_url;
      return status === 200 && typeof url === "string" && url !== "" ? { endSessionUrl: url } : {};
    },

    me: () => get<User>("/auth/me"),
    workspaces: () => get<Workspace[]>("/workspaces"),
    projects: (ws) => get<Project[]>(`/${enc(ws)}/projects`),
    permissions: (ws, pid) => get<CallerPermissions>(`/${enc(ws)}/${enc(pid)}/permissions`),

    pendingReview(scope, o = {}) {
      return get<PendingReviewPage>(
        `/${enc(scope.workspace)}/${enc(scope.project)}/pending-review/${stream(scope)}${query({
          locales: o.locales?.join(","),
          limit: o.limit,
          offset: o.offset,
        })}`,
      );
    },

    async pendingCount(scope, locale) {
      const page = await client.pendingReview(scope, { locales: [locale], limit: 1 });
      return page.total;
    },

    async deck(scope, locale, o = {}) {
      try {
        const body = await get<ServerDeck>(
          `${ROUTES.deck(scope)}${query({ locale, limit: o.limit, cursor: o.cursor, group: o.group })}`,
        );
        return deckFromServer(body, locale);
      } catch (err) {
        if (!(err instanceof ApiError) || (err.status !== 404 && err.status !== 405)) throw err;
      }
      // A server without the deck route: the pending-review queue, in the
      // server's order, ungrouped and on one page.
      if (o.cursor) return { locale, cards: [], total: 0, ordered: false };
      const page = await client.pendingReview(scope, { locales: [locale], limit: o.limit ?? 200 });
      return {
        locale,
        cards: page.entries.flatMap((e) => {
          const card = cardFromEntry(e);
          return card ? [card] : [];
        }),
        total: page.total,
        ordered: false,
      };
    },

    reviewContext(scope, blockId, locale) {
      return get<ReviewContext>(
        `/${enc(scope.workspace)}/${enc(scope.project)}/blocks/${stream(scope)}/${enc(blockId)}/review-context${query(
          { target_locale: locale },
        )}`,
      );
    },

    async fixCandidates(scope, card, o = {}) {
      try {
        const body = await get<ServerFixCandidates>(
          `${ROUTES.fixCandidates(scope, card.blockId)}${query({
            locale: card.locale,
            reason: o.reason,
            ai: o.ai ? "true" : undefined,
          })}`,
        );
        return fixesFromServer(body);
      } catch (err) {
        if (err instanceof ApiError && (err.status === 404 || err.status === 405)) return null;
        throw err;
      }
    },

    async applyChanges(scope, set) {
      // The change route answers a refusal with its result and the refusal's
      // status (409 stale, 422 guard, 403 not permitted), so every status with
      // a result body is an answer.
      const { status, body } = await send(
        ROUTES.changes(scope),
        { method: "POST", body: JSON.stringify(set) },
        (s) => s < 500 && s !== 401,
      );
      if (!isChangeResult(body)) {
        const message =
          (body as { error?: string } | undefined)?.error ?? "the change was not accepted";
        throw new ApiError(status, message, body);
      }
      return body;
    },

    async feed(ws, pid, q) {
      const page = await get<FeedPage>(`${ROUTES.feed(ws, pid)}${feedQueryString(q)}`);
      return { entries: page.entries ?? [], next_cursor: page.next_cursor || undefined };
    },
  };
  return client;
}

function isChangeResult(body: unknown): body is ChangeResult {
  return typeof body === "object" && body !== null && "status" in body && "ops" in body;
}

/** A block's source as runs: its typed runs, or its text as one run. */
export function sourceRunsOf(block: BlockInfo): Run[] {
  if (block.source_runs && block.source_runs.length > 0) return block.source_runs;
  return [{ text: block.source }];
}

/** A block's translation in one locale as runs: its typed runs, or its text as one run. */
export function targetRunsOf(block: BlockInfo, locale: string): Run[] {
  const runs = block.targets_runs?.[locale];
  if (runs && runs.length > 0) return runs;
  const t = block.targets?.[locale];
  return t?.text ? [{ text: t.text }] : [];
}

/** A card from a pending-review entry, or null when the entry has no block or revision. */
export function cardFromEntry(e: PendingReviewEntry): DeckCard | null {
  const block = e.block;
  if (!block) return null;
  const rev = block.target_revisions?.[e.locale];
  if (!rev) return null;
  return {
    key: `${e.block_id}:${e.locale}`,
    blockId: e.block_id,
    item: e.item_name,
    locale: e.locale,
    collectionId: e.collection_id,
    name: block.name,
    source: sourceRunsOf(block),
    target: targetRunsOf(block, e.locale),
    targetRevision: rev,
    sourceRevision: block.source_revision,
    group: "unsorted",
    preReview: e.pre_review,
    notes: [],
    findingsCount: 0,
    failingFindingsCount: 0,
    ownWork: false,
  };
}

/** One card of the server's deck (GET /:ws/:id/review-deck/:ref). */
export interface ServerDeckCard {
  block_id: string;
  item_name: string;
  key?: string;
  collection_id?: string;
  locale: string;
  source_runs?: Run[];
  target_runs?: Run[];
  source_text?: string;
  target_text?: string;
  source_revision?: string;
  target_revision: string;
  target_status?: string;
  pre_review?: PreReview;
  findings_count?: number;
  failing_findings_count?: number;
  term_compliance?: string;
  voice_score?: number;
  voice_bar?: number;
  notes?: DeckNote[];
  own_work?: boolean;
  group?: DeckGroup;
  reason?: string;
}

/** A page of the server's deck. */
export interface ServerDeck {
  locale?: string;
  cards?: ServerDeckCard[];
  counts?: DeckCounts;
  next_cursor?: string;
}

/** A card from the server's deck. */
export function cardFromServer(c: ServerDeckCard): DeckCard {
  let target: Run[] = [];
  if (c.target_runs && c.target_runs.length > 0) target = c.target_runs;
  else if (c.target_text) target = [{ text: c.target_text }];
  return {
    key: `${c.block_id}:${c.locale}`,
    blockId: c.block_id,
    item: c.item_name,
    locale: c.locale,
    collectionId: c.collection_id ?? "",
    name: c.key,
    source:
      c.source_runs && c.source_runs.length > 0 ? c.source_runs : [{ text: c.source_text ?? "" }],
    target,
    targetRevision: c.target_revision,
    sourceRevision: c.source_revision,
    group: c.group ?? "unsorted",
    reason: c.reason,
    preReview: c.pre_review,
    notes: c.notes ?? [],
    findingsCount: c.findings_count ?? 0,
    failingFindingsCount: c.failing_findings_count ?? 0,
    ownWork: c.own_work ?? false,
  };
}

function deckFromServer(body: ServerDeck, locale: string): Deck {
  const cards = (body.cards ?? []).map(cardFromServer);
  return {
    locale: body.locale ?? locale,
    cards,
    total: body.counts?.total ?? cards.length,
    counts: body.counts,
    nextCursor: body.next_cursor || undefined,
    ordered: true,
  };
}

interface ServerFixCandidates {
  target_revision?: string;
  candidates?: (Omit<FixCandidate, "id" | "reasons"> & { reasons?: string[] })[];
  dropped?: FixCandidates["dropped"];
  notes?: string[];
}

function fixesFromServer(body: ServerFixCandidates): FixCandidates {
  return {
    candidates: (body.candidates ?? []).map((c, i) => ({
      ...c,
      id: `${c.kind}-${i}`,
      reasons: c.reasons ?? [],
    })),
    dropped: body.dropped ?? [],
    notes: body.notes ?? [],
    targetRevision: body.target_revision,
  };
}

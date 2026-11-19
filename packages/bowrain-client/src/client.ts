/**
 * The Bowrain server API as typed calls. The client attaches the access token,
 * refreshes it once on a 401, and returns the server's answers as they are:
 * which cards make a deck, in which order, which fixes apply and who may decide
 * are all computed by the server.
 */

import type { ChangeResult, ChangeSet, OverlayView } from "@neokapi/contract-types";
import type { Run } from "@neokapi/kapi-format";

import {
  planFromServer,
  planScanFromServer,
  type PlanScan,
  type ReviewInvolvement,
  type ReviewPlan,
  type ServerReviewPlan,
} from "./plan.ts";
import type {
  Concept,
  AuthTokens,
  BlockInfo,
  CallerPermissions,
  ContextCard,
  ContextCardKind,
  ContextDeck,
  ContextOriginPage,
  ContextRuleExamples,
  ContextDecision,
  ContextDecisionResult,
  TermProposalsResult,
  Deck,
  DeckCard,
  DeckCollections,
  DeckCounts,
  DeckGroup,
  DeckNote,
  FeedDecisions,
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
  ReviewHome,
  ReviewInboxLanguage,
  User,
  Workspace,
  ReviewContainer,
  DeckPage,
  DeckPages,
  StoryIndex,
  ContainerUnit,
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
  /** Changes the signed-in person's own account; an empty name clears it. */
  updateMe(change: { name?: string }): Promise<User>;
  workspaces(): Promise<Workspace[]>;
  projects(workspace: string): Promise<Project[]>;
  permissions(workspace: string, project: string): Promise<CallerPermissions>;
  /**
   * Every language the caller may review, across workspaces, where anything
   * is waiting, most waiting first. Null from a server without the route.
   */
  reviewInbox(): Promise<ReviewInboxLanguage[] | null>;
  /**
   * The review inbox whole: the projects whose context waits for the caller,
   * then the languages. Null from a server without the route.
   */
  reviewHome(): Promise<ReviewHome | null>;
  /** One page of a project's context deck: the items nobody reviewed at their current revision. */
  contextDeck(
    workspace: string,
    project: string,
    opts?: { limit?: number; cursor?: string; kind?: ContextCardKind },
  ): Promise<ContextDeck>;
  /** One page of the items the bulk operation `op` brought into a project, reviewed or not. */
  contextOrigin(
    workspace: string,
    project: string,
    op: string,
    opts?: { limit?: number; cursor?: string },
  ): Promise<ContextOriginPage>;
  /**
   * Up to three places a voice rule matches the project's source text, found by a
   * bounded walk. `profile` and `id` are the card's `item.profile` and `item.id`.
   */
  contextRuleExamples(
    workspace: string,
    project: string,
    rule: { profile: string; id: string },
  ): Promise<ContextRuleExamples>;
  /** Keep, reject or put off one context card. A refusal throws an ApiError whose body carries `code`. */
  decideContext(
    workspace: string,
    project: string,
    decision: ContextDecision,
  ): Promise<ContextDecisionResult>;
  /**
   * Start proposing, with AI, the term each concept of the project takes in
   * each target language that has none. The proposals arrive in the context deck.
   */
  proposeTermTranslations(workspace: string, project: string): Promise<TermProposalsResult>;
  pendingReview(
    scope: ProjectScope,
    opts?: { locales?: string[]; limit?: number; offset?: number },
  ): Promise<PendingReviewPage>;
  /** The number of translations waiting for review in one language. */
  pendingCount(scope: ProjectScope, locale: string): Promise<number>;
  /**
   * One page of the review deck for a language, in the server's group order.
   * `collection` narrows it to one collection; an empty string selects the
   * blocks in no collection. `rule` narrows it to the translations with a
   * finding of one rule (a review plan rule's `key`).
   */
  deck(
    scope: ProjectScope,
    locale: string,
    opts?: {
      limit?: number;
      cursor?: string;
      group?: DeckGroup;
      collection?: string;
      rule?: string;
    },
  ): Promise<Deck>;
  /**
   * A language's review deck split by collection, in the server's order. Null
   * from a server without the route.
   */
  deckCollections(scope: ProjectScope, locale: string): Promise<DeckCollections | null>;
  /**
   * A language's review deck by page: each component and each document with
   * its waiting count. `collection` narrows it to one collection; `sample`
   * keeps the pages a reviewer reads under the involvement Context (one page
   * in ten per collection, and every page with a failing finding). Null from
   * a server without the route.
   */
  deckPages(
    scope: ProjectScope,
    locale: string,
    opts?: { collection?: string; sample?: boolean },
  ): Promise<DeckPages | null>;
  reviewContext(scope: ProjectScope, blockId: string, locale: string): Promise<ReviewContext>;
  /** The block's container: its component, its section, or the blocks around it. */
  reviewContainer(scope: ProjectScope, blockId: string, locale: string): Promise<ReviewContainer>;
  /** The story index of a collection's preview host, read through the server. */
  storyIndex(scope: ProjectScope, collectionId: string): Promise<StoryIndex>;
  /** One concept of the workspace terms store, with its terms in every language. */
  concept(workspace: string, conceptId: string): Promise<Concept>;
  /** The project's review plan for the caller. */
  reviewPlan(scope: ProjectScope): Promise<ReviewPlan>;
  /** Confirms the project's coordinates as the plan lists them. */
  confirmCoordinates(scope: ProjectScope): Promise<void>;
  /** Sets how much the project's reviewers can be involved. */
  setInvolvement(scope: ProjectScope, involvement: ReviewInvolvement): Promise<void>;
  /**
   * The plan's last step: establishes every waiting translation that clears
   * every bar, recorded with the reason "review policy". The server refuses
   * (409) once the first pass is taken and the key language is still open.
   */
  settlePlan(
    scope: ProjectScope,
    locales: string[],
  ): Promise<{ approved: number; skipped: number; firstPass: boolean }>;
  /** Starts a scan of the project for candidate terms. */
  startTermScan(scope: ProjectScope): Promise<PlanScan>;
  /** Establishes every waiting translation that clears every bar, in the languages given. */
  approvePassing(
    scope: ProjectScope,
    locales: string[],
  ): Promise<{ approved: number; skipped: number }>;
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
  /** The decisions a review entry of the feed counts, newest first. */
  feedDecisions(workspace: string, project: string, entry: string): Promise<FeedDecisions>;
}

const DEFAULT_STREAM = "main";

/** Paths of the review endpoints (the "API for the first version" section of the design note). */
export const ROUTES = {
  deck: (s: ProjectScope) => `/${enc(s.workspace)}/${enc(s.project)}/review-deck/${stream(s)}`,
  deckCollections: (s: ProjectScope) =>
    `/${enc(s.workspace)}/${enc(s.project)}/review-deck/${stream(s)}/collections`,
  fixCandidates: (s: ProjectScope, blockId: string) =>
    `/${enc(s.workspace)}/${enc(s.project)}/blocks/${stream(s)}/${enc(blockId)}/fix-candidates`,
  changes: (s: ProjectScope) =>
    `/${enc(s.workspace)}/projects/${enc(s.project)}/streams/${stream(s)}/changes`,
  feed: (workspace: string, project: string) => `/${enc(workspace)}/${enc(project)}/feed`,
  contextDeck: (workspace: string, project: string) =>
    `/${enc(workspace)}/${enc(project)}/context/deck`,
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
    async updateMe(change) {
      const { body } = await send("/auth/me", { method: "PATCH", body: JSON.stringify(change) });
      return body as User;
    },
    workspaces: () => get<Workspace[]>("/workspaces"),
    projects: (ws) => get<Project[]>(`/${enc(ws)}/projects`),
    permissions: (ws, pid) => get<CallerPermissions>(`/${enc(ws)}/${enc(pid)}/permissions`),

    async reviewInbox() {
      const home = await client.reviewHome();
      return home ? home.languages : null;
    },

    async reviewHome() {
      try {
        const body = await get<Partial<ReviewHome>>("/auth/me/review");
        return { context: body.context ?? [], languages: body.languages ?? [] };
      } catch (err) {
        if (err instanceof ApiError && (err.status === 404 || err.status === 405)) return null;
        throw err;
      }
    },

    async contextDeck(ws, pid, o = {}) {
      const deck = await get<ContextDeck>(
        `${ROUTES.contextDeck(ws, pid)}${query({ mode: "pending", limit: o.limit, cursor: o.cursor, kind: o.kind })}`,
      );
      return {
        ...deck,
        counts: { ...deck.counts, proposed: deck.counts?.proposed ?? 0 },
        cards: (deck.cards ?? []).map(contextCard),
        next_cursor: deck.next_cursor || undefined,
      };
    },

    async contextOrigin(ws, pid, op, o = {}) {
      const page = await get<ContextOriginPage>(
        `${ROUTES.contextDeck(ws, pid)}/origins/${enc(op)}${query({ limit: o.limit, cursor: o.cursor })}`,
      );
      return {
        ...page,
        cards: (page.cards ?? []).map(contextCard),
        next_cursor: page.next_cursor || undefined,
      };
    },

    async contextRuleExamples(ws, pid, rule) {
      const out = await get<ContextRuleExamples>(
        `${ROUTES.contextDeck(ws, pid)}/examples${query({ profile: rule.profile, id: rule.id })}`,
      );
      return { ...out, examples: out.examples ?? [] };
    },

    decideContext(ws, pid, decision) {
      return send(`${ROUTES.contextDeck(ws, pid)}/decide`, {
        method: "POST",
        body: JSON.stringify(decision),
      }).then((r) => r.body as ContextDecisionResult);
    },

    proposeTermTranslations(ws, pid) {
      return send(`/${enc(ws)}/${enc(pid)}/context/term-proposals`, { method: "POST" }).then(
        (r) => r.body as TermProposalsResult,
      );
    },

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
          `${ROUTES.deck(scope)}${query({ locale, limit: o.limit, cursor: o.cursor, group: o.group, rule: o.rule })}${
            o.collection === undefined ? "" : `&collection=${enc(o.collection)}`
          }`,
        );
        return deckFromServer(body, locale);
      } catch (err) {
        if (!(err instanceof ApiError) || (err.status !== 404 && err.status !== 405)) throw err;
      }
      // A server without the deck route: the pending-review queue, in the
      // server's order, ungrouped and on one page. That queue has no
      // collection filter, so a collection's deck reads as empty.
      if (o.cursor || o.collection !== undefined || o.rule)
        return { locale, cards: [], total: 0, ordered: false };
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

    async deckCollections(scope, locale) {
      try {
        return await get<DeckCollections>(`${ROUTES.deckCollections(scope)}${query({ locale })}`);
      } catch (err) {
        if (err instanceof ApiError && (err.status === 404 || err.status === 405)) return null;
        throw err;
      }
    },

    async deckPages(scope, locale, o = {}) {
      try {
        const body = await get<ServerDeckPages>(
          `/${enc(scope.workspace)}/${enc(scope.project)}/review-deck/${stream(scope)}/pages${query({ locale })}${
            o.collection === undefined ? "" : `&collection=${enc(o.collection)}`
          }${o.sample ? "&sample=true" : ""}`,
        );
        return deckPagesFromServer(body, locale);
      } catch (err) {
        if (err instanceof ApiError && (err.status === 404 || err.status === 405)) return null;
        throw err;
      }
    },

    reviewContext(scope, blockId, locale) {
      return get<ReviewContext>(
        `/${enc(scope.workspace)}/${enc(scope.project)}/blocks/${stream(scope)}/${enc(blockId)}/review-context${query(
          { target_locale: locale },
        )}`,
      );
    },

    async reviewContainer(scope, blockId, locale) {
      const body = await get<ServerContainer>(
        `/${enc(scope.workspace)}/${enc(scope.project)}/blocks/${stream(scope)}/${enc(blockId)}/container${query(
          { target_locale: locale },
        )}`,
      );
      return containerFromServer(body);
    },

    storyIndex(scope, collectionId) {
      return get<StoryIndex>(
        `/${enc(scope.workspace)}/${enc(scope.project)}/collections/${stream(scope)}/${enc(collectionId)}/preview/index`,
      );
    },

    async reviewPlan(scope) {
      return planFromServer(
        await get<ServerReviewPlan>(
          `/${enc(scope.workspace)}/${enc(scope.project)}/review/plan${query({ stream: scope.stream })}`,
        ),
      );
    },

    async confirmCoordinates(scope) {
      await send(`/${enc(scope.workspace)}/${enc(scope.project)}/review/plan/coordinates`, {
        method: "POST",
      });
    },

    async setInvolvement(scope, involvement) {
      await send(`/${enc(scope.workspace)}/${enc(scope.project)}/review/plan/involvement`, {
        method: "PUT",
        body: JSON.stringify({ involvement }),
      });
    },

    async settlePlan(scope, locales) {
      const { body } = await send(
        `/${enc(scope.workspace)}/${enc(scope.project)}/review/plan/settle`,
        { method: "POST", body: JSON.stringify({ stream: scope.stream, locales }) },
      );
      const r = body as { approved?: number; skipped?: number; first_pass?: boolean } | undefined;
      return {
        approved: r?.approved ?? 0,
        skipped: r?.skipped ?? 0,
        firstPass: r?.first_pass ?? false,
      };
    },

    async startTermScan(scope) {
      const { body } = await send(
        `/${enc(scope.workspace)}/${enc(scope.project)}/review/plan/scan`,
        { method: "POST" },
      );
      return planScanFromServer((body as { scan: Parameters<typeof planScanFromServer>[0] }).scan);
    },

    async approvePassing(scope, locales) {
      const { body } = await send(
        `/${enc(scope.workspace)}/${enc(scope.project)}/review/approve-passing`,
        { method: "POST", body: JSON.stringify({ stream: scope.stream, locales }) },
      );
      const r = body as { approved?: number; skipped?: number } | undefined;
      return { approved: r?.approved ?? 0, skipped: r?.skipped ?? 0 };
    },

    async concept(workspace, conceptId) {
      return conceptFromServer(
        await get<ServerConcept>(`/${enc(workspace)}/concepts/${enc(conceptId)}`),
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

    async feedDecisions(ws, pid, entry) {
      const body = await get<ServerFeedDecisions>(
        `${ROUTES.feed(ws, pid)}/${enc(entry)}/decisions`,
      );
      return {
        locale: body.locale,
        bulkApproved: body.bulk_approved ?? 0,
        decisions: (body.decisions ?? []).map((d) => ({
          card: cardFromServer(d.card),
          decision: d.decision,
          at: d.at,
          current: d.current,
          canUndo: d.can_undo,
        })),
      };
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
  reason_code?: string;
  copies?: {
    block_id: string;
    item_name: string;
    key?: string;
    collection_id?: string;
    source_revision?: string;
    target_revision: string;
  }[];
}

/** A page of the server's deck. */
export interface ServerDeck {
  locale?: string;
  cards?: ServerDeckCard[];
  counts?: DeckCounts;
  next_cursor?: string;
}

/** A card from the server's deck. */
/** The decisions of a feed roll-up as the server sends them. */
interface ServerFeedDecisions {
  locale: string;
  bulk_approved?: number;
  decisions?: {
    card: ServerDeckCard;
    decision: string;
    at: string;
    current: boolean;
    can_undo: boolean;
  }[];
}

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
    reasonCode: c.reason_code || undefined,
    preReview: c.pre_review,
    notes: c.notes ?? [],
    findingsCount: c.findings_count ?? 0,
    failingFindingsCount: c.failing_findings_count ?? 0,
    ownWork: c.own_work ?? false,
    copies: (c.copies ?? []).map((cp) => ({
      blockId: cp.block_id,
      item: cp.item_name,
      name: cp.key,
      collectionId: cp.collection_id ?? "",
      targetRevision: cp.target_revision,
      sourceRevision: cp.source_revision || undefined,
    })),
  };
}

interface ServerContainer {
  kind: ReviewContainer["kind"];
  title: string;
  item_name: string;
  locale: string;
  focus: string;
  more_before?: boolean;
  more_after?: boolean;
  next_waiting?: string;
  collection_id?: string;
  preview?: { kind: string; url: string; source_file: string };
  units?: {
    block_id: string;
    item_name: string;
    key: string;
    role: string;
    level?: number;
    source_runs?: Run[];
    target_runs?: Run[];
    target_status?: string;
    waiting?: boolean;
    source_revision: string;
    target_revision: string;
    message_key?: string;
    message_text?: string;
    overlays?: OverlayView[] | null;
  }[];
}

interface ServerConcept {
  id: string;
  domain?: string;
  definition?: string;
  do_not_translate?: boolean;
  terms?: {
    text: string;
    locale: string;
    status?: string;
    part_of_speech?: string;
    note?: string;
  }[];
}

export function conceptFromServer(c: ServerConcept): Concept {
  return {
    id: c.id,
    domain: c.domain || undefined,
    definition: c.definition || undefined,
    doNotTranslate: c.do_not_translate ?? false,
    terms: (c.terms ?? []).map((t) => ({
      text: t.text,
      locale: t.locale,
      status: t.status || "approved",
      partOfSpeech: t.part_of_speech || undefined,
      note: t.note || undefined,
    })),
  };
}

export function containerFromServer(c: ServerContainer): ReviewContainer {
  return {
    kind: c.kind,
    title: c.title,
    item: c.item_name,
    locale: c.locale,
    focus: c.focus,
    moreBefore: c.more_before ?? false,
    moreAfter: c.more_after ?? false,
    nextWaiting: c.next_waiting || undefined,
    collectionId: c.collection_id || undefined,
    preview: c.preview
      ? { kind: c.preview.kind, url: c.preview.url, sourceFile: c.preview.source_file }
      : undefined,
    units: (c.units ?? []).map((u) => ({
      blockId: u.block_id,
      item: u.item_name,
      key: u.key,
      role: u.role,
      level: u.level || undefined,
      source: u.source_runs ?? [],
      target: u.target_runs ?? [],
      targetStatus: u.target_status || undefined,
      waiting: u.waiting ?? false,
      sourceRevision: u.source_revision,
      targetRevision: u.target_revision,
      messageKey: u.message_key || undefined,
      runtimeText: u.message_text || undefined,
      overlays: u.overlays?.length ? u.overlays : undefined,
    })),
  };
}

interface ServerDeckPages {
  locale?: string;
  pages?: {
    kind: DeckPage["kind"];
    title: string;
    item_name: string;
    collection_id?: string;
    component?: string;
    waiting: number;
    findings?: number;
    first_block_id: string;
  }[];
  waiting?: number;
  more?: boolean;
}

export function deckPagesFromServer(body: ServerDeckPages, locale: string): DeckPages {
  const pages = (body.pages ?? []).map((p) => ({
    kind: p.kind,
    title: p.title,
    item: p.item_name,
    collectionId: p.collection_id ?? "",
    component: p.component || undefined,
    waiting: p.waiting,
    findings: p.findings ?? 0,
    firstBlockId: p.first_block_id,
  }));
  return {
    locale: body.locale ?? locale,
    pages,
    waiting: body.waiting ?? pages.reduce((n, p) => n + p.waiting, 0),
    more: body.more ?? false,
  };
}

/**
 * A container unit as a deck card, so a decision made in place goes through
 * the same change sets and queue as one made on a card.
 */
export function cardOfUnit(u: ContainerUnit, locale: string, collectionId = ""): DeckCard {
  return {
    key: `${u.blockId}:${locale}`,
    blockId: u.blockId,
    item: u.item,
    locale,
    collectionId,
    name: u.key,
    source: u.source,
    target: u.target,
    targetRevision: u.targetRevision,
    sourceRevision: u.sourceRevision || undefined,
    group: "unsorted",
    notes: [],
    findingsCount: 0,
    failingFindingsCount: 0,
    ownWork: false,
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

/**
 * A context card as the server sends it, with the candidate fields filled in
 * when a server leaves them out: not proposed, and a reach with no
 * collections listed.
 */
function contextCard(card: ContextCard): ContextCard {
  const out: ContextCard = { ...card, proposed: card.proposed ?? false };
  if (card.reach) out.reach = { ...card.reach, collections: card.reach.collections ?? [] };
  return out;
}

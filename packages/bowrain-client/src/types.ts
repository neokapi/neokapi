/**
 * The shapes the Bowrain server answers with, as the client reads them. Each
 * mirrors a Go response type in bowrain/server, named in its comment; a field
 * the server leaves out is optional here.
 */

import type { ReviewContext } from "@neokapi/contract-types";
import type { Run } from "@neokapi/kapi-format";

/** The access and refresh tokens the server issues (venueauth.TokenResponse). */
export interface AuthTokens {
  access_token: string;
  refresh_token?: string;
  token_type?: string;
  /** Seconds until the access token expires. */
  expires_in?: number;
}

/** The signed-in person (auth.User, GET /auth/me). */
export interface User {
  id: string;
  email: string;
  name?: string;
}

/** A workspace the person belongs to (auth.Workspace). */
export interface Workspace {
  id: string;
  name: string;
  slug: string;
  role?: string;
}

/** A project in a workspace (ProjectInfoResponse, summary view). */
export interface Project {
  id: string;
  name: string;
  default_source_language: string;
  target_languages: string[];
  default_stream?: string;
  block_count?: number;
}

/**
 * What the caller may do on one project (CallerPermissionsResponse). An empty
 * `languages` list means every language.
 */
export interface CallerPermissions {
  permissions: string[];
  languages: string[];
}

/** One locale's committed target (BlockTargetInfo). */
export interface BlockTarget {
  text: string;
  status?: string;
}

/** A block as the blocks and pending-review routes serve it (BlockInfoResponse). */
export interface BlockInfo {
  id: string;
  source_id?: string;
  name?: string;
  source: string;
  source_runs?: Run[];
  targets: Record<string, BlockTarget>;
  targets_runs?: Record<string, Run[]>;
  properties?: Record<string, string>;
  target_revisions?: Record<string, string>;
  /** The revision of the source edition, once the server serves it. */
  source_revision?: string;
}

/** An agent's pre-review of a translation (preReviewView). */
export interface PreReview {
  score: number;
  reviewer: string;
  reasons?: string[];
}

/** One pending-review entry (pendingReviewEntry). */
export interface PendingReviewEntry {
  block_id: string;
  item_name: string;
  locale: string;
  block?: BlockInfo;
  collection_id: string;
  term_compliance?: string;
  voice_score?: number;
  voice_bar?: number;
  pre_review?: PreReview;
}

/** A page of the pending-review queue (pendingReviewResponse). */
export interface PendingReviewPage {
  entries: PendingReviewEntry[];
  total: number;
  limit: number;
  offset: number;
}

/**
 * The group the server places a card in. `fast` cards suit a swipe, `closer`
 * cards need a slower read, and `desk` cards are reviewed on a desktop. A deck
 * read from the plain pending-review queue has no grouping, so every card is
 * `unsorted` and keeps the server's order.
 */
export type DeckGroup = "fast" | "closer" | "desk" | "unsorted";

/** One card of the review deck: one block's translation in one language. */
export interface DeckCard {
  /** Stable key: the block and the locale. */
  key: string;
  blockId: string;
  /** The item (file) the block is in: the `doc` a change set addresses. */
  item: string;
  locale: string;
  collectionId: string;
  /** The block's key in its item, such as `errors.network.timeout`. */
  name?: string;
  source: Run[];
  target: Run[];
  /** The translation's revision the card shows, sent back as `if_match`. */
  targetRevision: string;
  /** The source's revision the card shows, when the server serves it. */
  sourceRevision?: string;
  group: DeckGroup;
  /** Why the server placed the card in its group. */
  reason?: string;
  preReview?: PreReview;
  /** The term, voice, check and pre-review notes the server returns for the card. */
  notes: DeckNote[];
  findingsCount: number;
  failingFindingsCount: number;
  /**
   * True when the person wrote this translation. Under a `block`
   * separation-of-duties policy the server refuses their approval, so the
   * card offers no approve swipe.
   */
  ownWork: boolean;
}

/** A note on a card (`notes[]` of a deck card). */
export interface DeckNote {
  /** check, term, voice or pre_review; a kind a newer server adds reads as text. */
  kind: string;
  text: string;
}

/** How many cards each group holds. */
export interface DeckCounts {
  fast: number;
  closer: number;
  desk: number;
  total: number;
  /**
   * Translations the server has not classified yet. Their cards are absent
   * until a later request classifies them, so a client asks again while this
   * is above zero.
   */
  unclassified?: number;
}

/** A review deck for one language. */
export interface Deck {
  locale: string;
  cards: DeckCard[];
  total: number;
  counts?: DeckCounts;
  /** The cursor of the next page, absent on the last one. */
  nextCursor?: string;
  /** False when the deck was read from the pending-review queue. */
  ordered: boolean;
}

/** Where a ready-made fix comes from. */
export type FixKind = "term" | "memory" | "ai";

/** A ready-made fix for a translation, computed by the server. */
export interface FixCandidate {
  id: string;
  kind: FixKind;
  /** A short description of the fix, such as the term rule it applies. */
  label?: string;
  /** What a fix sends as `set_content`. */
  runs: Run[];
  text?: string;
  /** The server's word diff of the draft against this fix. */
  diff?: FixDiff;
  /** The reasons this fix addresses. */
  reasons: string[];
  /** A score from 0 to 100, for a content-memory match. */
  score?: number;
}

/** One span of the server's word diff. */
export interface FixDiffSpan {
  text: string;
  op: "same" | "cosmetic" | "removed" | "added";
}

/** The server's word diff: the draft's spans and the fix's spans. */
export interface FixDiff {
  kind?: string;
  prior: FixDiffSpan[];
  current: FixDiffSpan[];
}

/** The fix candidates for one card (GET .../fix-candidates). */
export interface FixCandidates {
  candidates: FixCandidate[];
  /** Candidates the server computed and withheld, with why. */
  dropped: { kind: string; label?: string; why: string }[];
  /** Why a kind of fix is missing, such as no AI provider being configured. */
  notes: string[];
  targetRevision?: string;
}

/** The reasons a reviewer can give for a fix or a rejection. */
export const FIX_REASONS = ["wrong_term", "meaning", "tone", "placeholder", "grammar"] as const;
export type FixReason = (typeof FIX_REASONS)[number];

/** The project feed's kinds (bstore.FeedKinds): a closed list. */
export const FEED_KINDS = [
  "push",
  "convergence_run",
  "review_summary",
  "recipe_change",
  "context_change",
  "change_set",
  "coverage_milestone",
] as const;
export type FeedKind = (typeof FEED_KINDS)[number];

/** Who did what a feed entry records: a person, an agent or a tool. */
export type FeedActorKind = "person" | "agent" | "tool";

/** The client screens a feed entry can open (FeedScreen*): a closed list. */
export const FEED_SCREENS = [
  "stream",
  "run",
  "review_deck",
  "recipe_diff",
  "context_deck",
  "change_set",
  "language",
] as const;
export type FeedScreen = (typeof FEED_SCREENS)[number];

/** The actor of a feed entry, as recorded (FeedActor). */
export interface FeedActor {
  id: string;
  kind: FeedActorKind;
  name?: string;
  session?: string;
  host?: string;
}

/** The member an agent or a tool acted for (FeedMember). */
export interface FeedMember {
  id: string;
  name?: string;
}

/** The screen an entry opens and what to open there (FeedLink). */
export interface FeedLink {
  screen: FeedScreen;
  /** The push, run or change set; the entry itself for a recipe diff. */
  id?: string;
  stream?: string;
  locale?: string;
  /** The context deck's mode. */
  mode?: "pending" | "replay";
  items?: string[];
}

/** One collection a recipe change touched (event.CollectionChange). */
export interface FeedCollectionChange {
  name: string;
  change: "added" | "removed" | "coordinates";
  before?: Record<string, string>;
  after?: Record<string, string>;
}

/** A recipe change's values before and after (event.RecipeChange). */
export interface FeedRecipeDiff {
  collections?: FeedCollectionChange[];
  settings?: { name: string; before: string; after: string }[];
  targets?: { before: string[]; after: string[] };
}

/**
 * One entry of a project's feed (FeedEntry). It carries counts and names,
 * never a sentence; the client renders each in the reader's language.
 */
export interface FeedEntry {
  id: string;
  /** A kind from FEED_KINDS; a kind a newer server adds reads as another string. */
  kind: FeedKind | (string & {});
  /** RFC 3339, with microseconds. */
  at: string;
  actor: FeedActor;
  on_behalf_of?: FeedMember;
  locales?: string[];
  coordinates?: Record<string, string>;
  /** Typed counts, keyed by what they count, such as `files` and `blocks` for a push. */
  counts: Record<string, number>;
  stream?: string;
  /** A run's outcome or a change set's step. */
  state?: string;
  diff?: FeedRecipeDiff;
  /** The first few items a context change touched; `counts` holds the full number. */
  samples?: FeedSample[];
  /** A run's result per language. */
  standing?: FeedStanding[];
  /** A change set's name. */
  name?: string;
  link: FeedLink;
}

/** One item a context change touched (event.ContextSample). */
export interface FeedSample {
  /** term_rule, note or voice_rule. */
  kind: string;
  operation?: string;
  term?: string;
  replacement?: string;
  text?: string;
}

/** One language's standing at the end of a run (FeedStanding). */
export interface FeedStanding {
  locale: string;
  /** shippable, parked or pending. */
  state: string;
  units?: number;
  produced?: number;
}

/** A page of a project's feed (FeedResponse), newest first. */
export interface FeedPage {
  entries: FeedEntry[];
  /** Absent on the last page. */
  next_cursor?: string;
}

/** The feed's filters (the query parameters of GET /:ws/:id/feed). */
export interface FeedQuery {
  kinds?: FeedKind[];
  locale?: string;
  /** Axis values the entries sit at, such as `{ product: "kapi" }`. */
  coordinates?: Record<string, string>;
  actor?: string;
  actorKind?: FeedActorKind;
  /** RFC 3339. */
  since?: string;
  cursor?: string;
  limit?: number;
}

export type { ReviewContext };

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
  kind: "check" | "term" | "voice" | "pre_review" | string;
  text: string;
}

/** How many cards each group holds. */
export interface DeckCounts {
  fast: number;
  closer: number;
  desk: number;
  total: number;
  /** True when the server classified only part of a long queue. */
  partial?: boolean;
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

export type { ReviewContext };

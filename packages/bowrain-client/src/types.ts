/**
 * The shapes the Bowrain server answers with, as the client reads them. Each
 * mirrors a Go response type in bowrain/server, named in its comment; a field
 * the server leaves out is optional here.
 */

import type { OverlayView, ReviewContext } from "@neokapi/contract-types";
import type { Run } from "@neokapi/kapi-format";

/** The access and refresh tokens the server issues (venueauth.TokenResponse). */
export interface AuthTokens {
  access_token: string;
  refresh_token?: string;
  token_type?: string;
  /** Seconds until the access token expires. */
  expires_in?: number;
}

/** The signed-in person (meResponse, GET and PATCH /auth/me). */
export interface User {
  id: string;
  email: string;
  /** The name the person set; absent until they set one. */
  name?: string;
  /** The name to show: the name they set, else their handle, else the email before the @. */
  shown_name?: string;
  /** The slug of their personal workspace. */
  handle?: string;
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

/**
 * One language waiting for the caller's review (reviewInboxLanguage). The
 * counts are the language's review deck: `waiting` is everything in it,
 * `unclassified` the part the deck groups when it is opened.
 */
export interface ReviewInboxLanguage {
  workspace: string;
  workspace_name: string;
  project: string;
  project_name: string;
  stream: string;
  locale: string;
  waiting: number;
  fast: number;
  closer: number;
  desk: number;
  unclassified: number;
}

/**
 * One project whose context waits for the caller's review
 * (contextInboxEntry). `by_kind` counts the cards of each kind the caller
 * may keep.
 */
export interface ContextInboxEntry {
  workspace: string;
  workspace_name: string;
  project: string;
  project_name: string;
  waiting: number;
  by_kind: Partial<Record<ContextCardKind, number>>;
}

/** The review inbox: the context waiting first, then the languages. */
export interface ReviewHome {
  context: ContextInboxEntry[];
  languages: ReviewInboxLanguage[];
}

/** The kinds of card the context deck holds, in the order it deals them. */
export type ContextCardKind = "voice_rule" | "concept" | "term_rule" | "note";

/** What a context card offers. */
export type ContextAction = "keep" | "reject" | "later";

/** The ground a keep is valid on, decided by the server. */
export type ContextReviewBasis = "peer" | "agent_authored" | "solo_owner" | "self_warned";

/** The item a context card names, in whichever store holds it. */
export interface ContextItemRef {
  kind: "op" | "concept" | "voice_rule";
  id: string;
  profile?: string;
}

/** Who wrote a context item, as its store recorded it. */
export interface ContextAuthor {
  kind: "person" | "agent" | "tool";
  name?: string;
  session?: string;
  /** The agent host a person recorded the item from. */
  host?: string;
  agent_authored: boolean;
}

/** An earlier attestation of an older revision of the item. */
export interface ContextReview {
  id: string;
  by: { kind: string; name?: string };
  at: string;
  revision: string;
  basis: ContextReviewBasis;
}

/**
 * The bulk operation that brought a context item in or last changed it: an
 * import of a context file, or a whole voice-profile write
 * (contextDeckOrigin). The deck deals the items it carried, never the
 * operation itself.
 */
export interface ContextOrigin {
  id: string;
  short: string;
  kind: "import" | "voice_edit";
  /** What an import read. */
  source?: "terms" | "memory" | "voice" | "decisions";
  /** The file an import read. */
  path?: string;
  /** The voice profile a voice edit wrote. */
  profile?: string;
  /** Items an import carried, or fields a voice edit changed. */
  count?: number;
  author: ContextAuthor;
  at: string;
  note?: string;
}

/** One place a voice rule matches the project's source text. */
export interface ContextRuleExample {
  before: string;
  match: string;
  after: string;
  /** The file the text sits in. */
  item?: string;
}

/** Where a voice rule matches the project's source text (a bounded walk). */
export interface ContextRuleExamples {
  examples: ContextRuleExample[];
  scanned: number;
  /** The walk read every block. False with no examples means it stopped early. */
  complete: boolean;
}

/** One context item waiting for review (contextDeckCard). */
export interface ContextCard {
  item: ContextItemRef;
  kind: ContextCardKind;
  revision: string;
  status?: "suggested" | "established" | "contested";
  term?: { term: string; replacement?: string; forms?: string[] };
  terms?: { text: string; locale: string; status?: string }[];
  pattern?: {
    list: "prohibited" | "required";
    regex: string;
    description?: string;
    rate?: string;
    profile?: string;
    /** The words and phrases the regex matches, when it is a short list of them. */
    words?: string[];
  };
  text?: string;
  language?: string;
  flags?: string[];
  where: {
    level: "project" | "workspace";
    coordinates?: Record<string, string>;
    valid_from?: string;
    valid_to?: string;
  };
  author: ContextAuthor;
  at?: string;
  review?: ContextReview;
  own_work: boolean;
  basis?: ContextReviewBasis;
  actions: ContextAction[];
  /** The bulk operation that brought the item in, when one did. */
  origin?: ContextOrigin;
  /** An attestation covers the item at its current revision (origin listings only). */
  reviewed?: boolean;
  /**
   * A candidate term: a concept whose source-language term is proposed. Keep
   * makes it a term of the project; reject deletes it, and no later scan
   * proposes it again. The deck deals candidates first, widest reach first.
   */
  proposed?: boolean;
  /** How many blocks use a candidate term, in all and per collection. */
  reach?: ContextReach;
}

/** The blocks that use a candidate term, in all and per collection, most first. */
export interface ContextReach {
  blocks: number;
  collections: { name: string; blocks: number }[];
}

/** A page of the items one bulk operation brought in (GET .../context/deck/origins/:op). */
export interface ContextOriginPage {
  origin: ContextOrigin;
  /** Items of the project from the operation, and those still waiting for review. */
  total: number;
  pending: number;
  cards: ContextCard[];
  next_cursor?: string;
}

/** A page of the context deck (GET /:ws/:id/context/deck). */
export interface ContextDeck {
  mode: "pending";
  /** False for a project no push has bound: nothing can be kept yet. */
  bound: boolean;
  counts: {
    total: number;
    by_kind: Record<ContextCardKind, number>;
    /** Candidate terms among the concepts. */
    proposed: number;
    agent: number;
    person: number;
  };
  cards: ContextCard[];
  next_cursor?: string;
}

/** One decision about a context card. */
export interface ContextDecision {
  item: ContextItemRef;
  revision: string;
  action: ContextAction;
  note?: string;
}

/** What a context decision did. */
export interface ContextDecisionResult {
  outcome: "attested" | "kept" | "dropped" | "proposed" | "later";
  changeset_id?: string;
}

/**
 * A language's review deck narrowed to one collection (deckCollection). The
 * counts are those of ReviewInboxLanguage; `translations` is every
 * translation of the language in the collection and `decided` the part no
 * longer waiting, so `decided` of `translations` is the progress. An empty
 * `collection_id` is the blocks in no collection.
 */
export interface DeckCollection {
  collection_id: string;
  name: string;
  /** The channel the collection's recipe binds, such as `neokapi/desktop`. */
  channel?: string;
  /** The point the collection declares: axis to value, such as `product: kapi`. */
  coordinates?: Record<string, string>;
  /** The collection's review priority; absent for no collection and for the whole language. */
  priority?: "high" | "normal" | "low";
  waiting: number;
  fast: number;
  closer: number;
  desk: number;
  unclassified: number;
  translations: number;
  decided: number;
}

/**
 * A language's review deck split by collection, in the server's order:
 * collections with anything waiting first, finished ones last. `all` is the
 * whole language.
 */
export interface DeckCollections {
  locale: string;
  stream: string;
  all: DeckCollection;
  collections: DeckCollection[];
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
  /**
   * The reason as a code a client words itself: rejected, failing_findings,
   * stale_source, redraft, long_source, findings, no_pre_review, low_score or
   * fast. A code a newer server adds falls back to `reason`.
   */
  reasonCode?: string;
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
  /**
   * The other translations waiting with the same source and the same
   * translation. A decision on the card is sent for each of them in the same
   * change set. A card read before the server dealt copies has none.
   */
  copies?: DeckCopy[];
}

/** A translation a deck card stands for (`copies[]` of a deck card). */
export interface DeckCopy {
  blockId: string;
  item: string;
  /** The block's key in its item. */
  name?: string;
  collectionId: string;
  targetRevision: string;
  sourceRevision?: string;
}

/** A note on a card (`notes[]` of a deck card). */
export interface DeckNote {
  /** check, term, voice or pre_review; a kind a newer server adds reads as text. */
  kind: string;
  text: string;
}

/** How many cards each group holds. */
export interface DeckCounts {
  /**
   * Every translation the deck deals, grouped or not. The review inbox and
   * the deck's collections count the same way, so a language shows one
   * number on every screen.
   */
  waiting: number;
  fast: number;
  closer: number;
  desk: number;
  /**
   * Translations the server has not classified yet. Their cards are absent
   * until a later request classifies them, so a client asks again while this
   * is above zero.
   */
  unclassified: number;
  /** The classified part of `waiting`: the cards the deck can deal now. */
  total: number;
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
  /** Set on the entries the signed-in reader made. */
  viewer?: boolean;
}

/** The member an agent or a tool acted for (FeedMember). */
export interface FeedMember {
  id: string;
  name?: string;
  /** Set when the member is the signed-in reader. */
  viewer?: boolean;
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
  /**
   * Why a parked run stopped: no_progress (the rest waits for review),
   * checks_failing, source_not_ready, needs_credits, needs_ai_key or
   * rate_limited.
   */
  stall_reason?: string;
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

/** One review decision a feed entry counts, beside the translation as it stands now. */
export interface FeedDecision {
  /** The translation now, in the deck's shape: a withdrawal names its revision. */
  card: DeckCard;
  /** approved, rejected, or unreviewed for a withdrawal. */
  decision: string;
  at: string;
  /** Still the newest decision on the translation, which stands where it left it. */
  current: boolean;
  /** The reader made it and it still holds, so it can be withdrawn. */
  canUndo: boolean;
}

/** The decisions of a review entry of the feed (GET /:ws/:id/feed/:entry/decisions). */
export interface FeedDecisions {
  locale: string;
  decisions: FeedDecision[];
  /** Approvals of a bulk pass, which records no decision per translation. */
  bulkApproved: number;
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

/**
 * What a reader meets a block inside: a React component's strings, a
 * document's section from its heading to the next, or the blocks around it.
 */
export interface ReviewContainer {
  kind: "component" | "section" | "window";
  /** The component, the section's heading, or the file. */
  title: string;
  item: string;
  locale: string;
  /** The block the container was asked for. */
  focus: string;
  units: ContainerUnit[];
  /** Blocks of the container past what the response holds. */
  moreBefore: boolean;
  moreAfter: boolean;
  /**
   * The first translation waiting in the item after the container: where a
   * reviewer walking a document goes next. Absent when none waits.
   */
  nextWaiting?: string;
  /** The collection of the container's item. */
  collectionId?: string;
  /** Where a component's container runs, when its collection declares a host. */
  preview?: ContainerPreview;
}

/** A preview host a component can be seen running in. */
export interface ContainerPreview {
  /** The host kind, such as `storybook`. */
  kind: string;
  url: string;
  /** The component's source path, which names the story that renders it. */
  sourceFile: string;
}

/** One story, as a Storybook's `index.json` describes it. */
export interface StoryEntry {
  id: string;
  title: string;
  name: string;
  importPath: string;
  type?: string;
}

/** The `index.json` a Storybook serves. */
export interface StoryIndex {
  entries: Record<string, StoryEntry>;
}

/**
 * A part of a language's review deck a reviewer reads whole: one component's
 * strings, or the blocks of one document that belong to no component.
 */
export interface DeckPage {
  kind: "component" | "document";
  /** The component, or the document's file name. */
  title: string;
  item: string;
  collectionId: string;
  component?: string;
  waiting: number;
  /** The page's first waiting translation, which opens its container. */
  firstBlockId: string;
}

/** A language's review deck by page. */
export interface DeckPages {
  locale: string;
  pages: DeckPage[];
  /** The sum of the pages' waiting counts. */
  waiting: number;
  /** Pages past the ones returned. */
  more: boolean;
}

/** One block of a container, laid out by its role. */
export interface ContainerUnit {
  blockId: string;
  item: string;
  key: string;
  /** heading, paragraph, list_item, button, label, link, caption, cell, code or text. */
  role: string;
  /** A heading's level, 1 to 6. */
  level?: number;
  source: Run[];
  target: Run[];
  targetStatus?: string;
  /** A translation waiting for review. */
  waiting: boolean;
  sourceRevision: string;
  targetRevision: string;
  /** The key a running component looks the string up by. */
  messageKey?: string;
  /**
   * The translation as the runtime's catalog holds it, codes in their native
   * syntax. It goes to a running component and is never drawn.
   */
  runtimeText?: string;
  /**
   * The term check's spans over the source and the translation, run-anchored
   * (unitTerms reads them). A client draws them and matches no text itself.
   */
  overlays?: OverlayView[];
}

/** One term of a concept, in one language. */
export interface ConceptTerm {
  text: string;
  locale: string;
  status: string;
  partOfSpeech?: string;
  note?: string;
}

/** A concept in the workspace terms store: one meaning and its terms in each language. */
export interface Concept {
  id: string;
  domain?: string;
  definition?: string;
  doNotTranslate: boolean;
  terms: ConceptTerm[];
}

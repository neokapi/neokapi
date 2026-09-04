// The review model's five layers, as the shared cards read them.
//
// Two hosts assemble the model: `host.ReviewContext` for kapi desktop, the CLI
// and the MCP tools, and the platform's review-context response for its REST
// surfaces. They spell the same facts differently, so each surface maps its
// own type onto these views in a few lines and the cards draw one thing. A
// field a host does not carry is simply absent, and the card says so.

import type { FindingTone } from "../../lib/finding-severity";
import type { NeighbourhoodEntry } from "../ui/neighbourhood-table";

/** One constraint on wording bound at the point (profile.TermRule). */
export interface ReviewTermRuleView {
  term: string;
  replacement?: string;
  note?: string;
  severity?: string;
  do_not_translate?: boolean;
}

/** One term the unit's source matches, with the renderings the store mandates. */
export interface ReviewTermHitView {
  term: string;
  renderings?: string[];
  domain?: string;
}

/** The voice profile in force, with what it asks for and how the unit measures against it. */
export interface ReviewVoiceView {
  name: string;
  /** Where it was loaded from: a path, `pack:<name>`, or `store:<name>`. */
  source?: string;
  /** The profile rendered as prose, the same text the translate prompt carries. */
  guide?: string;
  /** The unit's latest voice score, where the host keeps one. */
  score?: number;
  /** The lowest score the profile accepts. */
  bar?: number;
}

/** One governance profile's validity window. */
export interface ReviewProfileWindowView {
  name: string;
  /** "active" | "upcoming" | "expired". */
  state: string;
  valid_from?: string;
  valid_to?: string;
}

/** Where the unit's file sits and what governs it there. */
export interface ReviewPointView {
  /** Profile and channel as the recipe writes the binding (`profile/channel`). */
  ref?: string;
  /** True when resolution fell through to the project's default point. */
  default?: boolean;
  /** The source file, project-relative. */
  path?: string;
  collection?: string;
  coordinates?: Record<string, string>;
  voice?: ReviewVoiceView;
  /** The rules bearing on this unit first. */
  termRules?: ReviewTermRuleView[];
  /** How many rules the point binds in all, so a capped list says what it is part of. */
  termsTotal?: number;
  /** The terms the source matches, where the host looks them up. Absent when it does not. */
  termHits?: ReviewTermHitView[];
  termHitsLoading?: boolean;
  profiles?: ReviewProfileWindowView[];
  notes?: string[];
}

/** The unit in its document: the blocks before and after it, in document order. */
export interface ReviewNeighbourhoodView {
  key?: string;
  /** Nearest last, so reading top to bottom reads the document. */
  before: NeighbourhoodEntry[];
  /** Nearest first. */
  after: NeighbourhoodEntry[];
}

/** The unit's previous source and the target approved for it. */
export interface ReviewPriorView {
  source: string;
  target: string;
  /** True when the context it was approved under still governs. */
  governed: boolean;
}

/** The content memory's best answer for this source, with its wording. */
export interface ReviewMatchView {
  /** Match percent, 0 to 100. */
  score: number;
  source?: string;
  target: string;
  /** How the memory matched it, where the host says: "exact", "fuzzy". */
  kind?: string;
}

/** What has already been approved for this unit. */
export interface ReviewHistoryView {
  prior?: ReviewPriorView;
  match?: ReviewMatchView;
  /** The committed content memory has never been read into this copy. */
  unseeded?: boolean;
}

/** One finding on the unit, from any checker, painted by its tone. */
export interface ReviewFindingView {
  /** A stable id for the row, used as its test hook. */
  id?: string;
  category?: string;
  /** The severity word the checker used, shown as given. */
  severity?: string;
  /** How hard it bites, on the shared scale. */
  tone: FindingTone;
  message: string;
  suggestion?: string;
  originalText?: string;
  /** Which side of the unit the finding is about; a source-side finding is marked. */
  field?: "source" | "target";
}

/** One AI pre-review remark. */
export interface ReviewAIRemarkView {
  severity?: string;
  message: string;
  suggestion?: string;
}

/** How the target under review was produced. */
export interface ReviewOriginView {
  /** human | memory | mt | ai | ocr | asr, or whatever the host records. */
  kind?: string;
  engine?: string;
  tool?: string;
  timestamp?: string;
}

/** The decision in force over the unit. */
export interface ReviewDecisionView {
  /** approved | rejected | signed-off. */
  state?: string;
  by?: string;
  at?: string;
  note?: string;
  /** The decision was recorded against source wording that has since changed. */
  sourceMoved?: boolean;
}

/** Where the target came from, and who decided on it. */
export interface ReviewProvenanceView {
  origin?: ReviewOriginView;
  decision?: ReviewDecisionView;
  /** A note on the unit outside the decision record. */
  note?: string;
}

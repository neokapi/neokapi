/** Voice types matching Go core/profile package */

import type { Anchor } from "@neokapi/contract-types";

export type { Anchor };

export interface ToneProfile {
  personality: string[];
  formality: "casual" | "neutral" | "formal" | "technical";
  emotion: "warm" | "neutral" | "authoritative";
  humor: "none" | "light" | "frequent";
  guidelines?: string;
}

export interface Pattern {
  regex: string;
  description: string;
  severity: "minor" | "major" | "critical";
}

export interface StyleRules {
  active_voice: boolean;
  sentence_length: "short" | "medium" | "varied";
  person_pov: "first_plural" | "second" | "third";
  contractions: "always" | "sometimes" | "never";
  prohibited_patterns?: Pattern[];
  required_patterns?: Pattern[];
}

export interface TermRule {
  term: string;
  replacement?: string;
  note?: string;
  severity?: "minor" | "major" | "critical";
}

export interface VocabularyRules {
  preferred_terms?: TermRule[];
  forbidden_terms?: TermRule[];
  competitor_terms?: TermRule[];
  abbreviations?: Record<string, string>;
}

export interface VoiceExample {
  before: string;
  after: string;
  explanation?: string;
  category?: "tone" | "style" | "vocabulary";
}

export interface LocaleOverride {
  formality?: string;
  humor?: string;
  person_pov?: string;
  cultural_notes?: string;
  vocabulary_overrides?: TermRule[];
  example_overrides?: VoiceExample[];
}

export interface ChannelOverride {
  tone?: ToneProfile;
  style?: StyleRules;
}

/**
 * PersonaOverride layers an individual author's voice on top of the brand
 * profile. Tone/style replace the resolved tone/style; the vocabulary deltas
 * are additive and bounded by the brand's guardrails — avoided_terms extend the
 * forbidden set, and a preferred_term the brand already forbids is dropped
 * rather than re-allowed. Mirror of Go core/profile.PersonaOverride.
 */
export interface PersonaOverride {
  tone?: ToneProfile;
  style?: StyleRules;
  preferred_terms?: TermRule[];
  avoided_terms?: TermRule[];
}

export interface VoiceProfile {
  id: string;
  name: string;
  description?: string;
  tone: ToneProfile;
  style: StyleRules;
  vocabulary: VocabularyRules;
  examples: VoiceExample[];
  locales?: Record<string, LocaleOverride>;
  channels?: Record<string, ChannelOverride>;
  personas?: Record<string, PersonaOverride>;
  workspace_id: string;
  /**
   * The minimum voice-compliance score (0–100) a block must reach to count as
   * compliant. Absent or 0 means the default bar (complianceBar / core/profile's
   * DefaultMinScore).
   */
  min_score?: number;
  version: number;
  created_at: string;
  updated_at: string;
  created_by?: string;
}

export type Dimension = "tone" | "style" | "vocabulary" | "clarity" | "compliance";

/**
 * A reviewer's in-place correction (original → corrected), fed to the
 * correction-learning loop via `POST /:ws/:id/voice/:ref/corrections`.
 * Repeated corrections surface as candidate rules and, past the profile's
 * auto-promote threshold, harden into an enforced check. `profile_id` is the
 * bound voice profile — resolved from `stream.properties.voice_profile_id`
 * (falling back to the workspace default).
 */
export interface VoiceCorrectionRequest {
  profile_id: string;
  block_id?: string;
  dimension: Dimension;
  original_text: string;
  corrected_text: string;
  finding_id?: string;
}

/** Server acknowledgement of a stored correction. */
export interface VoiceCorrectionResult {
  /** Whether the correction crossed the threshold and auto-promoted a rule. */
  auto_promoted?: boolean;
}

export type VoiceSeverity = "neutral" | "minor" | "major" | "critical";

/**
 * One voice finding, exactly as `core/check.Finding` serializes it — the shape
 * the profile check, the draft check and the stored brand scores all emit.
 *
 * Two fields used to be described wrongly, and both failed silently. The
 * grouping field is `category` on the wire, not `dimension`: the findings list
 * read `finding.dimension` and rendered an empty chip for every server-sourced
 * finding, and `ContextScanLiveTester` carried a normaliser to paper over it on
 * the one path it knew about. And `position` is a run-anchored
 * {@link Anchor}, not `{start, end}` character offsets — a consumer that
 * believed the declaration would have indexed into the wrong thing.
 *
 * `position` is optional because a finding need not locate anything: a checker
 * that judges the whole block leaves it at the zero range.
 */
export interface VoiceFinding {
  /**
   * Groups the finding. A voice check reports a {@link Dimension}; the field
   * is free-form on the Go side so a new checker adds a category without
   * touching the core, and it is typed as such here.
   */
  category: string;
  severity: VoiceSeverity;
  message: string;
  suggestion?: string;
  /** Run-anchored span over the checked runs. */
  position?: Anchor;
  original_text?: string;
  /**
   * The checker that produced this finding, stamped when it was recorded.
   * Several checkers accumulate into one annotation and only the last of them
   * is named on it, so a consumer holding findings alone — a flow run's report,
   * where the steps are whatever the recipe declared — reads this to attribute
   * each one. Optional: a finding recorded before the stamp existed carries
   * none, and a single-checker surface already knows the answer.
   */
  check?: string;
  /** Checker-specific detail: the matched rule id, a replacement, a concept id. */
  metadata?: Record<string, string>;
}

export interface DimensionScore {
  dimension: Dimension;
  score: number;
  penalty: number;
  issues: number;
}

export interface VoiceComplianceScore {
  overall: number;
  dimensions: DimensionScore[];
  findings: VoiceFinding[];
  word_count: number;
  profile_id: string;
}

export interface StoredScore {
  id: string;
  project_id: string;
  stream: string;
  block_id: string;
  profile_id: string;
  locale: string;
  score: number;
  dimensions: DimensionScore[];
  findings: VoiceFinding[];
  checked_at: string;
}

export interface ScoreTrend {
  date: string;
  avg_score: number;
  count: number;
}

export interface CreateVoiceProfileRequest {
  name: string;
  description?: string;
  tone: ToneProfile;
  style: StyleRules;
  vocabulary: VocabularyRules;
  examples: VoiceExample[];
  personas?: Record<string, PersonaOverride>;
  /**
   * The profile's own compliance bar (0–100); omitted or 0 means the default
   * (complianceBar / core/profile's DefaultMinScore). Authored content, not
   * server-managed metadata: the bar decides which blocks the ship gate and
   * bulk approve-passing count as compliant, so every write surface carries it.
   */
  min_score?: number;
}

export interface UpdateVoiceProfileRequest extends CreateVoiceProfileRequest {
  id: string;
}

// ── Correction-learning loop (AD-019) ──────────────────────────────────────

/** A vocabulary rule derived from repeated corrections. */
export interface SuggestedRule {
  term: string;
  replacement: string;
  correction_count: number;
  dimension: Dimension;
}

export type RuleDecisionStatus = "pending" | "approved" | "rejected" | "promoted";

/** A suggested rule paired with the team's decision about it. */
export interface CandidateRule extends SuggestedRule {
  status: RuleDecisionStatus;
  promoted_version?: number;
  auto?: boolean;
  decided_by?: string;
  decided_at?: string;
}

/** Per-collection slice of a blast radius. */
export interface CollectionBlastRadius {
  collection_id: string;
  collection_name: string;
  affected_blocks: number;
  avg_score_delta: number;
}

/** Impact of promoting a candidate rule across existing content. */
export interface BlastRadius {
  total_blocks: number;
  affected_blocks: number;
  improved_blocks: number;
  degraded_blocks: number;
  new_violations: number;
  resolved_violations: number;
  critical_count: number;
  collections: CollectionBlastRadius[];
}

/** Outcome of a voice compliance drift analysis. */
export interface DriftResult {
  drifted: boolean;
  recent_avg: number;
  baseline_avg: number;
  drop: number;
  recent_days: number;
  recent_count: number;
  reason?: string;
}

// ── Workspace voice compliance rollup ──────────────────────────────────────

/** Coarse score-trend direction; "" when a project has no history yet. */
export type VoiceTrend = "up" | "down" | "flat" | "";

/** One project's row in the workspace voice compliance rollup. */
export interface VoiceRollupEntry {
  project_id: string;
  project_name: string;
  /** Effective bound profile (resolution ladder); absent when nothing is bound. */
  profile_id?: string;
  profile_name?: string;
  /** Rounded mean of stored block scores; null when the project was never scored. */
  overall: number | null;
  dimensions?: DimensionScore[];
  trend: VoiceTrend;
  drift?: DriftResult;
  scored_blocks: number;
  /** ISO timestamp of the most recent check; null when never scored. */
  last_scored_at: string | null;
}

/** The workspace-wide voice compliance rollup, plus its pagination envelope. */
export interface VoiceRollup {
  projects: VoiceRollupEntry[];
  total: number;
  limit: number;
  offset: number;
}

/** Query options for the rollup: pagination + drift-window overrides. */
export interface VoiceRollupOptions {
  limit?: number;
  offset?: number;
  recentDays?: number;
  minScore?: number;
  dropPoints?: number;
}

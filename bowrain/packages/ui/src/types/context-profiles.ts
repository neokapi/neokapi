/**
 * Governance profiles: the points a workspace's content occupies in its context
 * space, mirroring the server's `GET /:ws/profiles` aggregation.
 *
 * A profile is derived, not stored. A push declares each collection's
 * coordinates and the voice they resolved to; grouping the collections by point
 * recovers the profiles. The recipe's own `profiles[]` regions never cross the
 * wire, so one broad profile governing two points reads here as two points.
 */

/** The voice governing a profile, with the size of what it enforces. */
export interface ContextProfileVoice {
  id: string;
  name: string;
  description?: string;
  version: number;
  updated_at: string;
  preferred_terms: number;
  forbidden_terms: number;
  competitor_terms: number;
  pattern_rules: number;
  locale_overrides: number;
  channel_overrides: number;
}

/** One collection sitting at a profile's point. */
export interface ContextProfileCollection {
  id: string;
  name: string;
  project_id: string;
  project_name: string;
  stream?: string;
  item_label: string;
  /** "recipe" when a kapi.yaml declares it, "workspace" when the hub created it. */
  owner: string;
}

/**
 * A profile's standing: what the stored voice checks say about the content
 * governed here.
 *
 * Scoped by the VOICE, not by the point — a stored score records the voice it
 * resolved through, and that is the narrowest coordinate the check itself
 * carries. Two points sharing one voice therefore share this number.
 */
export interface ContextProfileChecks {
  score: number;
  scored_blocks: number;
  findings: number;
  last_checked_at?: string;
}

/** One person holding a point. */
export interface ContextProfileCustodian {
  user_id: string;
  name?: string;
  email?: string;
  /** The region their membership names; empty when their authority is unbounded. */
  scope?: string;
  /** The membership granting the custody — a person can hold a point via several. */
  project_id: string;
}

/**
 * Who governs one point.
 *
 * `covered` asks whether anyone holds this point *specifically*. Blanket
 * authority does not make a point covered: the question is which content has
 * nobody who knows it, and an owner who can approve everything is the fallback
 * rather than the answer.
 *
 * This reports and never blocks. An ungoverned point is an org-chart gap, not a
 * content defect.
 */
export interface ContextProfileCustody {
  covered: boolean;
  custodians: ContextProfileCustodian[];
  /** People who can act here only because their authority is unbounded. */
  fallback?: ContextProfileCustodian[];
}

export interface ContextProfile {
  /** URL id: "default", `axis~value` pairs joined by ".", or `voice~<id>`. */
  slug: string;
  /**
   * The conventional name — the coordinate values joined by a hyphen in
   * alphabetical axis order, which is also the project directory the profile's
   * overrides live in. Empty for the default profile.
   */
  name: string;
  label: string;
  is_default: boolean;
  /** True once a push has placed content at this point. */
  declared: boolean;
  coordinates?: Record<string, string>;
  /** The point's value on the channel axis; it selects an override in the voice. */
  channel?: string;
  voice?: ContextProfileVoice;
  collections: ContextProfileCollection[];
  /** Change-sets in review carrying a voice rule for this profile's voice. */
  pending_changes: number;
  /** The stored checks that resolved through this profile's voice. Absent when
   *  nothing here has been checked. */
  checks?: ContextProfileChecks;
  /** Who governs this point. Absent for a voice bound to no point, which is not
   *  a place and therefore cannot be uncovered. */
  custody?: ContextProfileCustody;
}

/** The vocabulary every profile shares. */
export interface ContextProfileTerms {
  concept_count: number;
}

/** The workspace's most recent context scan. */
export interface ContextProfileScan {
  job_id: string;
  status: string;
  progress: number;
  updated_at: string;
}

export interface ContextProfilesResponse {
  profiles: ContextProfile[];
  /** Every axis the workspace's points name, sorted. */
  axes: string[];
  terms: ContextProfileTerms;
  scan?: ContextProfileScan;
  /** What a scan covers. "workspace": a scan job carries no coordinates. */
  scan_scope: string;
}

// The voice profile as the backend serves it.
//
// These mirror core/profile's JSON tags one-to-one rather than projecting a
// subset: a field the recipe can author and this surface cannot show is a rule
// governing the project invisibly, which is the defect the Voice page exists to
// close. A field added to the profile appears here and is rendered.

/** How the voice sounds. */
export interface ToneProfile {
  personality?: string[];
  formality?: string;
  emotion?: string;
  humor?: string;
  guidelines?: string;
}

/** How often a pattern may appear before it counts as a violation. */
export interface PatternRate {
  max: number;
  per_words?: number;
}

/** A regular expression the writing must avoid, or must contain. */
export interface Pattern {
  regex: string;
  description: string;
  severity: string;
  rate?: PatternRate;
  /** Where it applies: prose, code or heading. Empty means everywhere. */
  scope?: string;
}

/** How the voice is constructed. */
export interface StyleRules {
  active_voice?: boolean;
  sentence_length?: string;
  person_pov?: string;
  contractions?: string;
  prohibited_patterns?: Pattern[];
  required_patterns?: Pattern[];
}

/** One term, what to use instead, and how hard it bites. */
export interface TermRule {
  term: string;
  replacement?: string;
  note?: string;
  severity?: string;
  /** Ties the rule to a concept in the terms store. */
  concept_id?: string;
  do_not_translate?: boolean;
  forms?: string[];
  case_sensitive?: boolean;
  scope?: string;
}

/** The wording constraints, grouped by what the group means. */
export interface VocabularyRules {
  preferred_terms?: TermRule[];
  forbidden_terms?: TermRule[];
  competitor_terms?: TermRule[];
  abbreviations?: Record<string, string>;
}

/** A rewrite that shows the voice rather than describing it. */
export interface VoiceExample {
  before: string;
  after: string;
  explanation?: string;
  category?: string;
}

/** What changes for one language. */
export interface LocaleOverride {
  formality?: string;
  humor?: string;
  person_pov?: string;
  cultural_notes?: string;
  vocabulary_overrides?: TermRule[];
  example_overrides?: VoiceExample[];
}

/** What changes on one channel. */
export interface ChannelOverride {
  tone?: ToneProfile;
  style?: StyleRules;
}

/** What changes for one author voice, inside the profile's guardrails. */
export interface PersonaOverride {
  tone?: ToneProfile;
  style?: StyleRules;
  preferred_terms?: TermRule[];
  avoided_terms?: TermRule[];
}

/** How far a profile may promote its own suggested rules without review. */
export interface AutonomyConfig {
  auto_promote_at_count?: number;
}

/** A voice profile, as authored. */
export interface VoiceProfile {
  id?: string;
  name: string;
  description?: string;
  tone?: ToneProfile;
  style?: StyleRules;
  vocabulary?: VocabularyRules;
  examples?: VoiceExample[];
  locales?: Record<string, LocaleOverride>;
  channels?: Record<string, ChannelOverride>;
  personas?: Record<string, PersonaOverride>;
  autonomy?: AutonomyConfig;
  /** The compliance score a target must reach. */
  min_score?: number;
  version?: number;
  version_note?: string;
}

/** A declared validity window, and where it stands at the resolution instant. */
export interface VoiceValidity {
  from?: string;
  to?: string;
  /** "active", "upcoming" or "expired". */
  state: string;
}

/** A binding the instant excluded, and what governs in its place. */
export interface VoiceFallback {
  profile: string;
  expired: boolean;
  boundary?: string;
  /** Empty when the project's default point governs instead. */
  governing?: string;
  message: string;
}

/** The recipe binding that selected a profile. */
export interface VoiceBinding {
  /** "profile_file", "pack" or "profile". */
  kind: string;
  value: string;
}

/** The point a question resolved to. */
export interface VoicePointRef {
  path?: string;
  profile?: string;
  channel?: string;
  collection?: string;
  ref?: string;
  default: boolean;
}

/** One point in the project's context space, and the voice in force there. */
export interface VoicePoint {
  point: VoicePointRef;
  label: string;
  coordinates?: Record<string, string>;
  channels?: string[];
  collections: string[];
  /** The recipe key the governing binding was declared on. */
  field?: string;
  /** A path, `pack:<name>` or `store:<name>`. */
  source?: string;
  binding?: VoiceBinding;
  termstore?: string;
  profile?: VoiceProfile;
  guide?: string;
  validity?: VoiceValidity;
  fallback?: VoiceFallback;
  notes?: string[];
  edit: VoiceEditTarget;
}

/** Where a save at a point writes, and whether it may. */
export interface VoiceEditTarget {
  target?: string;
  writable: boolean;
  exists: boolean;
  /** True when the point reads a voice bound coarser than itself. */
  inherited: boolean;
  reason?: string;
}

/** A problem `kapi voice validate` reports. */
export interface ProfileProblem {
  field?: string;
  message: string;
  /** True when the problem is a note rather than a refusal. */
  warning?: boolean;
}

/** The result of a save. */
export interface VoiceSaveResult {
  saved: boolean;
  target?: string;
  changed: boolean;
  problems: ProfileProblem[];
  guide?: string;
}

/** The values a constrained field accepts, and what happens to one outside. */
export interface FieldValueSet {
  values: string[];
  /** True when a value outside the set is kept and rendered rather than refused. */
  open: boolean;
}

/** Every point the recipe declares, with its voice. */
export interface ProjectVoiceResult {
  /** The instant governance was resolved at. */
  at: string;
  points: VoicePoint[];
  notes?: string[];
}

/**
 * Whether a rule's severity fails a check or only reports it.
 *
 * `minor` and `neutral` warn; everything else fails, unset included — a rule
 * resolved from a terms store carries no severity and must not be silently
 * downgraded.
 */
export function severityFails(severity?: string): boolean {
  const s = (severity ?? "").toLowerCase();
  return s !== "minor" && s !== "neutral";
}

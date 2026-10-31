// The shapes the Map, the Reader and Decide render. The desktop backend and
// the Bowrain server each produce them; the views read nothing else.

import type { Anchor, Run } from "@neokapi/contract-types";

// --- The Reader (matches backend ReaderFileDTO, backend/reader.go) ---

/** One content file as the Reader draws it. */
export interface ReaderFile {
  /** Project-relative and slash-separated. */
  path: string;
  abs: string;
  format?: string;
  /** The file's source blocks in file order. */
  blocks: ReaderBlock[];
  /** Findings that name no block of the file. */
  document: ReaderFinding[];
  findings: number;
  fixable: number;
  verdict: "passed" | "failed" | "did_not_run";
  /** The context answer for the file, in the order `kapi context <path>` prints it. */
  rules: ReaderRule[];
  rules_total: number;
  attention: string[];
  /** The fixes the change service refused in the last fix, with its reasons. */
  refused: ReaderRefusal[];
}

/** One block of the file. */
export interface ReaderBlock {
  id: string;
  runs: Run[];
  findings: ReaderFinding[];
}

/** One finding, placed on its block. */
export interface ReaderFinding {
  /** Names the finding for FixFinding. */
  id: string;
  block_id?: string;
  position?: Anchor;
  span?: { start: number; end: number };
  /** name, word, tone, or the check that raised it. */
  kind: string;
  label: string;
  fails: boolean;
  suggested?: boolean;
  fixable: boolean;
  /** The translation a target-side finding is about. */
  locale?: string;
}

/** One rule that holds at the file. */
export interface ReaderRule {
  /** tone, name, word, keep or note. */
  kind: string;
  rule: string;
  status: string;
  /** Where the rule is held: terms, a pack, the workspace, the voice profile. */
  from?: string;
  /** Who recorded the rule, and when (RFC 3339), when the answer carries them. */
  by?: string;
  at?: string;
}

/** One fix the change service refused, with its reason. */
export interface ReaderRefusal {
  finding_id: string;
  reason: string;
}

// --- Context operations: who recorded a rule, where it was seen, the digest ---

/** Who performed a context operation. */
export interface ContextActor {
  /** "person", "agent" or "tool". */
  kind: string;
  /** The person's handle, the agent's client name, or the tool's name. */
  name?: string;
  /** The id one agent run's operations share. */
  session?: string;
  /** The machine the agent ran on, when its name carries one. */
  host?: string;
}

/** Where a subject was seen. */
export interface ContextEvidence {
  path?: string;
  unit?: string;
  quote?: string;
}

/** A term rule as the operation log holds it. */
export interface DigestTermRule {
  term?: string;
  replacement?: string;
  forms?: string[];
  advisory?: boolean;
}

/** What a digest item is about, as the operation log holds it. */
export interface DigestSubject {
  kind?: string;
  term?: DigestTermRule;
  memory?: { source: string; target: string; target_locale: string; source_locale?: string };
  text?: string;
}

/** How often the project's content writes a rule's forms. */
export interface DigestUsage {
  preferred: string;
  preferred_count: number;
  rejected: string[];
  rejected_count: number;
  within?: string;
  line: string;
}

/** One rule, suggestion or fact as the digest shows it. */
export interface DigestItem {
  id: string;
  short: string;
  status: string;
  /** "names", "words", "writing" or "memory". */
  theme: string;
  sentence: string;
  subject: DigestSubject;
  quote?: ContextEvidence;
  collection?: string;
  standing?: string;
  usage?: DigestUsage;
  noticed_by: { kind: string; name?: string; session?: string };
  at: string;
  /** Recorded (or established) after the person last looked. */
  new: boolean;
  scope: string;
  keepable: boolean;
  /** A suggestion, or a rule in force, a person can drop. */
  droppable: boolean;
  /** A rule in force a person can apply more widely. */
  widenable: boolean;
  /** The widenings open to a widenable rule: "workspace", and each axis of its point. */
  widen_to?: string[];
  established_at?: string;
  how?: string[];
}

/** A disagreement a person settles by choosing a side. */
export interface DigestConflict {
  sides: DigestItem[];
  by_evidence: boolean;
  reason: string;
}

// --- The Map and Decide (backend MapSummary, RulesDecideViewDTO) ---

/** One content file on the Map (MapSummary). */
export interface MapFileRow {
  /** Absolute, for the preview. */
  path: string;
  /** Project-relative and slash-separated, for the row. */
  relative: string;
  format?: string;
  collection?: string;
  /** The file's blocks in the block cache. */
  blocks: number;
  /** The file's findings in the last checks run over the whole project. */
  findings: number;
}

/** One item awaiting a decision on the Map. */
export interface MapSuggestionRow {
  /** "conflict" (the item is the disagreement's first side) or "suggested". */
  group: "conflict" | "suggested";
  item: DigestItem;
  conflict?: DigestConflict;
  /** Distinct blocks and files the item's evidence names. */
  blocks: number;
  files: number;
}

/** One answer to a Decide question (RuleOptionDTO): a form of a name, or one side of a disagreement. */
export interface RuleOption {
  /** One wording of a name. */
  form?: string;
  /** One side of a disagreement: its id, short id and sentence. */
  id?: string;
  short?: string;
  sentence?: string;
  /** Where the project's source writes it. */
  blocks: number;
  files: number;
  /** What keeping this option does. */
  consequence: string;
  /** Who suggested the side, for a disagreement. */
  noticed_by?: ContextActor;
}

/** One question Decide asks about a suggestion (RuleDecisionDTO), as `kapi rules` asks it. */
export interface RuleDecision {
  /** The suggestion's id. A disagreement has none; each option carries its side's. */
  id?: string;
  short?: string;
  kind: "name" | "word" | "note" | "memory" | "disagreement";
  question: string;
  /** What a disagreement is about. */
  reason?: string;
  sentence: string;
  /** The wording the question is about, for its row in the list. */
  label: string;
  /** How many wordings a word rule's check reports. */
  avoided: number;
  /** The forms of a name or the sides of a disagreement, the most used first. */
  options: RuleOption[];
  /** The option a keep takes unless the person picks another. */
  chosen: number;
  /** The question takes a form the person types (Other…). */
  other: boolean;
  /** What keeping it does, for a question with no options. */
  consequence?: string;
  /** The sentence the suggestion was seen in, and its file. */
  seen?: ContextEvidence;
  /** Where a kept rule holds. */
  applies: string;
  noticed_by: ContextActor;
}

/** Every question Decide asks about one project (RulesDecideViewDTO). */
export interface RulesDecideView {
  project: string;
  project_name: string;
  decisions: RuleDecision[];
  /** The counts `kapi rules list` opens with. */
  rules: number;
  suggested: number;
  narrowed: number;
}

/** Everything the Map renders, in one read (MapSummary). */
export interface MapSummary {
  project_key?: string;
  files: number;
  blocks: number;
  rules: number;
  /** Findings of the last whole-project checks run; null when none has run. */
  findings: number | null;
  checked_at?: string;
  /** Files the block cache holds blocks for, and when the store sync last found it current. */
  read_files: number;
  read_at?: string;
  suggested: number;
  conflicts: number;
  awaiting: number;
  target_languages: string[];
  file_rows: MapFileRow[];
  suggestions: MapSuggestionRow[];
}

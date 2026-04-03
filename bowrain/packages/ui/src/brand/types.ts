/** Brand voice types matching Go core/brand package */

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
  workspace_id: string;
  version: number;
  created_at: string;
  updated_at: string;
  created_by?: string;
}

export type Dimension = "tone" | "style" | "vocabulary" | "clarity" | "brand_compliance";

export type BrandSeverity = "neutral" | "minor" | "major" | "critical";

export interface BrandVoiceFinding {
  dimension: Dimension;
  severity: BrandSeverity;
  message: string;
  suggestion?: string;
  position: { start: number; end: number };
  original_text?: string;
}

export interface DimensionScore {
  dimension: Dimension;
  score: number;
  penalty: number;
  issues: number;
}

export interface BrandComplianceScore {
  overall: number;
  dimensions: DimensionScore[];
  findings: BrandVoiceFinding[];
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
  findings: BrandVoiceFinding[];
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
}

export interface UpdateVoiceProfileRequest extends CreateVoiceProfileRequest {
  id: string;
}

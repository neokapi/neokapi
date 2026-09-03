import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Pattern, TermRule, VoiceProfile } from "../../types/voice";
import { PatternRuleRow, RulesBlock, TermRuleRow } from "./rules";

const meta: Meta = {
  title: "Voice/Rules",
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj;

const termWithSeverity: TermRule = {
  term: "log in",
  replacement: "sign in",
  severity: "major",
  note: "One spelling across the product.",
  concept_id: "c-signin",
};

const plainTerm: TermRule = {
  term: "bulletproof",
};

const storeResolvedTerm: TermRule = {
  term: "utilise",
  replacement: "use",
  forms: ["utilize", "utilization"],
};

const pattern: Pattern = {
  regex: "\\b(?:synergy|leverage)\\b",
  description: "Corporate filler.",
  severity: "minor",
  rate: { max: 2, per_words: 1000 },
};

function Rows({ children }: { children: React.ReactNode }) {
  return <ul className="max-w-xl divide-y text-sm">{children}</ul>;
}

export const TermRuleWithSeverity: Story = {
  name: "Rule row / Term with severity",
  render: () => (
    <Rows>
      <TermRuleRow rule={termWithSeverity} />
    </Rows>
  ),
};

export const PlainTermRule: Story = {
  name: "Rule row / Plain term (store-resolved, no severity)",
  render: () => (
    <Rows>
      <TermRuleRow rule={plainTerm} />
      <TermRuleRow rule={storeResolvedTerm} />
    </Rows>
  ),
};

export const PatternRule: Story = {
  name: "Rule row / Pattern (regex behind the tooltip)",
  render: () => (
    <Rows>
      <PatternRuleRow pattern={pattern} />
    </Rows>
  ),
};

const populated: VoiceProfile = {
  name: "Northsea",
  vocabulary: {
    preferred_terms: [termWithSeverity],
    forbidden_terms: [plainTerm],
    abbreviations: { API: "application programming interface" },
  },
  style: {
    prohibited_patterns: [pattern],
    required_patterns: [
      { regex: "\\bplease\\b", description: "Ask, do not instruct.", severity: "neutral" },
    ],
  },
};

export const RulesList: Story = {
  name: "Rules list / Populated",
  render: () => (
    <div className="max-w-xl">
      <RulesBlock profile={populated} />
    </div>
  ),
};

export const RulesListEmpty: Story = {
  name: "Rules list / Empty",
  render: () => (
    <div className="max-w-xl">
      <RulesBlock profile={{ name: "Plain" }} />
    </div>
  ),
};

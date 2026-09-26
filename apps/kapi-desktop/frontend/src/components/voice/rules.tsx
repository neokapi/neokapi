// The one Rules list a voice profile is read through.
//
// A term rule and a pattern are different shapes in the model, but a reader
// meets them as the same thing: a statement of what to write, with the machine
// detail tucked away and the cost of breaking it marked. So both draw to one
// row shape here, and the say-this, never-say, always-write and never-write
// groups sit under a single section.

import { ArrowRight, Braces, Link2 } from "lucide-react";
import { Badge, SimpleTooltip } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";

import type { Pattern, StyleRules, TermRule, VoiceProfile } from "../../types/voice";

/**
 * An advisory rule as a neutral chip.
 *
 * Colour is reserved for a judgement a person makes or a finding a check
 * raised (see `packages/ui/docs/judgement-colours.md`); a rule stating what a
 * violation would cost is neither, so the chip stays neutral. A rule fails a
 * check unless it is advisory, so only an advisory rule draws a chip.
 */
export function AdvisoryChip({ advisory }: { advisory?: boolean }) {
  if (!advisory) return null;
  const fails = false;
  return (
    <SimpleTooltip
      content={fails ? t("A violation fails the check") : t("A violation is reported only")}
    >
      <Badge
        variant="outline"
        className="font-normal text-muted-foreground"
        data-testid="voice-advisory"
      >
        {t("reports only")}
      </Badge>
    </SimpleTooltip>
  );
}

/**
 * One pattern rule.
 *
 * The human description is the label; the regular expression it matches on is
 * tucked behind the `{}` affordance, so the row reads as a rule rather than as
 * code.
 */
export function PatternRuleRow({ pattern }: { pattern: Pattern }) {
  const allowance = pattern.rate
    ? pattern.rate.per_words
      ? t("up to {max} per {words} words", {
          max: pattern.rate.max,
          words: pattern.rate.per_words,
        })
      : t("up to {max}", { max: pattern.rate.max })
    : undefined;
  return (
    <li className="flex flex-wrap items-baseline gap-2 py-1.5" data-testid="voice-pattern">
      <span className="min-w-0 flex-1 text-sm text-foreground">
        {pattern.description || pattern.regex}
      </span>
      {pattern.description && (
        <SimpleTooltip content={pattern.regex}>
          <span
            className="cursor-help text-muted-foreground/70"
            data-testid="voice-pattern-regex"
            aria-label={t("Matching pattern")}
          >
            <Braces className="size-3.5" />
          </span>
        </SimpleTooltip>
      )}
      {pattern.scope && (
        <Badge variant="outline" className="font-normal text-muted-foreground">
          {pattern.scope}
        </Badge>
      )}
      {allowance && <span className="text-[11px] text-muted-foreground">{allowance}</span>}
      <AdvisoryChip advisory={pattern.advisory} />
    </li>
  );
}

/**
 * One term rule, drawn to the same shape as a pattern.
 *
 * The wording change is the label; how the rule matches (its other forms, case
 * sensitivity) is tucked behind the `{}` affordance, and a link marks a rule
 * backed by a concept in the terms store.
 */
export function TermRuleRow({ rule }: { rule: TermRule }) {
  const matching = [
    rule.forms?.length ? t("also {forms}", { forms: rule.forms.join(", ") }) : undefined,
    rule.case_sensitive ? t("case-sensitive") : undefined,
  ]
    .filter(Boolean)
    .join(" · ");
  return (
    <li className="flex flex-wrap items-baseline gap-2 py-1.5" data-testid="voice-term-rule">
      <span className="min-w-0 flex-1 text-sm">
        <span className="font-medium text-foreground">{rule.term}</span>
        {rule.replacement ? (
          <span className="text-muted-foreground">
            {" "}
            <ArrowRight className="inline size-3 align-[-1px]" /> {rule.replacement}
          </span>
        ) : (
          <span className="ml-1 text-[11px] text-muted-foreground">
            {t("no replacement, so tools skip it")}
          </span>
        )}
        {rule.note && <span className="ml-2 text-[11px] text-muted-foreground">{rule.note}</span>}
      </span>
      {matching && (
        <SimpleTooltip content={matching}>
          <span
            className="cursor-help text-muted-foreground/70"
            data-testid="voice-term-detail"
            aria-label={t("Matching detail")}
          >
            <Braces className="size-3.5" />
          </span>
        </SimpleTooltip>
      )}
      {rule.do_not_translate && (
        <Badge variant="outline" className="font-normal text-muted-foreground">
          {t("keep as written")}
        </Badge>
      )}
      {rule.scope && (
        <Badge variant="outline" className="font-normal text-muted-foreground">
          {rule.scope}
        </Badge>
      )}
      {rule.concept_id && (
        <SimpleTooltip content={t("Linked to a concept in your terms")}>
          <span
            className="text-muted-foreground/70"
            data-testid="voice-concept-link"
            aria-label={t("Linked concept")}
          >
            <Link2 className="size-3" />
          </span>
        </SimpleTooltip>
      )}
      <AdvisoryChip advisory={rule.advisory} />
    </li>
  );
}

/** A titled group of rule rows within the Rules section. */
export function RuleGroup({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div>
      <p className="mb-1 text-xs font-medium text-foreground">{title}</p>
      <ul className="divide-y text-sm">{children}</ul>
    </div>
  );
}

/** A term-rule group, or nothing when the group is empty. */
export function TermGroup({ title, rules }: { title: string; rules?: TermRule[] }) {
  if (!rules?.length) return null;
  return (
    <RuleGroup title={title}>
      {rules.map((r) => (
        <TermRuleRow key={`${r.term}:${r.replacement ?? ""}`} rule={r} />
      ))}
    </RuleGroup>
  );
}

/** The never-write and always-write pattern groups of a style, or nothing. */
export function PatternGroups({ style }: { style?: StyleRules }) {
  if (!style?.prohibited_patterns?.length && !style?.required_patterns?.length) return null;
  return (
    <>
      {!!style.prohibited_patterns?.length && (
        <RuleGroup title={t("Never write")}>
          {style.prohibited_patterns.map((p) => (
            <PatternRuleRow key={p.regex} pattern={p} />
          ))}
        </RuleGroup>
      )}
      {!!style.required_patterns?.length && (
        <RuleGroup title={t("Always write")}>
          {style.required_patterns.map((p) => (
            <PatternRuleRow key={p.regex} pattern={p} />
          ))}
        </RuleGroup>
      )}
    </>
  );
}

/**
 * The voice's pattern rules: the always-write and never-write patterns. Word
 * rules are terms and are shown with the project's terms.
 */
export function RulesBlock({ profile }: { profile: VoiceProfile }) {
  const style = profile.style;
  const hasRules = !!style?.prohibited_patterns?.length || !!style?.required_patterns?.length;
  return (
    <div className="space-y-3">
      <PatternGroups style={style} />
      {!hasRules && (
        <p className="text-sm text-muted-foreground">
          {t("This profile declares no pattern rule.")}
        </p>
      )}
    </div>
  );
}

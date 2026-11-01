// Kept by: the rules in force, each with who put it in force and when, the
// newest first. A row holds the rule's kind and the rule; its meta line names
// who kept it, the day, and where it holds. A list longer than GROUP_ABOVE
// rows is split under a heading per kind.

import { t } from "@neokapi/i18n-react/runtime";
import {
  EmptyState,
  ErrorNotice,
  LoadingSpinner,
  SectionHeading,
  When,
} from "@neokapi/ui-primitives";
import { NoticedBy } from "./NoticedBy";
import { KindMark, kindWord } from "./ReaderView";
import type { ContextActor, KeptRule } from "./types";

/** A list holding more rules than this is split under a heading per kind. */
export const GROUP_ABOVE = 12;

/** The order the kinds' groups take. A kind not named here follows them. */
const KIND_ORDER = ["name", "word", "tone", "keep", "note"];

/** The rules, the newest first. A rule with no time sorts last. */
function newestFirst(rules: readonly KeptRule[]): KeptRule[] {
  const at = (r: KeptRule) => Date.parse(r.kept_at) || 0;
  return [...rules].sort((a, b) => at(b) - at(a));
}

/** The rules under their kinds, in KIND_ORDER, each group the newest first. */
function byKind(rules: readonly KeptRule[]): [string, KeptRule[]][] {
  const groups = new Map<string, KeptRule[]>();
  for (const r of rules) groups.set(r.kind, [...(groups.get(r.kind) ?? []), r]);
  const rank = (k: string) => {
    const i = KIND_ORDER.indexOf(k);
    return i === -1 ? KIND_ORDER.length : i;
  };
  return [...groups].sort(([a], [b]) => rank(a) - rank(b) || a.localeCompare(b));
}

export interface KeptRulesViewProps {
  /** The rules, null while the first read is in flight. */
  rules: KeptRule[] | null;
  error?: unknown;
  /** The name a person is shown by. Absent, a person with no name is "you". */
  nameOf?: (who: ContextActor) => string | undefined;
}

/** The rules in force and who kept each one. */
export function KeptRulesView({ rules, error, nameOf }: KeptRulesViewProps) {
  return (
    <section
      data-slot="kept-rules"
      aria-label={t("Kept by", "the list of rules in force and who kept each")}
      className="space-y-2"
    >
      <SectionHeading count={rules?.length}>
        {t("Kept by", "the list of rules in force and who kept each")}
      </SectionHeading>
      {error ? (
        <ErrorNotice error={error} title={t("The kept rules could not be read")} />
      ) : !rules ? (
        <LoadingSpinner />
      ) : rules.length === 0 ? (
        <EmptyState title={t("No rule kept yet.")} />
      ) : rules.length > GROUP_ABOVE ? (
        byKind(newestFirst(rules)).map(([kind, group]) => (
          <div key={kind} data-slot="kept-group" data-kind={kind} className="pt-2">
            <h3 className="flex items-baseline justify-between text-[11px] font-semibold text-muted-foreground">
              <span>{kindWord(kind)}</span>
              <span className="tabular-nums">{group.length}</span>
            </h3>
            <KeptList rules={group} nameOf={nameOf} />
          </div>
        ))
      ) : (
        <KeptList rules={newestFirst(rules)} nameOf={nameOf} />
      )}
    </section>
  );
}

/** The rows of one list of rules, in the order given. */
function KeptList({
  rules,
  nameOf,
}: {
  rules: readonly KeptRule[];
  nameOf?: (who: ContextActor) => string | undefined;
}) {
  return (
    <ul className="divide-y divide-border">
      {rules.map((rule, i) => (
        <KeptRuleRow key={`${rule.kind}:${rule.rule}:${i}`} rule={rule} nameOf={nameOf} />
      ))}
    </ul>
  );
}

/** One rule: its kind and the rule, then who kept it, the day and where it holds. */
function KeptRuleRow({
  rule,
  nameOf,
}: {
  rule: KeptRule;
  nameOf?: (who: ContextActor) => string | undefined;
}) {
  return (
    <li data-slot="kept-rule" data-kind={rule.kind} className="flex items-start gap-3 py-2">
      <KindMark kind={rule.kind} />
      <div className="min-w-0 flex-1 space-y-0.5">
        <div className="text-sm">{rule.rule}</div>
        <div className="flex flex-wrap items-center gap-x-1.5 text-[11px] text-muted-foreground">
          <span data-slot="kept-by">
            <NoticedBy who={rule.kept_by} nameOf={nameOf} />
          </span>
          <span aria-hidden>·</span>
          <When iso={rule.kept_at} dateStyle="medium" />
          <span aria-hidden>·</span>
          <span data-slot="kept-applies">{rule.applies}</span>
        </div>
      </div>
    </li>
  );
}

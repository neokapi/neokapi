// Kept by: the rules in force, each with who put it in force and when. A row
// holds the rule's kind and the rule; its meta line names who kept it, the
// day, and where it holds.

import { t } from "@neokapi/i18n-react/runtime";
import {
  EmptyState,
  ErrorNotice,
  LoadingSpinner,
  SectionHeading,
  When,
} from "@neokapi/ui-primitives";
import { NoticedBy } from "./NoticedBy";
import { KindMark } from "./ReaderView";
import type { ContextActor, KeptRule } from "./types";

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
      ) : (
        <ul className="divide-y divide-border">
          {rules.map((rule, i) => (
            <KeptRuleRow key={`${rule.kind}:${rule.rule}:${i}`} rule={rule} nameOf={nameOf} />
          ))}
        </ul>
      )}
    </section>
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

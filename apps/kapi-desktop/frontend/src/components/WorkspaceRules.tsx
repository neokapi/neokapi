// The rules that hold in every project: each one a person applied to the
// whole workspace from a project's review. One row per rule, the sentence and
// then who decided it, where and when.

import { useQuery } from "@tanstack/react-query";
import { t } from "@neokapi/i18n-react/runtime";
import { ErrorNotice, SectionHeading } from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { useInvalidateOnEvent } from "../hooks/useInvalidateOnEvent";
import { RuleSentence } from "./ContextDigest";
import type { WorkspaceRule } from "../types/api";

export interface WorkspaceRulesProps {
  /** Pre-loaded rules for Storybook and tests, which reach no backend. */
  rules?: WorkspaceRule[];
}

export function WorkspaceRules({ rules: given }: WorkspaceRulesProps) {
  const query = useQuery({
    queryKey: qk.workspaceRules(),
    queryFn: () => api.workspaceRules(),
    enabled: !given,
  });
  // A rule applied more widely in any project, from this app or another
  // process, moves the workspace's log.
  useInvalidateOnEvent("workspace:changed", [qk.workspaceRules()]);

  const rules = given ?? query.data ?? null;

  return (
    <section data-slot="workspace-rules" aria-labelledby="workspace-rules-heading">
      <div id="workspace-rules-heading">
        <SectionHeading>Holds in every project</SectionHeading>
      </div>
      <div className="mt-2">
        {query.error && !given ? (
          <ErrorNotice error={query.error} title="The workspace's rules could not be read" />
        ) : rules === null ? null : rules.length === 0 ? (
          <p data-slot="workspace-rules-empty" className="text-sm text-muted-foreground">
            Nothing yet. A rule you apply more widely in a project&apos;s review holds here, in
            every project you work on.
          </p>
        ) : (
          <div className="divide-y divide-border/60">
            {rules.map((rule) => (
              <div key={rule.id} data-slot="workspace-rule" className="py-2">
                <p className="text-sm leading-snug">
                  <RuleSentence item={rule} />
                </p>
                <p
                  data-slot="workspace-rule-how"
                  className="mt-0.5 text-[11px] text-muted-foreground"
                >
                  {ruleMeta(rule)}
                </p>
              </div>
            ))}
          </div>
        )}
      </div>
    </section>
  );
}

/** "kept by you · decided in Acme Help Center · 12 September". */
export function ruleMeta(rule: WorkspaceRule): string {
  const parts = [...(rule.how ?? [])];
  parts.push(t("decided in {project}", { project: rule.project_name || rule.project_key }));
  const at = new Date(rule.at);
  if (!Number.isNaN(at.getTime())) {
    parts.push(at.toLocaleDateString(undefined, { day: "numeric", month: "long" }));
  }
  return parts.join(" · ");
}

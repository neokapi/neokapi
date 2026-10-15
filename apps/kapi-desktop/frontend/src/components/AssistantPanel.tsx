// What your assistant reads, in three parts: which assistants are connected
// to the project, the rules files kapi writes where an assistant reads them
// as it starts, and, for one file, the whole answer an assistant gets.
//
// A tab with no checkout has no project files for an assistant to start in,
// so it shows the first part's line saying so and leaves the other two out.

import { PageHeader, SectionHeading } from "@neokapi/ui-primitives";
import { AgentContextPane } from "./AgentContextPane";
import { AssistantHosts } from "./AssistantHosts";
import { RulesFilesView } from "./RulesFilesView";
import type { AgentHostStatus, RulesFileView } from "./contextTypes";

export interface AssistantPanelProps {
  tabID: string;
  /** Open the brief on this file rather than on the picker. */
  path?: string;
  /** A tab over a project's context alone, with no checkout here. */
  contextOnly?: boolean;
  /** Pre-loaded hosts for Storybook and tests, which reach no backend. */
  hosts?: AgentHostStatus[];
  /** Pre-loaded rules files for Storybook and tests. */
  rulesFiles?: RulesFileView[];
}

export function AssistantPanel({
  tabID,
  path,
  contextOnly,
  hosts,
  rulesFiles,
}: AssistantPanelProps) {
  return (
    <div data-slot="assistant-panel">
      <PageHeader title="What your assistant reads" />
      <div className="space-y-8">
        <section data-slot="assistant-connections">
          <SectionHeading className="mb-2">Connected assistants</SectionHeading>
          <AssistantHosts tabID={tabID} hosts={hosts} contextOnly={contextOnly} />
        </section>

        {!contextOnly && (
          <section data-slot="assistant-rules-files">
            <SectionHeading className="mb-2">Rules files</SectionHeading>
            <RulesFilesView tabID={tabID} rulesFiles={rulesFiles} />
          </section>
        )}

        {!contextOnly && (
          <section data-slot="assistant-brief">
            <SectionHeading className="mb-2">The brief for a file</SectionHeading>
            <p className="text-[11px] text-muted-foreground">
              For one file, the full answer an assistant gets, the same text{" "}
              <code translate="no">kapi context &lt;path&gt;</code> prints.
            </p>
            <div className="mt-3 h-[32rem] overflow-hidden rounded-md border border-border">
              <AgentContextPane tabID={tabID} path={path} />
            </div>
          </section>
        )}
      </div>
    </div>
  );
}

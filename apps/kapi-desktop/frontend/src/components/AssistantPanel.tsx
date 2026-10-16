// What your assistant reads: which assistants are connected to the project,
// then a reader with two tabs. Rules files shows the sections kapi writes
// where an assistant reads them as it starts; the brief for a file shows the
// whole answer an assistant gets for one file.
//
// The panel opens on the rules files, or on the brief when it is handed a
// path, as Ask and Checks do when they open it at a file. A tab with no
// checkout has no project files for an assistant to start in, so it shows the
// connections' line saying so and leaves the tabs out.

import { useState } from "react";
import {
  PageHeader,
  SectionHeading,
  SimpleTooltip,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@neokapi/ui-primitives";
import { AgentContextPane } from "./AgentContextPane";
import { AssistantHosts } from "./AssistantHosts";
import { RulesFilesView, rulesFilesHelp } from "./RulesFilesView";
import type { AgentHostStatus, RulesFileView } from "../types/api";

/** The two tabs of the reader. */
export type AssistantTab = "rules" | "brief";

export interface AssistantPanelProps {
  tabID: string;
  /** Open the brief on this file rather than on the rules files. */
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
  const [tab, setTab] = useState<AssistantTab>(path ? "brief" : "rules");
  // A path handed in later, from Ask or Checks, opens the brief on it.
  const [openedAt, setOpenedAt] = useState(path);
  if (path !== openedAt) {
    setOpenedAt(path);
    if (path) setTab("brief");
  }

  return (
    <div data-slot="assistant-panel">
      <PageHeader title="What your assistant reads" />
      <section data-slot="assistant-connections">
        <SectionHeading className="mb-1">Connected assistants</SectionHeading>
        <AssistantHosts tabID={tabID} hosts={hosts} contextOnly={contextOnly} />
      </section>

      {!contextOnly && (
        <Tabs value={tab} onValueChange={(v) => setTab(v as AssistantTab)} className="mt-8 gap-4">
          <TabsList variant="line">
            <TabsTrigger value="rules" data-testid="assistant-tab-rules">
              {/* On the label, not the trigger: a tooltip trigger sets its own
                  data-state, which hides the tab's active style. */}
              <SimpleTooltip content={rulesFilesHelp()}>
                <span data-slot="assistant-tab-rules-label">Rules files</span>
              </SimpleTooltip>
            </TabsTrigger>
            <TabsTrigger value="brief" data-testid="assistant-tab-brief">
              The brief for a file
            </TabsTrigger>
          </TabsList>
          <TabsContent value="rules">
            <RulesFilesView tabID={tabID} rulesFiles={rulesFiles} />
          </TabsContent>
          <TabsContent value="brief">
            <div data-slot="assistant-brief">
              <AgentContextPane key={path ?? ""} tabID={tabID} path={path} />
            </div>
          </TabsContent>
        </Tabs>
      )}
    </div>
  );
}

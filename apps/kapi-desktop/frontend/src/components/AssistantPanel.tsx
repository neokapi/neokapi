// What your assistant reads, reached from the Assistants page: a reader with
// two tabs. Rules files shows the sections kapi writes where an assistant
// reads them as it starts; the brief for a file shows the whole answer an
// assistant gets for one file.
//
// The panel opens on the rules files, or on the brief when it is handed a
// path, as Ask and Checks do when they open it at a file.

import { useState } from "react";
import { ArrowLeft } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import {
  Button,
  PageHeader,
  SimpleTooltip,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@neokapi/ui-primitives";
import { AgentContextPane } from "./AgentContextPane";
import { RulesFilesView, rulesFilesHelp } from "./RulesFilesView";
import type { RulesFileView } from "../types/api";

/** The two tabs of the reader. */
export type AssistantTab = "rules" | "brief";

export interface AssistantPanelProps {
  tabID: string;
  /** Open the brief on this file rather than on the rules files. */
  path?: string;
  /** Return to the Assistants page. Absent shows no back button. */
  onBack?: () => void;
  /** Pre-loaded rules files for Storybook and tests, which reach no backend. */
  rulesFiles?: RulesFileView[];
}

export function AssistantPanel({ tabID, path, onBack, rulesFiles }: AssistantPanelProps) {
  const [tab, setTab] = useState<AssistantTab>(path ? "brief" : "rules");
  // A path handed in later, from Ask or Checks, opens the brief on it.
  const [openedAt, setOpenedAt] = useState(path);
  if (path !== openedAt) {
    setOpenedAt(path);
    if (path) setTab("brief");
  }

  return (
    <div data-slot="assistant-panel">
      <PageHeader
        title="What your assistant reads"
        backButton={
          onBack && (
            <Button size="sm" variant="ghost" data-slot="assistant-reads-back" onClick={onBack}>
              <ArrowLeft size={14} />
              {t("Assistants")}
            </Button>
          )
        }
      />
      <Tabs value={tab} onValueChange={(v) => setTab(v as AssistantTab)} className="gap-4">
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
    </div>
  );
}

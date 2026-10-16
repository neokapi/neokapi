// The workspace's history: every operation recorded in every project, as a
// list of sessions beside a detail pane with the reset each session offers.
// The home links here from beside its title.

import { ArrowLeft } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { Button, PageHeader, SimpleTooltip } from "@neokapi/ui-primitives";
import { ContextFeedPanel } from "./ContextFeedPanel";
import type { ContextFeed } from "../types/api";

export interface HistoryViewProps {
  /** Return to the home. */
  onBack: () => void;
  /** Pre-loaded feed for Storybook and tests, which reach no backend. */
  feed?: ContextFeed;
  /** The session to open on, for Storybook. Absent opens on the newest. */
  defaultSelected?: string;
}

/** The command the page does the work of, for the reader who has a terminal. */
const COMMAND = "kapi context log";

export function HistoryView({ onBack, feed, defaultSelected }: HistoryViewProps) {
  return (
    <div data-slot="history-view" className="flex h-full min-h-0 flex-col">
      <PageHeader
        className="mb-0 shrink-0 px-6 pt-5 pb-3"
        title="History"
        backButton={
          <Button variant="ghost" size="icon-sm" onClick={onBack} aria-label="Back to home">
            <ArrowLeft size={16} />
          </Button>
        }
        actions={
          <SimpleTooltip content={t("The same as {command} in a terminal", { command: COMMAND })}>
            <code
              data-slot="history-command"
              translate="no"
              className="rounded-md border border-border px-1.5 py-0.5 font-mono text-[11px] text-muted-foreground"
            >
              {COMMAND}
            </code>
          </SimpleTooltip>
        }
      />
      <div className="min-h-0 flex-1 border-t border-border">
        <ContextFeedPanel feed={feed} defaultSelected={defaultSelected} />
      </div>
    </div>
  );
}

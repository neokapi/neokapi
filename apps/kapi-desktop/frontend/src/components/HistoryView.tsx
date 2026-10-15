// The workspace's history: every operation recorded in every project, grouped
// by session, with the reset each session offers. The home links here from
// beside its title.

import { ArrowLeft } from "lucide-react";
import { Button, PageHeader } from "@neokapi/ui-primitives";
import { ContextFeedPanel } from "./ContextFeedPanel";
import type { ContextFeed } from "../types/api";

export interface HistoryViewProps {
  /** Return to the home. */
  onBack: () => void;
  /** Pre-loaded feed for Storybook and tests, which reach no backend. */
  feed?: ContextFeed;
}

export function HistoryView({ onBack, feed }: HistoryViewProps) {
  return (
    <div data-slot="history-view" className="mx-auto max-w-3xl p-6">
      <PageHeader
        title="History"
        subtitle={
          <code className="font-mono" translate="no">
            kapi context log
          </code>
        }
        backButton={
          <Button variant="ghost" size="icon-sm" onClick={onBack} aria-label="Back to home">
            <ArrowLeft size={16} />
          </Button>
        }
      />
      <ContextFeedPanel feed={feed} />
    </div>
  );
}

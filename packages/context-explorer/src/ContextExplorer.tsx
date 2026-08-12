// The assembled explorer (Apache-2.0).
//
// One selection, three panes. The ladder and the filter bar edit the same scope
// tuple, and every pane re-asks its question at whatever the tuple now names —
// which is what makes "the same explorer against a project and against a
// workspace" one component set rather than two that resemble each other.

import { useState } from "react";
import { cn } from "@neokapi/ui-primitives";
import { ContextSearchPanel } from "./ContextSearchPanel";
import { GovernsPane } from "./GovernsPane";
import { LadderBreadcrumbs } from "./LadderBreadcrumbs";
import { LivesPane } from "./LivesPane";
import { RelatesPane } from "./RelatesPane";
import { ScopeFilterBar } from "./ScopeFilterBar";
import { useScope } from "./ScopeProvider";
import type { RelationSubject } from "./types";

export interface ContextExplorerProps {
  /** Cap on each pane's list. */
  limit?: number;
  /** Rendered above the ladder — a surface's own title or toolbar. */
  header?: React.ReactNode;
  className?: string;
}

export function ContextExplorer({ limit, header, className }: ContextExplorerProps) {
  const { setDimension, setScope, scope } = useScope();
  const [subject, setSubject] = useState<RelationSubject | undefined>(undefined);

  return (
    <div className={cn("flex min-h-0 flex-col gap-4", className)} data-testid="context-explorer">
      {header}

      <div className="space-y-3">
        <LadderBreadcrumbs />
        <ScopeFilterBar />
      </div>

      <ContextSearchPanel limit={limit} onSelect={setSubject} />

      <div className="grid gap-4 lg:grid-cols-3">
        <GovernsPane limit={limit} />
        <LivesPane
          limit={limit}
          onOpenCollection={(collection) => setDimension("collection", collection)}
          onOpenItem={(item) => setSubject({ kind: "block", id: item.id, label: item.document })}
        />
        <RelatesPane
          subject={subject}
          limit={limit}
          onOpenOccurrence={(place) =>
            setScope({
              ...scope,
              project: place.project ?? scope.project,
              collection: place.collection ?? scope.collection,
              path: place.document,
            })
          }
        />
      </div>
    </div>
  );
}

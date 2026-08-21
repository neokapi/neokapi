// ConceptStorySection — one concept's dashboard in the Brand hub (AD-021), now the
// framework concept UI. It renders @neokapi/concept-ui's ConceptDashboard against
// a RestConceptDataSource built over the workspace's ApiAdapter, so bowrain gets
// the full feature set: terms, the local relations widget, geography (markets),
// constraints, the revision timeline, observations, and threaded discussion — plus
// governance-aware editing. When an edit is governed (a banned/promoted term, an
// un-forbidding, a REPLACED_BY relation) the source raises a GovernedEditError;
// this section catches it and offers to open it as an experiment instead of failing
// silently. Replaces the deleted tabbed ConceptStoryView.
import { useCallback, useMemo, useState } from "react";
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@neokapi/ui-primitives";
import type { Concept, ConceptDataSource } from "@neokapi/concept-ui";
import { ConceptDashboard } from "@neokapi/concept-ui";
import { FlaskConical } from "../../components/icons";
import { useApi } from "../../context/ApiContext";
import { useWorkspace } from "../../context/WorkspaceContext";
import { createRestConceptSource, type GovernedEditError } from "./restConceptSource";
import { ConceptEditDialog } from "./ConceptEditDialog";

export interface ConceptStorySectionProps {
  conceptId: string;
  /** Back to the Concepts list. */
  onBack: () => void;
  /** Re-centre on another concept (a relation target). */
  onOpenConcept: (conceptId: string) => void;
  /** Jump to the Experiments section to open a change-set for a governed edit. */
  onOpenExperiments?: () => void;
}

export function ConceptStorySection({
  conceptId,
  onBack,
  onOpenConcept,
  onOpenExperiments,
}: ConceptStorySectionProps) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  const [governed, setGoverned] = useState<GovernedEditError | null>(null);
  const [editing, setEditing] = useState<Concept | null>(null);
  // Bumped after an edit lands so the dashboard remounts and reloads its data
  // (concept-ui's panels keep their own state and expose no external reload).
  const [reloadKey, setReloadKey] = useState(0);

  const handleGoverned = useCallback((error: GovernedEditError) => setGoverned(error), []);
  const source: ConceptDataSource = useMemo(
    () => createRestConceptSource(api, ws, { onGovernedEdit: handleGoverned }),
    [api, ws, handleGoverned],
  );

  return (
    <div className="mx-auto w-full max-w-7xl px-1 py-1">
      <ConceptDashboard
        key={`${conceptId}#${reloadKey}`}
        conceptId={conceptId}
        source={source}
        onNavigate={onOpenConcept}
        onBack={onBack}
        onEdit={setEditing}
      />

      {editing && (
        <ConceptEditDialog
          concept={editing}
          source={source}
          open={editing !== null}
          onOpenChange={(open) => {
            if (!open) setEditing(null);
          }}
          onApplied={() => setReloadKey((k) => k + 1)}
        />
      )}

      <GovernedEditDialog
        error={governed}
        onClose={() => setGoverned(null)}
        onOpenExperiments={
          onOpenExperiments
            ? () => {
                setGoverned(null);
                setEditing(null);
                onOpenExperiments();
              }
            : undefined
        }
      />
    </div>
  );
}

function GovernedEditDialog({
  error,
  onClose,
  onOpenExperiments,
}: {
  error: GovernedEditError | null;
  onClose: () => void;
  onOpenExperiments?: () => void;
}) {
  return (
    <Dialog open={error !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <FlaskConical className="size-4 text-primary" />
            This change needs review
          </DialogTitle>
          <DialogDescription>
            {error?.detail
              ? `Changing ${error.detail} is governed — it must travel through a reviewed change-set before it reaches the live graph.`
              : "This change is governed — it must travel through a reviewed change-set before it reaches the live graph."}
          </DialogDescription>
        </DialogHeader>
        <p className="text-sm text-muted-foreground">
          Open it as an experiment in the Experiments section: add the governed change there, then
          submit it for review.
        </p>
        <DialogFooter>
          <Button variant="ghost" size="sm" onClick={onClose}>
            Not now
          </Button>
          {onOpenExperiments && (
            <Button size="sm" onClick={onOpenExperiments}>
              <FlaskConical />
              Open Experiments
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// The prompt an edit raises when the server refuses it as governed: banning or
// promoting a term, un-forbidding one, or a replaced-by relation travels
// through a reviewed change-set, so the edit opens as an experiment instead.
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@neokapi/ui-primitives";
import { FlaskConical } from "../../components/icons";
import type { GovernedEditError } from "./restConceptSource";

export interface GovernedEditDialogProps {
  /** The refusal to explain, or null when the dialog is closed. */
  error: GovernedEditError | null;
  onClose: () => void;
  /** Open the Changes section, where the edit goes into a change-set. Absent hides the act. */
  onOpenExperiments?: () => void;
}

export function GovernedEditDialog({ error, onClose, onOpenExperiments }: GovernedEditDialogProps) {
  return (
    <Dialog open={error !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-md" data-slot="concept-governed-dialog">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <FlaskConical className="size-4 text-primary" />
            This change needs review
          </DialogTitle>
          <DialogDescription>
            {error?.detail
              ? `Changing ${error.detail} is governed: it must travel through a reviewed change-set before it reaches the live graph.`
              : "This change is governed: it must travel through a reviewed change-set before it reaches the live graph."}
          </DialogDescription>
        </DialogHeader>
        <p className="text-sm text-muted-foreground">
          Open it as an experiment in the Changes section: add the governed change there, then
          submit it for review.
        </p>
        <DialogFooter>
          <Button variant="ghost" size="sm" onClick={onClose}>
            Not now
          </Button>
          {onOpenExperiments && (
            <Button size="sm" onClick={onOpenExperiments}>
              <FlaskConical />
              Open Changes
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

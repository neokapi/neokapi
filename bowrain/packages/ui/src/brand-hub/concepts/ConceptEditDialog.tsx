// ConceptEditDialog — the bowrain editor behind the concept-ui dashboard's "Edit"
// affordance (AD-021). The framework concept UI renders an Edit button when the
// data source can edit terms but leaves the editor to the consuming app; bowrain
// fills it with a per-term status editor. Changing a term's status applies
// through the data source's setTermStatus, so an ORDINARY transition (e.g.
// proposed → approved) lands directly, while a GOVERNED one (to/from forbidden or
// preferred, un-forbidding) is refused by the server and surfaced to the user as
// "open it as an experiment" — the dialog closes and the source's onGovernedEdit
// raises the governed prompt.
import { useEffect, useState } from "react";
import {
  Badge,
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@neokapi/ui-primitives";
import type { Concept, ConceptDataSource, Term, TermStatus } from "@neokapi/concept-ui";
import { TERM_STATUSES, TERM_STATUS_LABEL, primaryName } from "@neokapi/concept-ui";
import { isGovernedEditError } from "./restConceptSource";

export interface ConceptEditDialogProps {
  /** The concept being edited (the dashboard hands this to onEdit). */
  concept: Concept;
  /** The governance-aware data source whose setTermStatus applies the change. */
  source: ConceptDataSource;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Called after a status change lands, so the dashboard can reload. */
  onApplied?: () => void;
}

export function ConceptEditDialog({
  concept,
  source,
  open,
  onOpenChange,
  onApplied,
}: ConceptEditDialogProps) {
  const [terms, setTerms] = useState<Term[]>(concept.terms);
  const [busyKey, setBusyKey] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Re-seed from the concept whenever the dialog (re)opens for a concept.
  useEffect(() => {
    if (open) {
      setTerms(concept.terms);
      setError(null);
    }
  }, [open, concept]);

  const changeStatus = async (term: Term, status: TermStatus) => {
    if (!source.setTermStatus || status === term.status) return;
    const key = `${term.locale}:${term.text}`;
    setBusyKey(key);
    setError(null);
    try {
      await source.setTermStatus(concept.id, { locale: term.locale, text: term.text }, status);
      setTerms((prev) =>
        prev.map((t) => (t.locale === term.locale && t.text === term.text ? { ...t, status } : t)),
      );
      onApplied?.();
    } catch (caught) {
      if (isGovernedEditError(caught)) {
        // The source's onGovernedEdit already raised the "open as experiment"
        // prompt; step out of the way so it is visible.
        onOpenChange(false);
        return;
      }
      setError(caught instanceof Error ? caught.message : "Could not change the term status.");
    } finally {
      setBusyKey(null);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Edit “{primaryName(concept)}”</DialogTitle>
          <DialogDescription>
            Set each term&apos;s status. Banning or promoting a term is governed and opens as an
            experiment for review.
          </DialogDescription>
        </DialogHeader>

        {terms.length === 0 ? (
          <p className="py-4 text-sm text-muted-foreground">This concept has no terms to edit.</p>
        ) : (
          <ul className="space-y-2">
            {terms.map((term) => {
              const key = `${term.locale}:${term.text}`;
              return (
                <li
                  key={key}
                  className="flex items-center justify-between gap-3 rounded-lg border px-3 py-2"
                >
                  <span className="flex min-w-0 items-center gap-2">
                    <Badge variant="outline" className="shrink-0 font-mono text-[10px]">
                      {term.locale}
                    </Badge>
                    <span className="truncate text-sm font-medium text-foreground">
                      {term.text}
                    </span>
                  </span>
                  <Select
                    value={term.status}
                    onValueChange={(value) => void changeStatus(term, value as TermStatus)}
                    disabled={busyKey === key}
                  >
                    <SelectTrigger
                      size="sm"
                      className="w-40"
                      aria-label={`Status for ${term.text}`}
                    >
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {TERM_STATUSES.map((status) => (
                        <SelectItem key={status} value={status}>
                          {TERM_STATUS_LABEL[status]}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </li>
              );
            })}
          </ul>
        )}

        {error && <p className="text-sm text-destructive">{error}</p>}

        <DialogFooter>
          <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>
            Done
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

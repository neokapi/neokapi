import { useState, useEffect } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
  Button,
  Label,
  Input,
  LocaleLabel,
  Textarea,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@neokapi/ui-primitives";
import { useCreateConcept } from "../../hooks/useConceptsApi";
import { useCreateChangeset } from "../../hooks/useChangesetsApi";
import { ErrorNotice } from "../../errors";
import { ShieldCheck, Info } from "../icons";

/** Term statuses a reviewer can request from the source-side lane. */
type MarkStatus = "admitted" | "preferred" | "forbidden";

const STATUS_OPTIONS: { value: MarkStatus; label: string; governed: boolean; hint: string }[] = [
  {
    value: "admitted",
    label: "Track it (admitted)",
    governed: false,
    hint: "Add to your terms as a known term. A direct, discovery-level mark.",
  },
  {
    value: "preferred",
    label: "Preferred",
    governed: true,
    hint: "Make this the preferred term. A governed change: it changes what every translation is checked against.",
  },
  {
    value: "forbidden",
    label: "Forbidden",
    governed: true,
    hint: "Forbid this term. A governed change: it hardens into a check on every future generation.",
  },
];

export interface MarkSourceTermDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  projectId: string;
  sourceLocale: string;
  /** Prefills the term from the selected source text. */
  initialTerm?: string;
  /** Called with a status message on success. */
  onDone?: (message: string) => void;
  /** Navigate to a governance change-set (governed marks open one for review). */
  onOpenChangeset?: (changesetId: string) => void;
}

/**
 * MarkSourceTermDialog marks a selected source span as a term. Governance
 * matters: a discovery-level `admitted` mark is created directly, but a
 * status-changing `preferred` / `forbidden` mark is a governed transition: it
 * opens a change-set for review rather than being applied silently (the direct
 * path would 409). Marking a term re-checks the translations that use it across
 * the project's locales when the change is merged; that fan-out is a separate
 * track, so this affordance stays correct and legible without triggering it.
 */
export function MarkSourceTermDialog({
  open,
  onOpenChange,
  projectId,
  sourceLocale,
  initialTerm = "",
  onDone,
  onOpenChangeset,
}: MarkSourceTermDialogProps) {
  const createConcept = useCreateConcept();
  const createChangeset = useCreateChangeset();
  const [term, setTerm] = useState(initialTerm);
  const [domain, setDomain] = useState("general");
  const [definition, setDefinition] = useState("");
  const [status, setStatus] = useState<MarkStatus>("admitted");

  useEffect(() => {
    if (open) {
      setTerm(initialTerm);
      setDomain("general");
      setDefinition("");
      setStatus("admitted");
      createConcept.reset();
      createChangeset.reset();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, initialTerm]);

  const opt = STATUS_OPTIONS.find((o) => o.value === status)!;
  const pending = createConcept.isPending || createChangeset.isPending;
  const error = createConcept.error ?? createChangeset.error;
  const canSubmit = term.trim().length > 0 && !pending;

  const submit = () => {
    const text = term.trim();
    if (!opt.governed) {
      // Discovery mark → direct concept create (admitted stays off the
      // governed 409 path).
      createConcept.mutate(
        {
          project_id: projectId,
          domain: domain.trim() || "general",
          definition: definition.trim() || text,
          terms: [{ text, locale: sourceLocale, status: "admitted" }],
        },
        {
          onSuccess: () => {
            onDone?.(`Added "${text}" to your terms.`);
            onOpenChange(false);
          },
        },
      );
      return;
    }
    // Governed transition → open a change-set for review (never a silent 409).
    createChangeset.mutate(
      {
        name: `Mark "${text}" ${status}`,
        description: `Requested from review: mark the ${sourceLocale} term "${text}" as ${status}. Add the governed concept op and submit for approval.`,
      },
      {
        onSuccess: (cs) => {
          onDone?.(`Opened change-set "${cs.name}" for review.`);
          onOpenChange(false);
          onOpenChangeset?.(cs.id);
        },
      },
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="mark-term-dialog">
        <DialogHeader>
          <DialogTitle>Mark as term</DialogTitle>
          <DialogDescription>
            Add a source term to the shared terms. Preferred and forbidden marks are governed: they
            open a change-set for review.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor="mark-term-text" className="flex items-center gap-1.5">
              Term
              <LocaleLabel locale={sourceLocale} source className="font-normal" />
            </Label>
            <Input
              id="mark-term-text"
              value={term}
              onChange={(e) => setTerm(e.target.value)}
              data-testid="mark-term-text"
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <Label htmlFor="mark-term-domain">Domain</Label>
              <Input
                id="mark-term-domain"
                value={domain}
                onChange={(e) => setDomain(e.target.value)}
                data-testid="mark-term-domain"
              />
            </div>
            <div className="space-y-1.5">
              <Label>Mark as</Label>
              <Select value={status} onValueChange={(v) => setStatus(v as MarkStatus)}>
                <SelectTrigger data-testid="mark-term-status">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {STATUS_OPTIONS.map((o) => (
                    <SelectItem key={o.value} value={o.value}>
                      {o.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="mark-term-def">Definition (optional)</Label>
            <Textarea
              id="mark-term-def"
              value={definition}
              onChange={(e) => setDefinition(e.target.value)}
              rows={2}
              data-testid="mark-term-def"
            />
          </div>

          <div
            className={
              opt.governed
                ? "flex items-start gap-2 rounded-md border border-warning/30 bg-warning/5 p-2 text-xs text-warning"
                : "flex items-start gap-2 rounded-md border border-border bg-muted/30 p-2 text-xs text-muted-foreground"
            }
            data-testid="mark-term-impact"
          >
            {opt.governed ? (
              <ShieldCheck className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            ) : (
              <Info className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            )}
            <span>
              {opt.hint}
              {opt.governed
                ? " On merge it re-checks the translations that use this term across the project's locales."
                : ""}
            </span>
          </div>

          {error && <ErrorNotice error={error} title="Couldn't mark the term" variant="inline" />}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={submit} disabled={!canSubmit} data-testid="mark-term-submit">
            {opt.governed ? "Open change-set" : "Add term"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

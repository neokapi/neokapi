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
  Textarea,
  LocaleLabel,
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@neokapi/ui-primitives";
import { useCreateSourceProposal } from "../../hooks/useSourceProposalsApi";
import { ErrorNotice } from "../../errors";
import { AlertTriangle } from "../icons";
import type { SourceProposalKind } from "../../types/api";

const KIND_OPTIONS: { value: SourceProposalKind; label: string; hint: string }[] = [
  {
    value: "text-fix",
    label: "Fix the source text",
    hint: "A typo or wording fix. The new source replaces the old one and every locale is re-drafted from it.",
  },
  {
    value: "mark-dnt",
    label: "Mark do-not-translate",
    hint: "This string should not be translated. The new source is applied and recorded do-not-translate.",
  },
];

export interface ProposeSourceChangeDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  projectId: string;
  blockId: string;
  itemName: string;
  sourceLocale: string;
  /** The target locale the reviewer was working on (attribution). */
  foundInLocale?: string;
  /** Prefills the proposed text with the current source (or the selection). */
  initialSource?: string;
  /** How many target locales a source change re-drafts (blast-radius legibility). */
  localeCount?: number;
  /** Called with a status message on success. */
  onDone?: (message: string) => void;
}

/**
 * ProposeSourceChangeDialog is the back-to-source lane (RV-F). A reviewer who
 * catches a source-TEXT problem while reviewing a target proposes a fix here.
 * Unlike "Mark as term" (annotate → re-check the locales that use the term), a
 * source change is a TRANSFORM: once a source owner approves it, the source is
 * rewritten and EVERY locale is re-drafted. The dialog makes that wider blast
 * radius legible so a reviewer knows the difference before proposing.
 */
export function ProposeSourceChangeDialog({
  open,
  onOpenChange,
  projectId,
  blockId,
  itemName,
  sourceLocale,
  foundInLocale,
  initialSource = "",
  localeCount,
  onDone,
}: ProposeSourceChangeDialogProps) {
  const createProposal = useCreateSourceProposal(projectId);
  const [proposed, setProposed] = useState(initialSource);
  const [rationale, setRationale] = useState("");
  const [kind, setKind] = useState<SourceProposalKind>("text-fix");

  useEffect(() => {
    if (open) {
      setProposed(initialSource);
      setRationale("");
      setKind("text-fix");
      createProposal.reset();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, initialSource]);

  const opt = KIND_OPTIONS.find((o) => o.value === kind)!;
  const trimmed = proposed.trim();
  const unchanged = trimmed === initialSource.trim();
  const canSubmit =
    trimmed.length > 0 && !(kind === "text-fix" && unchanged) && !createProposal.isPending;

  const localeText =
    localeCount && localeCount > 0
      ? `all ${localeCount} locale${localeCount === 1 ? "" : "s"}`
      : "every locale";

  const submit = () => {
    createProposal.mutate(
      {
        block_id: blockId,
        item_name: itemName,
        proposed_source: trimmed,
        kind,
        rationale: rationale.trim() || undefined,
        found_in_locale: foundInLocale,
      },
      {
        onSuccess: () => {
          onDone?.("Source change proposed. A source owner will review it.");
          onOpenChange(false);
        },
      },
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="propose-source-dialog">
        <DialogHeader>
          <DialogTitle>Propose a source change</DialogTitle>
          <DialogDescription>
            Caught a problem in the source while reviewing{" "}
            {foundInLocale ? <LocaleLabel locale={foundInLocale} hideCode /> : "a translation"}?
            Propose a fix. A source owner approves it, and because that changes the source itself it
            is governed separately from your target review.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor="propose-source-kind">Kind</Label>
            <Select value={kind} onValueChange={(v) => setKind(v as SourceProposalKind)}>
              <SelectTrigger id="propose-source-kind" data-testid="propose-source-kind">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {KIND_OPTIONS.map((o) => (
                  <SelectItem key={o.value} value={o.value}>
                    {o.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="propose-source-text" className="flex items-center gap-1.5">
              Proposed source
              <LocaleLabel locale={sourceLocale} source className="font-normal" />
            </Label>
            <Textarea
              id="propose-source-text"
              value={proposed}
              onChange={(e) => setProposed(e.target.value)}
              rows={3}
              data-testid="propose-source-text"
            />
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="propose-source-rationale">Why (optional)</Label>
            <Textarea
              id="propose-source-rationale"
              value={rationale}
              onChange={(e) => setRationale(e.target.value)}
              rows={2}
              placeholder="e.g. British spelling; house style is US English"
              data-testid="propose-source-rationale"
            />
          </div>

          {/* Blast-radius legibility: a source change is wider than a term mark. */}
          <div
            className="flex items-start gap-2 rounded-md border border-warning/30 bg-warning/5 p-2 text-xs text-warning"
            data-testid="propose-source-impact"
          >
            <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            <span>
              {opt.hint} On approval this re-drafts {localeText}, wider than a term mark, which only
              re-checks the locales that use the term.
            </span>
          </div>

          {createProposal.error && (
            <ErrorNotice
              error={createProposal.error}
              title="Couldn't propose the change"
              variant="inline"
            />
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={submit} disabled={!canSubmit} data-testid="propose-source-submit">
            Propose change
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

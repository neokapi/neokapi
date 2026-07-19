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
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@neokapi/ui-primitives";
import type { Dimension } from "../../brand/types";
import { useRecordBrandCorrection } from "../../hooks/useBrandApi";
import { ErrorNotice } from "../../errors";

const DIMENSIONS: { value: Dimension; label: string }[] = [
  { value: "vocabulary", label: "Vocabulary" },
  { value: "tone", label: "Tone" },
  { value: "style", label: "Style" },
  { value: "clarity", label: "Clarity" },
  { value: "brand_compliance", label: "Brand compliance" },
];

export interface SuggestBrandRuleDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  projectId: string;
  stream?: string;
  /** The bound brand profile the correction feeds. */
  profileId: string;
  /** Prefills the "original" field (the reviewed text a correction improves). */
  initialOriginal?: string;
  /** The block the correction is anchored to, when known. */
  blockId?: string;
  /** Called with a status message on success. */
  onDone?: (message: string) => void;
}

/**
 * SuggestBrandRuleDialog records a reviewer's in-place correction
 * (original → corrected) into the correction-learning loop for the project's
 * bound brand profile. Repeated corrections surface as candidate rules and,
 * past the profile's threshold, harden into a check on every future
 * generation — the loop-improvement signal a reviewer emits without leaving
 * the block.
 */
export function SuggestBrandRuleDialog({
  open,
  onOpenChange,
  projectId,
  stream,
  profileId,
  initialOriginal = "",
  blockId,
  onDone,
}: SuggestBrandRuleDialogProps) {
  const record = useRecordBrandCorrection(projectId, stream);
  const [original, setOriginal] = useState(initialOriginal);
  const [corrected, setCorrected] = useState("");
  const [dimension, setDimension] = useState<Dimension>("vocabulary");

  useEffect(() => {
    if (open) {
      setOriginal(initialOriginal);
      setCorrected("");
      setDimension("vocabulary");
      record.reset();
    }
    // Only re-seed when the dialog opens or the prefill changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, initialOriginal]);

  const canSubmit = original.trim().length > 0 && corrected.trim().length > 0 && !record.isPending;

  const submit = () => {
    record.mutate(
      {
        profile_id: profileId,
        block_id: blockId,
        dimension,
        original_text: original.trim(),
        corrected_text: corrected.trim(),
      },
      {
        onSuccess: (result) => {
          onDone?.(
            result.auto_promoted
              ? "Recorded — this correction crossed the threshold and became a rule."
              : "Recorded — it surfaces as a candidate rule after enough corrections.",
          );
          onOpenChange(false);
        },
      },
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="suggest-brand-rule-dialog">
        <DialogHeader>
          <DialogTitle>Suggest a brand rule</DialogTitle>
          <DialogDescription>
            Turn this fix into a brand-voice correction. Repeated corrections become a candidate
            rule you can promote into a check on every future generation.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor="brand-rule-original">Original</Label>
            <Textarea
              id="brand-rule-original"
              value={original}
              onChange={(e) => setOriginal(e.target.value)}
              rows={2}
              data-testid="brand-rule-original"
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="brand-rule-corrected">Preferred</Label>
            <Textarea
              id="brand-rule-corrected"
              value={corrected}
              onChange={(e) => setCorrected(e.target.value)}
              rows={2}
              placeholder="What it should say instead"
              data-testid="brand-rule-corrected"
            />
          </div>
          <div className="space-y-1.5">
            <Label>Dimension</Label>
            <Select value={dimension} onValueChange={(v) => setDimension(v as Dimension)}>
              <SelectTrigger data-testid="brand-rule-dimension">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {DIMENSIONS.map((d) => (
                  <SelectItem key={d.value} value={d.value}>
                    {d.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          {record.isError && (
            <ErrorNotice
              error={record.error}
              title="Couldn't record the correction"
              variant="inline"
            />
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={submit} disabled={!canSubmit} data-testid="brand-rule-submit">
            Record correction
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

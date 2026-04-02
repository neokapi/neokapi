import React, { useState, useCallback } from "react";
import { Popover, PopoverContent, PopoverTrigger } from "@neokapi/ui-primitives/components/ui/popover";
import { Button } from "@neokapi/ui-primitives/components/ui/button";
import { Input } from "@neokapi/ui-primitives/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@neokapi/ui-primitives/components/ui/select";
import type { AddConceptRequest, TermInfo } from "../../types/api";

interface TermCreationPopoverProps {
  selectedText: string;
  sourceLocale: string;
  targetLocale: string;
  onSubmit: (req: AddConceptRequest) => Promise<void>;
  onClose: () => void;
  open: boolean;
  anchorPosition?: { x: number; y: number };
}

type TermStatus = "preferred" | "admitted" | "deprecated";

export function TermCreationPopover({
  selectedText,
  sourceLocale,
  targetLocale,
  onSubmit,
  onClose,
  open,
}: TermCreationPopoverProps) {
  const [sourceTerm, setSourceTerm] = useState(selectedText);
  const [targetTranslation, setTargetTranslation] = useState("");
  const [domain, setDomain] = useState("");
  const [status, setStatus] = useState<TermStatus>("preferred");
  const [submitting, setSubmitting] = useState(false);

  // Reset form when popover opens with new selected text.
  React.useEffect(() => {
    if (open) {
      setSourceTerm(selectedText);
      setTargetTranslation("");
      setDomain("");
      setStatus("preferred");
    }
  }, [open, selectedText]);

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      if (!sourceTerm.trim()) return;

      setSubmitting(true);
      try {
        const terms: TermInfo[] = [
          {
            text: sourceTerm.trim(),
            locale: sourceLocale,
            status,
          },
        ];
        if (targetTranslation.trim()) {
          terms.push({
            text: targetTranslation.trim(),
            locale: targetLocale,
            status,
          });
        }

        const req: AddConceptRequest = {
          project_id: "",
          domain: domain.trim(),
          definition: "",
          terms,
        };

        await onSubmit(req);
        onClose();
      } finally {
        setSubmitting(false);
      }
    },
    [sourceTerm, targetTranslation, domain, status, sourceLocale, targetLocale, onSubmit, onClose],
  );

  return (
    <Popover open={open} onOpenChange={(isOpen: boolean) => !isOpen && onClose()}>
      <PopoverTrigger asChild>
        <span />
      </PopoverTrigger>
      <PopoverContent
        className="w-80 p-4"
        side="bottom"
        align="start"
        onOpenAutoFocus={(e: Event) => e.preventDefault()}
      >
        <form onSubmit={handleSubmit} className="space-y-3">
          <div className="text-sm font-medium mb-2">Add Term</div>

          <div className="space-y-1">
            <label className="text-xs text-muted-foreground">Source term ({sourceLocale})</label>
            <Input
              value={sourceTerm}
              onChange={(e) => setSourceTerm(e.target.value)}
              placeholder="Source term"
              className="h-8 text-sm"
              autoFocus
            />
          </div>

          <div className="space-y-1">
            <label className="text-xs text-muted-foreground">Translation ({targetLocale})</label>
            <Input
              value={targetTranslation}
              onChange={(e) => setTargetTranslation(e.target.value)}
              placeholder="Target translation"
              className="h-8 text-sm"
            />
          </div>

          <div className="space-y-1">
            <label className="text-xs text-muted-foreground">Domain (optional)</label>
            <Input
              value={domain}
              onChange={(e) => setDomain(e.target.value)}
              placeholder="e.g. legal, medical"
              className="h-8 text-sm"
            />
          </div>

          <div className="space-y-1">
            <label className="text-xs text-muted-foreground">Status</label>
            <Select value={status} onValueChange={(v: string) => setStatus(v as TermStatus)}>
              <SelectTrigger className="h-8 text-sm">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="preferred">Preferred</SelectItem>
                <SelectItem value="admitted">Admitted</SelectItem>
                <SelectItem value="deprecated">Deprecated</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="flex justify-end gap-2 pt-1">
            <Button type="button" variant="ghost" size="sm" onClick={onClose} disabled={submitting}>
              Cancel
            </Button>
            <Button type="submit" size="sm" disabled={!sourceTerm.trim() || submitting}>
              {submitting ? "Adding..." : "Add Term"}
            </Button>
          </div>
        </form>
      </PopoverContent>
    </Popover>
  );
}

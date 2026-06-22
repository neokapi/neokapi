import { useState, useEffect } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
  Button,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/kapi-react/runtime";
import type { AIModelOption } from "../types/api";
import { api } from "../hooks/useApi";
import { AIModelList } from "./AIModelList";

export interface AIModelPromptDialogProps {
  open: boolean;
  /** Pre-loaded models for Storybook/tests — skips api.listAIModels(). */
  models?: AIModelOption[];
  /** Fired after the user picks a model and it is persisted as the default. */
  onResolved: () => void;
  onCancel: () => void;
}

/**
 * Run-time prompt shown when a flow uses AI but no default model is configured
 * (and credentials don't auto-resolve). The user picks a model — model-first,
 * provider inferred — which is persisted as the shared default and the run then
 * proceeds. Cancelling aborts the launch.
 */
export function AIModelPromptDialog({
  open,
  models: propModels,
  onResolved,
  onCancel,
}: AIModelPromptDialogProps) {
  const [models, setModels] = useState<AIModelOption[]>(propModels ?? []);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!open || propModels) return;
    void api.listAIModels().then((m) => setModels(m ?? []));
  }, [open, propModels]);

  const pick = async (m: AIModelOption) => {
    setBusy(true);
    setError(null);
    try {
      await api.setDefaultModel(m.model, m.provider);
      onResolved();
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) onCancel();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("Choose an AI model")}</DialogTitle>
          <DialogDescription>
            {t(
              "This flow uses AI but no default model is set. Pick one to run with — the provider follows from the model. You can change it later in Settings → AI Models.",
            )}
          </DialogDescription>
        </DialogHeader>
        {error && (
          <p className="text-sm text-destructive" role="alert">
            {error}
          </p>
        )}
        <AIModelList models={models} onSelect={(m) => void pick(m)} />
        <DialogFooter>
          <Button variant="outline" onClick={onCancel} disabled={busy}>
            {t("Cancel")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

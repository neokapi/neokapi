import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type { AISetupNeed } from "../types/api";

export interface AICredentialMissingDialogProps {
  /** The missing credential; the dialog is open while this is set. */
  need: AISetupNeed | null;
  /** Opens App Settings on Credentials, where a provider key is added. */
  onAddKey?: () => void;
  onCancel: () => void;
}

/**
 * Shown before a run when the recipe names a provider whose key is missing.
 * Choosing a model cannot fix that, because the stored default never
 * overrides a provider the recipe names, so the dialog leads to the
 * credentials instead and never changes the stored default. The run does not
 * start.
 */
export function AICredentialMissingDialog({
  need,
  onAddKey,
  onCancel,
}: AICredentialMissingDialogProps) {
  const provider = need?.provider || t("the provider");
  return (
    <Dialog
      open={need !== null}
      onOpenChange={(o) => {
        if (!o) onCancel();
      }}
    >
      <DialogContent data-testid="ai-credential-missing">
        <DialogHeader>
          <DialogTitle>{t("No key for {provider}", { provider })}</DialogTitle>
          <DialogDescription>
            {t("The recipe runs this flow on {provider}. Add a key for it to run.", { provider })}
          </DialogDescription>
        </DialogHeader>
        {need?.message && (
          <p
            className="rounded-md bg-muted px-3 py-2 font-mono text-xs break-words text-muted-foreground"
            translate="no"
          >
            {need.message}
          </p>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={onCancel}>
            Cancel
          </Button>
          {onAddKey && (
            <Button onClick={onAddKey} className="sm:ml-4">
              Add a key
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

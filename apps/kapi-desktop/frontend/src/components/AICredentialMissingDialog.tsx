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
 * Shown before a run when the run names a provider whose key is missing.
 * Choosing a model cannot fix that, because the stored default never
 * overrides a named provider, so the dialog leads to the credentials instead
 * and never changes the stored default. The run does not start. It says where
 * in the app the key goes and nothing about commands or environment
 * variables: an app opened from the Dock does not read the shell's.
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
            {t("Add a key for {provider} in Settings › AI Models.", { provider })}
          </DialogDescription>
        </DialogHeader>
        {(need?.flow || need?.tool) && (
          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
            {need?.flow && (
              <>
                <dt className="text-muted-foreground">{t("Flow")}</dt>
                <dd translate="no">{need.flow}</dd>
              </>
            )}
            {need?.tool && (
              <>
                <dt className="text-muted-foreground">{t("Step")}</dt>
                <dd translate="no">{need.tool}</dd>
              </>
            )}
          </dl>
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

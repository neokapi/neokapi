import { ShieldAlert } from "lucide-react";
import { Alert, AlertDescription, AlertTitle, Button } from "@neokapi/ui-primitives";
import type { ChangeFinding } from "@neokapi/contract-types";
import { t } from "@neokapi/i18n-react/runtime";

export interface GatePromptProps {
  /** What the project's checks found in the wording, from the gate_failed refusal. */
  findings: ChangeFinding[];
  /**
   * Save the wording as it is. The change lands with its findings, and the
   * record of the edit lists them as overridden.
   */
  onOverride: () => void;
  /** Keep the editor open and change the wording instead. */
  onDismiss: () => void;
  busy?: boolean;
}

/**
 * A rule in force fails on the wording a person saved, so nothing was written.
 * The prompt lists what the check found and lets the person save the wording
 * anyway, a deliberate override the edit's record keeps, or go back to
 * editing.
 */
export function GatePrompt({ findings, onOverride, onDismiss, busy }: GatePromptProps) {
  return (
    <Alert className="border-destructive/40" data-slot="gate-prompt">
      <ShieldAlert className="text-destructive" aria-hidden />
      <AlertTitle>{t("A rule in force fails on this wording")}</AlertTitle>
      <AlertDescription>
        <p>{t("Nothing was saved. The project's checks found:")}</p>
        <ul className="my-2 w-full space-y-1" data-slot="gate-prompt-findings">
          {findings.map((f, i) => (
            <li
              key={`${f.rule}:${i}`}
              className="rounded-md border bg-muted/40 px-2 py-1.5 text-foreground"
              data-slot="gate-prompt-finding"
            >
              <span className="font-mono text-[11px] text-muted-foreground" translate="no">
                {f.rule}
              </span>
              <span className="block">{f.message}</span>
            </li>
          ))}
        </ul>
        <p className="mb-2">
          {t("Saving anyway keeps this wording and records the findings beside the edit.")}
        </p>
        <div className="flex flex-wrap items-center gap-2">
          <Button
            size="xs"
            variant="destructive"
            onClick={onOverride}
            disabled={busy}
            data-slot="gate-prompt-override"
          >
            {t("Save anyway")}
          </Button>
          <Button
            variant="outline"
            size="xs"
            onClick={onDismiss}
            disabled={busy}
            data-slot="gate-prompt-dismiss"
          >
            {t("Keep editing")}
          </Button>
        </div>
      </AlertDescription>
    </Alert>
  );
}

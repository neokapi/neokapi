import { RefreshCw, TriangleAlert } from "lucide-react";
import { Alert, AlertDescription, AlertTitle, Button } from "@neokapi/ui-primitives";
import type { CodeRead, CurrentEdition } from "@neokapi/contract-types";
import { t } from "@neokapi/i18n-react/runtime";

import { EditTextDisplay } from "./EditTextDisplay";

export interface StalePromptProps {
  /** The edition as it stands now, from the change service's stale refusal. */
  current: CurrentEdition;
  /** The codes that type the current text's chips. */
  codes?: Readonly<Record<string, CodeRead>>;
  /** The language the text is written in. */
  locale?: string;
  /** What applying again does, said in the button. */
  reapplyLabel?: string;
  /**
   * Apply the same change over the current text. Absent when the change
   * cannot be applied to the text as it stands, which `note` says why.
   */
  onReapply?: () => void;
  /** Why the change is not offered again over the current text. */
  note?: string;
  /** Keep the current text and drop the change. */
  onDiscard: () => void;
  busy?: boolean;
}

/**
 * The content changed between the moment it was read and the moment the change
 * was sent: someone saved the file in an editor, a run rewrote it, or another
 * surface changed it. Nothing was written. The prompt shows the text as it
 * stands and asks before applying the change over it.
 */
export function StalePrompt({
  current,
  codes,
  locale,
  reapplyLabel,
  onReapply,
  note,
  onDiscard,
  busy,
}: StalePromptProps) {
  return (
    <Alert className="border-warning/40" data-slot="stale-prompt">
      <TriangleAlert className="text-warning" aria-hidden />
      <AlertTitle>{t("Changed since you opened it")}</AlertTitle>
      <AlertDescription>
        <p>{t("Nothing was saved. This is the text as it stands now:")}</p>
        <div className="my-2 w-full rounded-md border bg-muted/40 px-2 py-1.5 text-foreground">
          <EditTextDisplay
            text={current.text ?? ""}
            codes={codes}
            locale={locale}
            data-slot="stale-prompt-current"
          />
        </div>
        {note && (
          <p className="mb-2" data-slot="stale-prompt-note">
            {note}
          </p>
        )}
        <div className="flex flex-wrap items-center gap-2">
          {onReapply && (
            <Button size="xs" onClick={onReapply} disabled={busy} data-slot="stale-prompt-reapply">
              {reapplyLabel ?? t("Apply my change over it")}
            </Button>
          )}
          <Button
            variant="outline"
            size="xs"
            onClick={onDiscard}
            disabled={busy}
            data-slot="stale-prompt-discard"
          >
            <RefreshCw size={12} />
            {t("Keep the current text")}
          </Button>
        </div>
      </AlertDescription>
    </Alert>
  );
}

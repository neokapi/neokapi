// A settings field's label, with its explanation behind an info mark.
//
// Settings pages are labelled rows, one control each. A sentence under a field
// is read once and then sits in the way; the info mark keeps it one hover away.

import { Info } from "lucide-react";
import { Label, SimpleTooltip, cn } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";

export interface SettingLabelProps {
  children: React.ReactNode;
  /** The explanation, shown in the info mark's tooltip. */
  help?: React.ReactNode;
  htmlFor?: string;
  className?: string;
}

export function SettingLabel({ children, help, htmlFor, className }: SettingLabelProps) {
  return (
    <span className={cn("mb-1 flex items-center gap-1", className)}>
      <Label htmlFor={htmlFor} className="text-xs text-muted-foreground">
        {children}
      </Label>
      {help && (
        <SimpleTooltip content={help}>
          <button
            type="button"
            className="inline-flex cursor-help text-muted-foreground/70 hover:text-foreground"
            aria-label={t("About this setting")}
            data-testid="setting-help"
          >
            <Info className="size-3" aria-hidden />
          </button>
        </SimpleTooltip>
      )}
    </span>
  );
}

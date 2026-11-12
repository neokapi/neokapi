import * as React from "react";
import { Info } from "lucide-react";
import { t } from "@neokapi/i18n-react/runtime";
import { cn } from "../../lib/utils";
import { SimpleTooltip } from "./tooltip";

/**
 * How wide a page's content runs. Each page shape has one width, so pages of
 * the same shape line up on the desktop and the platform alike:
 *
 * - `narrow`: a setup step, a sign-in, a form of two or three fields.
 * - `standard`: Settings and setup pages.
 * - `wide`: an Overview, a list or table page, a Reader with its side pane.
 * - `full`: List and detail, editors and canvases. These run edge to edge and
 *   draw their own panes.
 */
export type PageWidth = "narrow" | "standard" | "wide" | "full";

const WIDTH: Record<Exclude<PageWidth, "full">, string> = {
  narrow: "max-w-xl",
  standard: "max-w-5xl",
  wide: "max-w-7xl",
};

interface PageContainerProps extends React.HTMLAttributes<HTMLDivElement> {
  width: PageWidth;
}

/**
 * The column a page's content sits in: centred, at the width its shape takes,
 * with the same side padding on every page. A page sets its width here and
 * nowhere else, so no page carries a `max-w-*` or `mx-auto` of its own.
 */
export function PageContainer({ width, className, children, ...rest }: PageContainerProps) {
  return (
    <div
      data-slot="page-container"
      data-width={width}
      className={cn(
        width === "full"
          ? "flex h-full min-h-0 w-full flex-col"
          : cn("mx-auto w-full px-6 py-6", WIDTH[width]),
        className,
      )}
      {...rest}
    >
      {children}
    </div>
  );
}

interface SettingsRowProps {
  /** The setting's name. */
  label: React.ReactNode;
  /**
   * What the setting does, behind an information mark beside the label, for
   * a reader who asks. It is never a visible line under the label.
   */
  help?: React.ReactNode;
  /** The id of the control, so the label focuses it. */
  htmlFor?: string;
  children: React.ReactNode;
  className?: string;
}

/**
 * One setting: its label on the left and its control on the right, from the
 * `md` breakpoint up. Below it the two stack. The control fills its column,
 * so a settings page uses the width of its container rather than a column of
 * short fields at the left edge.
 */
export function SettingsRow({ label, help, htmlFor, children, className }: SettingsRowProps) {
  return (
    <div
      data-slot="settings-row"
      className={cn(
        "grid gap-2 py-3 md:grid-cols-[minmax(10rem,16rem)_minmax(0,1fr)] md:items-start md:gap-8",
        className,
      )}
    >
      <div className="flex items-center gap-1 md:pt-2">
        <label htmlFor={htmlFor} className="text-sm font-medium">
          {label}
        </label>
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
      </div>
      <div className="min-w-0">{children}</div>
    </div>
  );
}

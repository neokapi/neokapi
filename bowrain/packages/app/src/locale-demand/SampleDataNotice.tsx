import type { ReactNode } from "react";
import { Alert, AlertDescription, AlertTitle, Badge, FlaskConical, cn } from "@neokapi/ui";

/**
 * Disclosure for the fabricated demand dataset. The numbers on the locale
 * demand page are convincing enough to be mistaken for a workspace's own
 * traffic, so the label rides along with the data: the notice sits under the
 * page header, and `SampleDataMark` repeats on every surface that can be
 * screenshotted on its own (map, tables, drill-down).
 */
export interface SampleDataNoticeProps {
  /** Connect affordance; omitted when the workspace has no project to attach a connector to. */
  action?: ReactNode;
  /** Set when a configured PostHog source failed and the page fell back to the fixture. */
  degradedReason?: string;
  className?: string;
}

export function SampleDataNotice({ action, degradedReason, className }: SampleDataNoticeProps) {
  return (
    <Alert
      data-testid="sample-data-notice"
      className={cn(
        "mb-4 border-amber-500/40 bg-amber-500/5 px-3 py-2.5 dark:border-amber-400/30",
        className,
      )}
    >
      <FlaskConical className="text-amber-600 dark:text-amber-400" />
      <AlertTitle>Sample data: connect PostHog to see your own</AlertTitle>
      <AlertDescription>
        {degradedReason
          ? `PostHog could not be read, so every market, language and session count below comes from a fabricated dataset that ships with the product. None of it is this workspace's traffic. Reported cause: ${degradedReason}`
          : "Every market, language and session count below comes from a fabricated dataset that ships with the product. None of it is this workspace's traffic."}
      </AlertDescription>
      {action && <div className="col-start-2 mt-2 flex flex-wrap gap-2">{action}</div>}
    </Alert>
  );
}

/** The notice, shrunk to a chip, for surfaces that travel away from the header. */
export function SampleDataMark({ className }: { className?: string }) {
  return (
    <Badge
      variant="outline"
      data-testid="sample-data-mark"
      className={cn(
        "pointer-events-none border-amber-500/50 bg-background/90 font-normal text-amber-700 dark:text-amber-400",
        className,
      )}
    >
      Sample data
    </Badge>
  );
}

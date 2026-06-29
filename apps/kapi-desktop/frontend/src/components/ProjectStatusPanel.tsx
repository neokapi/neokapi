import { useState } from "react";
import { Button } from "@neokapi/ui-primitives";
import { t } from "@neokapi/kapi-react/runtime";
import { CheckCircle2, Hammer } from "lucide-react";
import { TranslationStatusPanel, type ProjectStatus } from "./TranslationStatusPanel";
import { ConvergencePanel } from "./ConvergencePanel";
import type { ConvergenceReport, ReviewItem } from "../types/api";

export type ProjectStatusView = "working" | "ship";

export interface ProjectStatusPanelProps {
  tabID: string;
  /** Which stage to show first. Defaults to "ship" (the released-state view). */
  defaultView?: ProjectStatusView;
  /** Pre-loaded working-stage status for Storybook/tests — skips the Wails call. */
  status?: ProjectStatus;
  /** Pre-loaded ship-stage report for Storybook/tests — skips the Wails call. */
  report?: ConvergenceReport;
  /** Override the approve action (tests/Storybook); defaults to the Wails call. */
  onApprove?: (item: ReviewItem) => Promise<void>;
}

/**
 * The single project-status surface, with one toggle over the two stages of a
 * project's work:
 *
 * - **Working** — block-store coverage, pre-merge: what a process-only run has
 *   produced but not yet written to files ({@link TranslationStatusPanel},
 *   `GetProjectStatus`).
 * - **Ship** — the released state read from the files plus the committed state
 *   store: the convergence ladder, ship/source gates, and the review queue
 *   ({@link ConvergencePanel}, `GetConvergence`).
 *
 * Both stages keep their own data source; the toggle only chooses which one is
 * mounted. This is the desktop embodiment of the project store's two readings —
 * see the "project store" concept in the docs.
 */
export function ProjectStatusPanel({
  tabID,
  defaultView = "ship",
  status,
  report,
  onApprove,
}: ProjectStatusPanelProps) {
  const [view, setView] = useState<ProjectStatusView>(defaultView);

  return (
    <div className="space-y-3" data-slot="project-status-panel">
      <div className="flex items-center justify-between gap-2">
        <h3 className="text-sm font-medium">{t("Project status")}</h3>
        <div
          className="inline-flex rounded-md border border-border p-0.5"
          role="tablist"
          aria-label={t("Project status stage")}
          data-slot="project-status-toggle"
        >
          <StageButton
            active={view === "working"}
            onClick={() => setView("working")}
            icon={<Hammer size={12} />}
            label={t("Working")}
            value="working"
          />
          <StageButton
            active={view === "ship"}
            onClick={() => setView("ship")}
            icon={<CheckCircle2 size={12} />}
            label={t("Ship")}
            value="ship"
          />
        </div>
      </div>

      <p className="text-xs text-muted-foreground" data-slot="project-status-hint">
        {view === "working"
          ? t("Block-store coverage from runs, before merge writes the files.")
          : t("Released state from the files and the committed review decisions.")}
      </p>

      {view === "working" ? (
        <TranslationStatusPanel tabID={tabID} status={status} />
      ) : (
        <ConvergencePanel tabID={tabID} showTitle={false} report={report} onApprove={onApprove} />
      )}
    </div>
  );
}

function StageButton({
  active,
  onClick,
  icon,
  label,
  value,
}: {
  active: boolean;
  onClick: () => void;
  icon: React.ReactNode;
  label: string;
  value: ProjectStatusView;
}) {
  return (
    <Button
      variant={active ? "secondary" : "ghost"}
      size="xs"
      onClick={onClick}
      role="tab"
      aria-selected={active}
      data-slot="project-status-stage"
      data-value={value}
      data-active={active}
    >
      {icon}
      {label}
    </Button>
  );
}

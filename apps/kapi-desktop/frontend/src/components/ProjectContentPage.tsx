// Project Settings → Content: what the project holds and where it sits.
//
// The collections are the recipe's content declarations: their file patterns,
// formats, languages, per-language coverage and a Run per collection. The
// standing above them is `kapi status` in one table: the content and
// governance axes, and the coordinate points the content sits at.

import { PageContainer, PageHeader } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type {
  ConvergenceReport,
  FormatInfo,
  KapiProject,
  ProjectServer,
  ProjectStatus,
} from "../types/api";
import { CollectionsPanel, type RunFlowHandler } from "./CollectionsPanel";
import { ProjectStanding, type ProjectPointsResult } from "./ProjectStanding";
import type { ContextPin } from "./ContextHub";

export interface ProjectContentPageProps {
  tabID: string;
  project: KapiProject;
  displayName: string;
  onUpdate: (project: KapiProject) => void;
  onRunFlow?: RunFlowHandler;
  /** Open the Review surface narrowed to a (collection, locale) scope. */
  onOpenReview?: (scope?: { collection?: string; locale?: string }) => void;
  /** Open Context standing at a point on the map. */
  onOpenPoint?: (pin: ContextPin) => void;
  hasTargetLanguages: boolean;
  /** Pre-loaded reads for Storybook and tests, which reach no backend. */
  status?: ProjectStatus;
  points?: ProjectPointsResult;
  server?: ProjectServer;
  basePath?: string;
  formatList?: FormatInfo[];
  convergence?: ConvergenceReport;
}

export function ProjectContentPage({
  tabID,
  project,
  displayName,
  onUpdate,
  onRunFlow,
  onOpenReview,
  onOpenPoint,
  hasTargetLanguages,
  status,
  points,
  server,
  basePath,
  formatList,
  convergence,
}: ProjectContentPageProps) {
  return (
    <PageContainer width="wide" data-slot="project-content-page">
      <PageHeader help="project-settings" title={t("Content")} />
      <ProjectStanding
        tabID={tabID}
        project={project}
        displayName={displayName}
        onOpenPoint={onOpenPoint}
        hasTargetLanguages={hasTargetLanguages}
        status={status}
        points={points}
        server={server}
      />
      <CollectionsPanel
        project={project}
        onUpdate={onUpdate}
        tabID={tabID}
        flows={project.flows}
        onRunFlow={onRunFlow}
        onOpenReview={onOpenReview}
        hasTargetLanguages={hasTargetLanguages}
        status={status}
        basePath={basePath}
        formatList={formatList}
        convergence={convergence}
      />
    </PageContainer>
  );
}

// What the Overview draws under its counts: the notices that stop work on the
// project, and the language standing.
//
// A project that declares a target language gets the convergence hero, with
// Catch up and the pre-flight Plan… dialog. A project with none gets
// no language work at all: a source-only project is complete, and the page
// does not prompt for what it lacks. Review's empty state and Project
// Settings are where a first target language is added.

import { useCallback, useEffect, useState } from "react";
import { AlertTriangle, Loader2, Plug, RefreshCw, Settings2 } from "lucide-react";
import { Badge, Button } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import type { ConvergePlan, ConvergenceReport, PluginIssue } from "../types/api";
import { api, type SampleInfo } from "../hooks/useApi";
import { ConvergenceHero } from "./ConvergenceHero";

export interface OverviewStandingProps {
  tabID: string;
  /** Whether the saved recipe declares a target language. */
  hasTargetLanguages: boolean;
  onNavigate: (view: string) => void;
  /** Open the Review surface. */
  onOpenReview?: () => void;
  /** Launch the convergence run (the run action → runner passes view). */
  onBringUpToDate?: () => void;
  /** When false, plugin requirements are unmet and the notice shows. */
  pluginsResolved?: boolean;
  pluginIssues?: PluginIssue[];
  /** Refresh this sample to the version bundled with the current kapi. */
  onResetSample?: () => void;
  /** Pre-loaded sample info for Storybook, which skips api.getSampleInfo(). */
  sampleInfo?: SampleInfo;
  /** Pre-loaded convergence for Storybook and tests. */
  convergence?: ConvergenceReport;
  /** Pre-loaded pre-flight plan for Storybook and tests. */
  plan?: ConvergePlan;
}

export function OverviewStanding({
  tabID,
  hasTargetLanguages,
  onNavigate,
  onOpenReview,
  onBringUpToDate,
  pluginsResolved,
  pluginIssues,
  onResetSample,
  sampleInfo: givenSample,
  convergence,
  plan,
}: OverviewStandingProps) {
  return (
    <div data-slot="overview-standing" className="space-y-4">
      {pluginsResolved === false && pluginIssues && pluginIssues.length > 0 && (
        <PluginIssuesNotice issues={pluginIssues} onNavigate={onNavigate} />
      )}
      <SampleUpgradeNotice tabID={tabID} given={givenSample} onReset={onResetSample} />
      {hasTargetLanguages && (
        <div data-slot="language-standing">
          <ConvergenceHero
            tabID={tabID}
            onBringUpToDate={onBringUpToDate}
            convergence={convergence}
            plan={plan}
            onOpenSettings={onNavigate}
            onOpenReview={onOpenReview}
            onOpenChecks={() => onNavigate("checks")}
          />
        </div>
      )}
    </div>
  );
}

function PluginIssuesNotice({
  issues,
  onNavigate,
}: {
  issues: PluginIssue[];
  onNavigate: (view: string) => void;
}) {
  const [installing, setInstalling] = useState<string | null>(null);
  const install = useCallback((plugin: string) => {
    setInstalling(plugin);
    void api.installPlugin(plugin);
  }, []);
  return (
    <div data-slot="plugin-issues" className="rounded-lg border border-warning/30 bg-warning/5 p-4">
      <div className="flex items-start gap-3">
        <AlertTriangle size={16} className="mt-0.5 shrink-0 text-warning-ink" />
        <div className="flex-1">
          <p className="text-sm font-medium">{t("Plugin requirements not met")}</p>
          <p className="mt-1 text-xs text-muted-foreground">
            {t(
              "This project requires plugins that are not installed or have incompatible versions. Content and flow features are disabled until this is resolved.",
            )}
          </p>
          <ul className="mt-2 space-y-1">
            {issues.map((issue) => (
              <li key={issue.plugin} className="flex items-center gap-2 text-xs">
                <Badge variant="outline" className="text-[10px]">
                  {issue.plugin}
                </Badge>
                {issue.type === "missing" ? (
                  <>
                    <span className="text-muted-foreground">{t("not installed")}</span>
                    <Button
                      size="xs"
                      variant="outline"
                      className="ml-auto"
                      onClick={() => install(issue.plugin)}
                      disabled={installing === issue.plugin}
                    >
                      {installing === issue.plugin ? (
                        <Loader2 size={11} className="animate-spin" />
                      ) : (
                        <Plug size={11} />
                      )}
                      {t("Install")}
                    </Button>
                  </>
                ) : (
                  <span className="text-muted-foreground">
                    {t("requires {required}, installed {installed}", {
                      required: issue.required ?? "",
                      installed: issue.installed_version ?? "",
                    })}
                  </span>
                )}
              </li>
            ))}
          </ul>
          <div className="mt-3 flex gap-2">
            <Button size="sm" variant="outline" onClick={() => onNavigate("project-settings")}>
              <Settings2 size={12} />
              {t("Edit plugin settings")}
            </Button>
            <Button size="sm" variant="outline" onClick={() => onNavigate("app-settings")}>
              <Plug size={12} />
              {t("Manage plugins")}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}

/** The notice for a sample created by an earlier kapi, until it is reset or kept. */
function SampleUpgradeNotice({
  tabID,
  given,
  onReset,
}: {
  tabID: string;
  given?: SampleInfo;
  onReset?: () => void;
}) {
  const [info, setInfo] = useState<SampleInfo | null>(given ?? null);
  const [dismissed, setDismissed] = useState(false);
  useEffect(() => {
    if (given) return;
    void api
      .getSampleInfo(tabID)
      .then((res) => {
        if (res) setInfo(res);
      })
      .catch(() => {});
  }, [tabID, given]);
  if (!info?.upgrade_available || dismissed) return null;
  return (
    <div
      data-slot="sample-upgrade"
      className="rounded-lg border border-primary/30 bg-primary/5 p-4"
    >
      <div className="flex items-start gap-3">
        <RefreshCw size={16} className="mt-0.5 shrink-0 text-primary" />
        <div className="flex-1">
          <p className="text-sm font-medium">{t("A newer version of this sample is available")}</p>
          <p className="mt-1 text-xs text-muted-foreground">
            {t(
              "This sample was created by an earlier version of kapi. Reset it to get the latest content and configuration. Your current copy is backed up first.",
            )}
          </p>
          <div className="mt-3 flex gap-2">
            <Button size="sm" onClick={() => onReset?.()}>
              <RefreshCw size={12} />
              {t("Reset to latest")}
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                setDismissed(true);
                void api.acknowledgeSampleRevision(tabID);
              }}
            >
              {t("Keep current")}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}

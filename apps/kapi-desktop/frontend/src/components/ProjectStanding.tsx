// What the project stands at, on two axes, and where its content sits.
//
// The layout is `kapi status`'s: an identity line, then one labelled row per
// axis. Content is what has been extracted and what is missing; governance is
// the voice in force, the terms bound and the venue. A reader who learns the
// shape here reads the CLI's output without learning it twice.
//
// The point map below it is the recipe's declared coordinate space as a table.
// A single-point project gets one quiet row rather than no map: hiding it would
// make coordinates read as an advanced feature instead of the model the app is
// built on.

import { useQuery } from "@tanstack/react-query";
import { Compass, FileText, ShieldCheck, Cloud } from "lucide-react";
import { Badge, Card, CardContent, SimpleTooltip, cn } from "@neokapi/ui-primitives";
import { t } from "@neokapi/i18n-react/runtime";
import { call } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import type { KapiProject, ProjectStatus, ProjectServer } from "../types/api";

/** One point the recipe declares, and what governs there. */
export interface ProjectPoint {
  ref: string;
  label: string;
  profile?: string;
  channel?: string;
  coordinates?: Record<string, string>;
  default: boolean;
  collections: string[];
  voice?: string;
  voice_field?: string;
  termstore?: string;
  validity?: { from?: string; to?: string; state: string };
  fallback?: { profile: string; expired: boolean; boundary?: string; message: string };
}

export interface ProjectPointsResult {
  at: string;
  points: ProjectPoint[];
  notes?: string[];
}

export interface ProjectStandingProps {
  tabID: string;
  project: KapiProject;
  displayName: string;
  /** Pre-loaded for Storybook/tests. */
  status?: ProjectStatus;
  points?: ProjectPointsResult;
  server?: ProjectServer;
  /** Open Context standing at a point. */
  onOpenPoint?: (pin: { coordinate?: string; collection?: string }) => void;
}

/** One labelled axis of the standing. */
function Axis({
  label,
  icon,
  children,
}: {
  label: string;
  icon: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <div className="flex items-baseline gap-3 py-1">
      <span className="flex w-24 shrink-0 items-center gap-1.5 text-xs text-muted-foreground">
        {icon}
        {label}
      </span>
      <span className="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1 text-sm">
        {children}
      </span>
    </div>
  );
}

/** A dot-separated fact in an axis row. */
function Fact({ children, muted }: { children: React.ReactNode; muted?: boolean }) {
  return (
    <span className={cn(muted && "text-muted-foreground")} data-testid="standing-fact">
      {children}
    </span>
  );
}

export function ProjectStanding({
  tabID,
  project,
  displayName,
  status: propStatus,
  points: propPoints,
  server: propServer,
  onOpenPoint,
}: ProjectStandingProps) {
  const statusQuery = useQuery({
    queryKey: qk.projectStatus(tabID),
    queryFn: () => call<ProjectStatus>("GetProjectStatus", tabID),
    enabled: !propStatus && !!tabID,
  });
  const pointsQuery = useQuery({
    queryKey: qk.projectPoints(tabID),
    queryFn: () => call<ProjectPointsResult>("ProjectPoints", tabID),
    enabled: !propPoints && !!tabID,
  });
  const serverQuery = useQuery({
    queryKey: qk.projectServer(tabID),
    queryFn: () => call<ProjectServer>("GetProjectServer", tabID),
    enabled: !propServer && !!tabID,
  });

  const status = propStatus ?? statusQuery.data ?? undefined;
  const points = propPoints ?? pointsQuery.data ?? undefined;
  const server = propServer ?? serverQuery.data ?? undefined;

  const targets = project.defaults?.target_languages ?? [];
  const blocks = (status?.collections ?? []).reduce((n, c) => n + (c.blockCount ?? 0), 0);
  const collections = status?.collections?.length ?? project.collections?.length ?? 0;

  // The voice governing the project's own point, which is what a reader means
  // by "the project's voice" before they look at the map.
  const rootPoint = points?.points.find((p) => p.default);
  const bound = points?.points.filter((p) => p.voice).length ?? 0;

  return (
    <div className="mb-6 space-y-3" data-testid="project-standing">
      <Card>
        <CardContent className="p-4">
          <div className="mb-2 flex flex-wrap items-baseline gap-x-2 gap-y-1 text-xs text-muted-foreground">
            <span className="font-medium text-foreground">{displayName}</span>
            {server?.stream && (
              <>
                <span>·</span>
                <span>{t("stream {name}", { name: server.stream })}</span>
              </>
            )}
            {server?.connected && server.host && (
              <>
                <span>·</span>
                <span>{t("venue {host}", { host: server.host })}</span>
              </>
            )}
          </div>

          <Axis label={t("content")} icon={<FileText size={12} />}>
            {status?.hasData ? (
              <>
                <Fact>{t("{count} unit(s) extracted", { count: blocks })}</Fact>
                <Fact muted>·</Fact>
                <Fact muted>{t("{count} collection(s)", { count: collections })}</Fact>
                {status.stale && (
                  <>
                    <Fact muted>·</Fact>
                    <Badge
                      variant="outline"
                      className="border-amber-500/40 font-normal text-amber-700 dark:text-amber-500"
                    >
                      {t("re-extract to refresh the counts")}
                    </Badge>
                  </>
                )}
              </>
            ) : (
              <Fact muted>
                {t("{count} collection(s), nothing extracted yet", { count: collections })}
              </Fact>
            )}
          </Axis>

          <Axis label={t("governance")} icon={<ShieldCheck size={12} />}>
            {rootPoint?.voice ? (
              <SimpleTooltip content={rootPoint.voice_field ?? ""}>
                <span data-testid="standing-voice">
                  {t("voice {name}", { name: rootPoint.voice })}
                </span>
              </SimpleTooltip>
            ) : (
              <span className="text-muted-foreground" data-testid="standing-voice">
                {t("no voice bound")}
              </span>
            )}
            <Fact muted>·</Fact>
            {rootPoint?.termstore ? (
              <Fact>{t("terms {source}", { source: rootPoint.termstore })}</Fact>
            ) : (
              <Fact muted>{t("no terms bound")}</Fact>
            )}
            <Fact muted>·</Fact>
            <Fact muted>
              {t("{count} of {total} point(s) governed", {
                count: bound,
                total: points?.points.length ?? 0,
              })}
            </Fact>
          </Axis>

          {targets.length > 0 && (
            <Axis label={t("languages")} icon={<Cloud size={12} />}>
              <Fact muted>{targets.join(", ")}</Fact>
            </Axis>
          )}
        </CardContent>
      </Card>

      {points && points.points.length > 0 && (
        <Card>
          <CardContent className="p-4">
            <h2 className="mb-2 flex items-center gap-1.5 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
              <Compass size={12} />
              {t("Where content sits")}
            </h2>
            <ul className="divide-y" data-testid="point-map">
              {points.points.map((p) => (
                <li key={p.ref || "__default__"}>
                  <button
                    type="button"
                    disabled={!onOpenPoint}
                    onClick={() =>
                      onOpenPoint?.({
                        coordinate: p.ref || undefined,
                        collection: p.collections[0],
                      })
                    }
                    className={cn(
                      "flex w-full flex-wrap items-baseline gap-x-3 gap-y-1 py-2 text-left text-sm",
                      onOpenPoint && "hover:bg-accent/40",
                    )}
                    data-testid="point-row"
                  >
                    <span className="min-w-[9rem] font-medium">{p.label}</span>
                    {p.coordinates && (
                      <span className="font-mono text-[11px] text-muted-foreground">
                        {Object.entries(p.coordinates)
                          .map(([axis, value]) => `${axis}:${value}`)
                          .join(" · ")}
                      </span>
                    )}
                    <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">
                      {p.collections.length > 0
                        ? p.collections.join(", ")
                        : t("no collection sits here")}
                    </span>
                    {p.fallback && (
                      <Badge
                        variant="outline"
                        className="border-destructive/40 font-normal text-destructive"
                      >
                        {t("fell through")}
                      </Badge>
                    )}
                    {p.voice ? (
                      <Badge variant="secondary" className="font-normal">
                        {p.voice}
                      </Badge>
                    ) : (
                      <span className="text-xs text-muted-foreground">{t("no voice")}</span>
                    )}
                  </button>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      )}
    </div>
  );
}

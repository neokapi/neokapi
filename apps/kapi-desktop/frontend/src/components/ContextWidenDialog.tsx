// What widening a rule would newly govern, read before anybody accepts it.
//
// Widening to the workspace puts the rule in force in every project this
// machine account works on, so the dialog lists the other projects by name.
// Widening past an axis keeps the rule in its project and stops it being
// specific about that axis. Either way the dialog lists the declared points
// the rule would newly answer at and, wherever a projection of the content is
// built on this machine, the units it would newly match.
//
// Coverage is stated, never implied. The backend says which projects it read
// units from and which it could not, with the reason, and the dialog repeats
// that in as many words, so an empty list reads as "not read" rather than
// "none".

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { t } from "@neokapi/i18n-react/runtime";
import {
  Badge,
  Button,
  CoordinateChip,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  ErrorNotice,
  LoadingSpinner,
} from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import type { ContextFeedEntry, ContextWidenPreview, DigestItem } from "../types/api";

/** The heading above each part of the preview. */
const LABEL = "text-xs font-medium uppercase tracking-wider text-muted-foreground";

/** How many matched units the dialog lists before it says how many more there are. */
const UNITS_SHOWN = 12;

/**
 * A rule in force as the widen preview needs it. The activity feed and the
 * rules view each hold the rule in their own shape, and both widen through
 * the same backend call.
 */
export interface ContextWidenRule {
  /** The project's workspace key. */
  project_key: string;
  id: string;
  /** The term the rule is about, empty for a rule that names none. */
  term: string;
  /** How far the rule answers now, as the log prints it. */
  scope: string;
}

/** The rule a feed entry holds. */
export function widenRuleOfEntry(entry: ContextFeedEntry): ContextWidenRule {
  return {
    project_key: entry.project_key,
    id: entry.id,
    term: entry.subject.term ?? "",
    scope: entry.scope.describe,
  };
}

/** The rule a digest item holds, in the project the digest is about. */
export function widenRuleOfDigestItem(project: string, item: DigestItem): ContextWidenRule {
  return {
    project_key: project,
    id: item.id,
    term: item.subject.term?.term ?? "",
    scope: item.scope,
  };
}

export interface ContextWidenDialogProps {
  /** The rule being widened, null when the dialog is closed. */
  rule: ContextWidenRule | null;
  /** "workspace", or the axis the rule would stop being specific about. */
  to: string;
  onClose: () => void;
  onConfirm: (rule: ContextWidenRule, to: string) => Promise<void> | void;
  /** Pre-loaded reach for Storybook and tests, which reach no backend. */
  preview?: ContextWidenPreview;
}

export function ContextWidenDialog({
  rule,
  to,
  onClose,
  onConfirm,
  preview,
}: ContextWidenDialogProps) {
  const [widening, setWidening] = useState(false);
  const reachQuery = useQuery({
    queryKey: qk.contextWidenReach(rule?.project_key ?? "", rule?.id ?? "", to),
    queryFn: () => api.contextWidenReach(rule?.project_key ?? "", rule?.id ?? "", to),
    enabled: !!rule && !!to && !preview,
  });
  if (!rule) return null;
  const reach = preview ?? reachQuery.data ?? null;

  const confirm = () => {
    setWidening(true);
    void Promise.resolve(onConfirm(rule, to)).finally(() => {
      setWidening(false);
      onClose();
    });
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-lg" data-slot="widen-dialog">
        <DialogHeader>
          <DialogTitle>
            {to === "workspace" ? (
              <>
                Put <span translate="no">{rule.term}</span> in force everywhere?
              </>
            ) : (
              <>
                Stop <span translate="no">{rule.term}</span> being specific about{" "}
                <span translate="no">{to}</span>?
              </>
            )}
          </DialogTitle>
          <DialogDescription>
            {to === "workspace"
              ? "The rule answers in every project in your workspace, beneath whatever each project declares for itself."
              : "The rule stays in this project and answers at every point that differs only in this axis."}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3 text-sm">
          <div>
            <div className={LABEL}>Now</div>
            <div className="mt-1 font-mono text-xs" translate="no">
              {rule.scope}
            </div>
          </div>
          {reach && (
            <div>
              <div className={LABEL}>After</div>
              <div className="mt-1 font-mono text-xs" translate="no">
                {reach.scope.describe}
              </div>
            </div>
          )}

          {reachQuery.error ? (
            <ErrorNotice error={reachQuery.error} title="The rule's reach could not be read" />
          ) : !reach ? (
            <LoadingSpinner />
          ) : (
            <>
              {to === "workspace" && <ProjectReach preview={reach} />}
              <PointReach preview={reach} rule={rule} />
              <UnitReach preview={reach} />
              <Coverage preview={reach} />
            </>
          )}
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={onClose} disabled={widening}>
            Cancel
          </Button>
          <Button
            onClick={confirm}
            disabled={widening}
            data-slot="widen-confirm"
            aria-label={t("Widen this rule")}
          >
            Widen the rule
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** The other projects a workspace-wide rule would newly answer in. */
function ProjectReach({ preview }: { preview: ContextWidenPreview }) {
  const n = preview.projects.length;
  return (
    <div>
      <div className={LABEL}>
        {n === 0
          ? "No other project is in your workspace yet"
          : `It would newly answer in ${n} other ${n === 1 ? "project" : "projects"}`}
      </div>
      <ul className="mt-1 space-y-1" data-slot="widen-projects">
        {preview.projects.map((project) => (
          <li key={project.project_key} className="flex items-center gap-2 text-xs">
            <span translate="no">{project.project_name || project.project_key}</span>
            {!project.checked_out && (
              <span className="text-muted-foreground">no copy on this machine</span>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}

/** The declared points a widened rule would newly answer at. */
function PointReach({ preview, rule }: { preview: ContextWidenPreview; rule: ContextWidenRule }) {
  if (preview.points.length === 0) {
    return (
      <p className="text-xs text-muted-foreground" data-slot="widen-points">
        {preview.to === "workspace"
          ? "No recipe on this machine declares another point the rule would newly answer at."
          : "The recipe declares no other point that differs only in this axis, so the rule answers where it already does."}
      </p>
    );
  }
  return (
    <div>
      <div className={LABEL}>
        It would newly answer at {preview.points.length}{" "}
        {preview.points.length === 1 ? "point" : "points"}
      </div>
      <ul className="mt-1 space-y-1.5" data-slot="widen-points">
        {preview.points.map((point) => (
          <li key={`${point.project_key}:${point.ref || point.label}`}>
            <div className="flex flex-wrap items-center gap-1">
              {point.project_key !== rule.project_key && (
                <Badge variant="outline" className="text-muted-foreground" translate="no">
                  {point.project_key}
                </Badge>
              )}
              <span className="font-mono text-xs" translate="no">
                {point.label}
              </span>
              {Object.entries(point.coordinates ?? {}).map(([axis, value]) => (
                <CoordinateChip key={axis} axis={axis} value={value} />
              ))}
            </div>
            {point.collections.length > 0 && (
              <div className="text-[11px] text-muted-foreground" translate="no">
                {point.collections.join(", ")}
              </div>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}

/** The units a widened rule would newly match, where a projection was read. */
function UnitReach({ preview }: { preview: ContextWidenPreview }) {
  const matched = preview.coverage.examined.reduce((n, e) => n + e.matched, 0);
  if (preview.coverage.examined.length === 0) return null;
  const shown = preview.units.slice(0, UNITS_SHOWN);
  const more = matched - shown.length;
  return (
    <div>
      <div className={LABEL}>
        {matched === 0
          ? "No unit would newly match"
          : `It would newly match ${matched} ${matched === 1 ? "unit" : "units"}`}
      </div>
      {shown.length > 0 && (
        <ul className="mt-1 space-y-1" data-slot="widen-units">
          {shown.map((unit) => (
            <li key={`${unit.project_key}:${unit.document}#${unit.unit}`} className="text-xs">
              <span className="font-mono" translate="no">
                {unit.document} · {unit.unit}
              </span>
              <div className="line-clamp-2 text-[11px] text-muted-foreground" translate="no">
                {unit.text}
              </div>
            </li>
          ))}
        </ul>
      )}
      {more > 0 && <p className="mt-1 text-xs text-muted-foreground">and {more} more</p>}
    </div>
  );
}

/** What the units were read from and what they were not, in the backend's words. */
function Coverage({ preview }: { preview: ContextWidenPreview }) {
  const { examined, not_examined } = preview.coverage;
  const read = examined.map(
    (e) => `${e.project_name || e.project_key} (${e.units} ${e.units === 1 ? "unit" : "units"})`,
  );
  return (
    <p data-slot="widen-coverage" className="text-xs text-muted-foreground">
      {read.length > 0 ? `Units were read from ${read.join(", ")}.` : "No units were read."}
      {not_examined.map((gap) => (
        <span key={gap.project_key}>
          {" "}
          <span translate="no">{gap.project_name || gap.project_key}</span>: {gap.reason}.
        </span>
      ))}
    </p>
  );
}

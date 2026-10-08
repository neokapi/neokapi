// What widening a rule would reach, read before anybody accepts it.
//
// Widening to the workspace puts the rule in force in every project this
// machine account works on, so the dialog lists them by name. Widening past an
// axis keeps the rule in its project and stops it being specific about that
// axis, so the dialog lists the points in the recipe the rule would newly
// answer at.
//
// Reach is not impact. Which files hold the term, and how many times, is not
// computed: the host API offers no such preview, and a count invented here
// would be a second answer about content that the engine never gave.

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
          ) : to === "workspace" ? (
            <ProjectReach preview={reach} />
          ) : (
            <PointReach preview={reach} />
          )}

          <p data-slot="widen-impact" className="text-xs text-muted-foreground">
            This is where the rule would answer. How much content it touches is not counted here;
            run a check afterwards to see what it reports.
          </p>
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

/** Every project a workspace-wide rule would answer in. */
function ProjectReach({ preview }: { preview: ContextWidenPreview }) {
  const others = preview.projects.filter((p) => !p.current);
  return (
    <div>
      <div className={LABEL}>
        {others.length === 0
          ? "No other project is in your workspace yet"
          : `It would newly answer in ${others.length} other ${others.length === 1 ? "project" : "projects"}`}
      </div>
      <ul className="mt-1 space-y-1" data-slot="widen-projects">
        {preview.projects.map((project) => (
          <li key={project.project_key} className="flex items-center gap-2 text-xs">
            <span translate="no">{project.project_name || project.project_key}</span>
            {project.current && (
              <Badge variant="outline" className="text-muted-foreground">
                Already
              </Badge>
            )}
            {!project.checked_out && (
              <span className="text-muted-foreground">no copy on this machine</span>
            )}
          </li>
        ))}
      </ul>
    </div>
  );
}

/** The recipe's points a widened rule would newly answer at. */
function PointReach({ preview }: { preview: ContextWidenPreview }) {
  if (preview.points.length === 0) {
    return (
      <p className="text-xs text-muted-foreground" data-slot="widen-points">
        The recipe declares no other point that differs only in this axis, so the rule answers where
        it already does.
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
          <li key={point.ref || point.label}>
            <div className="flex flex-wrap items-center gap-1">
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

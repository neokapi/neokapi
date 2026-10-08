// The confirmation for resetting a project's context to before a session.
//
// A reset sets aside every suggestion and rule recorded from the start of the
// session on, and every decision made about them since. The project's terms and
// content memory are rebuilt without them. The history keeps all of it, and a
// later reset to before this one brings it back. The confirmation names how
// many suggestions and rules go and what they were about, because a session can
// hold more than a reader remembers agreeing to.

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { t } from "@neokapi/i18n-react/runtime";
import {
  Button,
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
import type { ContextResetRequest, ContextResetSummary } from "../types/api";

export interface ContextResetDialogProps {
  /** What to reset to before, null when the dialog is closed. */
  request: ContextResetRequest | null;
  onClose: () => void;
  onConfirm: (request: ContextResetRequest) => Promise<void> | void;
  /** Pre-loaded scope for Storybook and tests, which reach no backend. */
  scope?: ContextResetSummary;
}

export function ContextResetDialog({
  request,
  onClose,
  onConfirm,
  scope,
}: ContextResetDialogProps) {
  const [resetting, setResetting] = useState(false);
  const scopeQuery = useQuery({
    queryKey: qk.contextResetScope(request?.project ?? "", request?.before ?? ""),
    queryFn: () => api.contextResetScope(request as ContextResetRequest),
    enabled: !!request && !scope,
  });
  if (!request) return null;
  const plan = scope ?? scopeQuery.data ?? null;

  const confirm = () => {
    setResetting(true);
    void Promise.resolve(onConfirm(request)).finally(() => {
      setResetting(false);
      onClose();
    });
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-lg" data-slot="reset-dialog">
        <DialogHeader>
          <DialogTitle>Reset to before this session?</DialogTitle>
          <DialogDescription>
            Every suggestion and rule recorded from the start of this session on is set aside, with
            every decision made about them since. The project's terms and content memory are rebuilt
            without them.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3 text-sm">
          {scopeQuery.error ? (
            <ErrorNotice
              error={scopeQuery.error}
              title="What this would set aside could not be read"
            />
          ) : !plan ? (
            <LoadingSpinner />
          ) : (
            <>
              <p data-slot="reset-count">
                {plan.set_aside === 1
                  ? "This sets aside 1 suggestion or rule."
                  : `This sets aside ${plan.set_aside} suggestions and rules.`}
                {plan.rules.length > 0 &&
                  ` ${plan.rules.length === 1 ? "1 rule is" : `${plan.rules.length} rules are`} in force and would be taken back out.`}
                {plan.decisions > 0 &&
                  ` ${plan.decisions === 1 ? "1 later decision is" : `${plan.decisions} later decisions are`} set aside with them.`}
              </p>
              {plan.subjects.length > 0 && (
                <ul className="space-y-0.5" data-slot="reset-subjects">
                  {plan.subjects.map((subject, i) => (
                    <li key={`${subject}:${i}`} className="font-mono text-xs" translate="no">
                      {subject}
                    </li>
                  ))}
                </ul>
              )}
              {plan.restored > 0 && (
                <p data-slot="reset-restored" className="text-xs text-muted-foreground">
                  {plan.restored === 1
                    ? "1 suggestion or rule an earlier reset set aside answers again."
                    : `${plan.restored} suggestions and rules an earlier reset set aside answer again.`}
                </p>
              )}
            </>
          )}
          <p className="text-xs text-muted-foreground">
            The history keeps everything set aside. A later reset to before this one brings it back.
          </p>
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={onClose} disabled={resetting}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={confirm}
            disabled={resetting}
            data-slot="reset-confirm"
            aria-label={t("Reset to before this session")}
          >
            Reset
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

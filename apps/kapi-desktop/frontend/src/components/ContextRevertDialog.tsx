// The confirmation for undoing recorded work.
//
// Undoing one rule takes it back out of the project's stores. Undoing a
// session does that for everything of the session a person kept and stops every
// candidate it recorded from advising. Either way the confirmation names how
// many operations go and what they were about, because a session can hold more
// than a reader remembers agreeing to.

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
import type { ContextRevertRequest, ContextRevertSummary } from "../types/api";

export interface ContextRevertDialogProps {
  /** What to undo, null when the dialog is closed. */
  request: ContextRevertRequest | null;
  onClose: () => void;
  onConfirm: (request: ContextRevertRequest) => Promise<void> | void;
  /** Pre-loaded scope for Storybook and tests, which reach no backend. */
  scope?: ContextRevertSummary;
}

export function ContextRevertDialog({
  request,
  onClose,
  onConfirm,
  scope,
}: ContextRevertDialogProps) {
  const [reverting, setReverting] = useState(false);
  const scopeQuery = useQuery({
    queryKey: qk.contextRevertScope(
      request?.project ?? "",
      request?.id ?? "",
      request?.session ?? "",
    ),
    queryFn: () => api.contextRevertScope(request as ContextRevertRequest),
    enabled: !!request && !scope,
  });
  if (!request) return null;
  const plan = scope ?? scopeQuery.data ?? null;
  const session = !!request.session;

  const confirm = () => {
    setReverting(true);
    void Promise.resolve(onConfirm(request)).finally(() => {
      setReverting(false);
      onClose();
    });
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-lg" data-slot="revert-dialog">
        <DialogHeader>
          <DialogTitle>
            {session ? "Undo everything this session recorded?" : "Take this rule back out?"}
          </DialogTitle>
          <DialogDescription>
            {session
              ? "Every rule of the session a person kept is removed from this project's terms and content memory, and every suggestion it recorded stops advising."
              : "The rule is removed from the project's stores and from the committed source the recipe binds. It stops answering at once."}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3 text-sm">
          {scopeQuery.error ? (
            <ErrorNotice error={scopeQuery.error} title="What this would undo could not be read" />
          ) : !plan ? (
            <LoadingSpinner />
          ) : (
            <>
              <p data-slot="revert-count">
                {plan.operations === 1
                  ? "This undoes 1 operation."
                  : `This undoes ${plan.operations} operations.`}
                {plan.rules.length > 0 &&
                  ` ${plan.rules.length === 1 ? "1 rule is" : `${plan.rules.length} rules are`} in force and would be removed.`}
              </p>
              {plan.subjects.length > 0 && (
                <ul className="space-y-0.5" data-slot="revert-subjects">
                  {plan.subjects.map((subject, i) => (
                    <li key={`${subject}:${i}`} className="font-mono text-xs" translate="no">
                      {subject}
                    </li>
                  ))}
                </ul>
              )}
            </>
          )}
          <p className="text-xs text-muted-foreground">
            The history keeps every operation. Undoing appends to it rather than erasing it.
          </p>
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={onClose} disabled={reverting}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={confirm}
            disabled={reverting}
            data-slot="revert-confirm"
            aria-label={session ? t("Undo this session") : t("Undo this rule")}
          >
            {session ? "Undo the session" : "Undo the rule"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

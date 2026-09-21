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
} from "@neokapi/ui-primitives";
import { api } from "../hooks/useApi";
import { qk } from "../lib/queryKeys";
import { useShortenHome } from "../hooks/useShortenHome";
import type { WorkspaceProject, WorkspaceRemoval } from "../types/api";

interface RemoveProjectDialogProps {
  project: WorkspaceProject | null;
  onClose: () => void;
  onConfirm: (key: string) => Promise<void> | void;
  /** Pre-loaded plan for Storybook and tests, which reach no backend. */
  removal?: WorkspaceRemoval;
}

/**
 * The confirmation for removing a project from the workspace.
 *
 * It names what goes: the project, the file its context is kept in, and the
 * kinds of context inside it. It also names what stays, because the checkouts
 * on disk are the part a reader is most likely to fear for.
 */
export function RemoveProjectDialog({
  project,
  onClose,
  onConfirm,
  removal,
}: RemoveProjectDialogProps) {
  const shortenHome = useShortenHome();
  const [removing, setRemoving] = useState(false);
  const planQuery = useQuery({
    queryKey: qk.workspaceRemoval(project?.key ?? ""),
    queryFn: () => api.workspaceRemovalFor(project?.key ?? ""),
    enabled: !!project && !removal,
  });
  if (!project) return null;
  const plan = removal ?? planQuery.data ?? null;
  const checkouts = plan?.checkouts ?? project.checkouts.map((c) => c.path);

  const confirm = () => {
    setRemoving(true);
    void Promise.resolve(onConfirm(project.key)).finally(() => {
      setRemoving(false);
      onClose();
    });
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Remove {project.name} from your workspace?</DialogTitle>
          <DialogDescription>
            This deletes the context Kapi holds for this project: its terms, its voice profiles, its
            content memory and every decision recorded against it. It cannot be undone.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3 text-sm">
          {plan?.store && (
            <div>
              <div className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
                Deleted
              </div>
              <div className="mt-1 break-all font-mono text-xs">{shortenHome(plan.store)}</div>
            </div>
          )}
          <div>
            <div className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
              Kept
            </div>
            {checkouts.length > 0 ? (
              <>
                <p className="mt-1 text-xs text-muted-foreground">
                  Your files stay exactly where they are:
                </p>
                <ul className="mt-1 space-y-0.5">
                  {checkouts.map((path) => (
                    <li key={path} className="break-all font-mono text-xs">
                      {shortenHome(path)}
                    </li>
                  ))}
                </ul>
              </>
            ) : (
              <p className="mt-1 text-xs text-muted-foreground">
                No copy of this project is on this machine, so nothing on disk changes.
              </p>
            )}
          </div>
          <p className="text-xs text-muted-foreground">
            Running Kapi in one of these folders again registers the project afresh, with an empty
            context.
          </p>
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={onClose} disabled={removing}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={confirm}
            disabled={removing}
            aria-label={t("Remove this project and its context")}
          >
            Remove project and context
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

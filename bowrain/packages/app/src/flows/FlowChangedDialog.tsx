import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@neokapi/ui";
import type { FlowDefinitionInfo } from "@neokapi/ui";
import { toolDisplayName, type FlowToolName } from "./flowGraph";

export interface FlowChangedState {
  /** The flow as the server now holds it. */
  current: FlowDefinitionInfo;
  /** The person's version, which the server refused. */
  mine: FlowDefinitionInfo;
}

export interface FlowChangedDialogProps {
  state: FlowChangedState | null;
  tools: FlowToolName[];
  /** Reload the flow as it now stands, dropping the refused version. */
  onKeep: () => void;
  /** Save the person's version over the current one. */
  onSaveMine: () => void;
}

function Steps({
  flow,
  tools,
  testId,
}: {
  flow: FlowDefinitionInfo;
  tools: FlowToolName[];
  testId: string;
}) {
  const steps = flow.nodes.filter((n) => n.type === "tool");
  return (
    <ol className="list-decimal space-y-0.5 pl-5 text-sm" data-testid={testId}>
      {steps.map((n) => (
        <li key={n.id}>{n.label || toolDisplayName(n.name, tools)}</li>
      ))}
    </ol>
  );
}

/**
 * Asks what to do when a flow save meets a flow someone else saved after it
 * was opened: the server refused the save, and the dialog shows the flow as it
 * stands and the person's version, so they keep the current flow or save
 * theirs over it. It follows the translation editor's StaleChangeDialog.
 */
export function FlowChangedDialog({ state, tools, onKeep, onSaveMine }: FlowChangedDialogProps) {
  return (
    <Dialog open={state !== null} onOpenChange={(open) => !open && onKeep()}>
      <DialogContent className="sm:max-w-[560px]" data-testid="flow-changed-dialog">
        <DialogHeader>
          <DialogTitle>This flow changed since you opened it</DialogTitle>
          <DialogDescription>
            Someone saved it after you opened it, so your change was not saved.
          </DialogDescription>
        </DialogHeader>
        {state && (
          <div className="grid gap-4 sm:grid-cols-2">
            <section className="space-y-1">
              <h3 className="text-xs font-medium text-muted-foreground">
                {state.current.name} now
              </h3>
              <Steps flow={state.current} tools={tools} testId="flow-changed-current" />
            </section>
            <section className="space-y-1">
              <h3 className="text-xs font-medium text-muted-foreground">Your version</h3>
              <Steps flow={state.mine} tools={tools} testId="flow-changed-mine" />
            </section>
          </div>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={onKeep} data-testid="flow-changed-keep">
            Keep the current flow
          </Button>
          <Button onClick={onSaveMine} data-testid="flow-changed-save-mine">
            Save my version
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

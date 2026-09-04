// The project's flow list: the shared outcome-first card per flow, the empty
// state, and the New flow dialog. Built-in flows are read-only and offer a
// copy; project flows can be deleted from the card.

import { useState } from "react";
import { Plus } from "lucide-react";
import {
  Button,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  ErrorNotice,
  FlowCard,
  FlowsEmptyState,
  Input,
  Label,
  PageHeader,
} from "@neokapi/ui";
import type { FlowDefinitionInfo, ToolInfo } from "@neokapi/ui";
import { flowStepNames } from "./flowGraph";

export interface ProjectFlowListProps {
  flows: FlowDefinitionInfo[];
  tools: ToolInfo[];
  loading?: boolean;
  /** A load or action failure to show above the list. */
  error?: unknown;
  onOpen: (flow: FlowDefinitionInfo) => void;
  /** Creates the flow; a rejection is shown in the dialog. */
  onCreate: (name: string) => Promise<void>;
  onCopy: (flow: FlowDefinitionInfo) => void;
  onDelete: (flow: FlowDefinitionInfo) => void;
}

export function ProjectFlowList({
  flows,
  tools,
  loading,
  error,
  onOpen,
  onCreate,
  onCopy,
  onDelete,
}: ProjectFlowListProps) {
  const [creating, setCreating] = useState(false);

  return (
    <div data-testid="flow-list">
      <PageHeader
        title="Flows"
        subtitle="A flow runs on the server over content from any connector. Rules pick a flow to run."
        actions={
          <Button size="sm" data-testid="new-flow-btn" onClick={() => setCreating(true)}>
            <Plus size={12} />
            New flow
          </Button>
        }
      />

      {error != null && <ErrorNotice error={error} title="Could not load flows" className="mb-4" />}

      {(loading || flows.length > 0) && (
        <div className="grid grid-cols-1 gap-2 md:grid-cols-2">
          {loading
            ? [0, 1, 2].map((i) => <FlowCard key={i} loading />)
            : flows.map((flow) => {
                const steps = flowStepNames(flow, tools);
                const builtIn = flow.source === "built-in";
                return (
                  <div key={flow.id} data-testid={`flow-item-${flow.id}`}>
                    <FlowCard
                      item={{
                        id: flow.id,
                        name: flow.name,
                        description: flow.description,
                        steps,
                        stepCount: steps.length,
                        source: flow.source,
                      }}
                      onClick={() => onOpen(flow)}
                      onCopy={builtIn ? () => onCopy(flow) : undefined}
                      onDelete={builtIn ? undefined : () => onDelete(flow)}
                    />
                  </div>
                );
              })}
        </div>
      )}

      {!loading && flows.length === 0 && (
        <FlowsEmptyState
          projectMode
          title="No flows yet"
          description="Add a flow to give this project its own sequence of steps."
          onCreate={() => setCreating(true)}
        />
      )}

      <NewFlowDialog open={creating} onOpenChange={setCreating} onCreate={onCreate} />
    </div>
  );
}

function NewFlowDialog({
  open,
  onOpenChange,
  onCreate,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreate: (name: string) => Promise<void>;
}) {
  const [name, setName] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<unknown>(null);

  const close = () => {
    onOpenChange(false);
    setName("");
    setError(null);
  };

  const submit = async () => {
    const trimmed = name.trim();
    if (!trimmed || pending) return;
    setPending(true);
    setError(null);
    try {
      await onCreate(trimmed);
      close();
    } catch (e) {
      setError(e);
    } finally {
      setPending(false);
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) close();
      }}
    >
      <DialogContent showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>New flow</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="new-flow-name" className="text-xs text-muted-foreground">
            Flow name
          </Label>
          <Input
            id="new-flow-name"
            data-testid="new-flow-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") void submit();
            }}
            placeholder="Translate and check"
            autoFocus
          />
          <p className="text-[11px] text-muted-foreground">
            You can start from a template in the editor.
          </p>
          {error != null && (
            <ErrorNotice error={error} title="Could not create the flow" variant="inline" />
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" size="sm" onClick={close}>
            Cancel
          </Button>
          <Button
            size="sm"
            data-testid="create-flow-btn"
            onClick={() => void submit()}
            disabled={!name.trim() || pending}
          >
            {pending ? "Creating..." : "Create"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

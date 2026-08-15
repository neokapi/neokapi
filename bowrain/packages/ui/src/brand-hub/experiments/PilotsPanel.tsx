// Pilots panel (AD-021): bind a change-set to a project's content stream so
// real content and real checks resolve through the draft before it merges — a
// what-if exercised on a slice of live content. Lists active pilots and stops
// them; new pilots pick a project + stream.
import { useState } from "react";
import { ErrorNotice } from "../../errors";
import {
  Badge,
  Button,
  Card,
  CardContent,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@neokapi/ui-primitives";
import { Plus, Trash2, FlaskConical } from "../../components/icons";
import type { ChangeSetDetail, Pilot } from "../../types/brand-graph";
import { TERMINAL_CHANGESET_STATUSES } from "../../types/brand-graph";
import { useAddPilot, useRemovePilot } from "../../hooks/useChangesetsApi";
import { useProjects } from "../../hooks/useProjectApi";
import { formatRelative } from "../shell/atoms";
import { TrialPanel, TrialToggle } from "./TrialPanel";

export interface PilotsPanelProps {
  changeset: ChangeSetDetail;
}

export function PilotsPanel({ changeset }: PilotsPanelProps) {
  const remove = useRemovePilot(changeset.id);
  const { data: projects } = useProjects();
  const [addOpen, setAddOpen] = useState(false);
  const terminal = TERMINAL_CHANGESET_STATUSES.has(changeset.status);

  return (
    <Card>
      <CardContent className="space-y-3 p-4">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-medium">Pilots</h3>
          {!terminal && (
            <Button size="sm" variant="outline" onClick={() => setAddOpen(true)}>
              <Plus />
              Pilot
            </Button>
          )}
        </div>

        {changeset.pilots.length === 0 ? (
          <div className="flex flex-col items-center gap-1.5 rounded-lg border border-dashed bg-muted/20 px-4 py-6 text-center">
            <FlaskConical className="size-5 text-muted-foreground" />
            <p className="max-w-xs text-xs text-muted-foreground">
              Bind this change to a content stream to exercise it on real content before merge.
            </p>
          </div>
        ) : (
          <ul className="space-y-1.5">
            {changeset.pilots.map((p) => (
              <PilotRow
                key={`${p.project_id}/${p.stream}`}
                changesetId={changeset.id}
                pilot={p}
                projectName={projects?.find((x) => x.id === p.project_id)?.name}
                onRemove={() => remove.mutate({ projectId: p.project_id, stream: p.stream })}
                removing={
                  remove.isPending &&
                  remove.variables?.projectId === p.project_id &&
                  remove.variables?.stream === p.stream
                }
              />
            ))}
          </ul>
        )}
        <AddPilotDialog changesetId={changeset.id} open={addOpen} onOpenChange={setAddOpen} />
      </CardContent>
    </Card>
  );
}

function PilotRow({
  changesetId,
  pilot,
  projectName,
  onRemove,
  removing,
}: {
  changesetId: string;
  pilot: Pilot;
  projectName?: string;
  onRemove: () => void;
  removing: boolean;
}) {
  // A trial is a walk over the stream, so it is fetched when it is asked for
  // rather than on every render of the detail page.
  const [showFindings, setShowFindings] = useState(false);

  return (
    <li className="space-y-2 rounded-md border bg-card px-3 py-2 text-sm">
      <div className="flex items-center gap-2">
        <div className="min-w-0 flex-1">
          <div className="truncate font-medium text-foreground">
            {projectName ?? pilot.project_id}
          </div>
          <div className="text-[11px] text-muted-foreground">
            started {formatRelative(pilot.created_at)}
          </div>
        </div>
        <Badge variant="outline" className="font-mono text-[10px]">
          {pilot.stream}
        </Badge>
        <TrialToggle open={showFindings} onToggle={() => setShowFindings((v) => !v)} />
        <Button
          size="icon"
          variant="ghost"
          className="size-7 text-muted-foreground hover:text-destructive"
          onClick={onRemove}
          disabled={removing}
          aria-label="Stop pilot"
        >
          <Trash2 />
        </Button>
      </div>
      {showFindings && (
        <TrialPanel
          changesetId={changesetId}
          projectId={pilot.project_id}
          stream={pilot.stream}
          projectName={projectName}
          className="border-0 bg-transparent shadow-none"
        />
      )}
    </li>
  );
}

function AddPilotDialog({
  changesetId,
  open,
  onOpenChange,
}: {
  changesetId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const add = useAddPilot(changesetId);
  const { data: projects } = useProjects();
  const [projectId, setProjectId] = useState("");
  const [stream, setStream] = useState("main");

  const canSubmit = projectId.length > 0 && stream.trim().length > 0 && !add.isPending;

  const submit = () => {
    if (!canSubmit) return;
    add.mutate(
      { project_id: projectId, stream: stream.trim() },
      {
        onSuccess: () => {
          setProjectId("");
          setStream("main");
          onOpenChange(false);
        },
      },
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Start a pilot</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="pilot-project">Project</Label>
            <Select
              value={projectId}
              onValueChange={(v) => {
                setProjectId(v);
                const p = projects?.find((x) => x.id === v);
                if (p?.default_stream) setStream(p.default_stream);
              }}
            >
              <SelectTrigger id="pilot-project">
                <SelectValue placeholder="Choose a project…" />
              </SelectTrigger>
              <SelectContent>
                {(projects ?? []).map((p) => (
                  <SelectItem key={p.id} value={p.id}>
                    {p.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="pilot-stream">Stream</Label>
            <Input
              id="pilot-stream"
              value={stream}
              onChange={(e) => setStream(e.target.value)}
              placeholder="main"
            />
          </div>
          {add.isError && (
            <ErrorNotice error={add.error} title="Couldn't start the pilot" variant="inline" />
          )}
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={submit} disabled={!canSubmit}>
            {add.isPending ? "Starting…" : "Start pilot"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

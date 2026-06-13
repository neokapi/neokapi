// Pilots panel (AD-021): bind a change-set to a project's content stream so
// real content and real checks resolve through the draft before it merges — a
// what-if exercised on a slice of live content. Lists active pilots and stops
// them; new pilots pick a project + stream.
import { useState } from "react";
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
import { useAddPilot, useRemovePilot } from "../../hooks/useChangesetsApi";
import { useProjects } from "../../hooks/useProjectApi";
import { formatRelative } from "../shell/atoms";

export interface PilotsPanelProps {
  changeset: ChangeSetDetail;
}

export function PilotsPanel({ changeset }: PilotsPanelProps) {
  const remove = useRemovePilot(changeset.id);
  const [addOpen, setAddOpen] = useState(false);
  const terminal = changeset.status === "merged" || changeset.status === "abandoned";

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
              Bind this experiment to a content stream to exercise it on real content before merge.
            </p>
          </div>
        ) : (
          <ul className="space-y-1.5">
            {changeset.pilots.map((p) => (
              <PilotRow
                key={`${p.project_id}/${p.stream}`}
                pilot={p}
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
  pilot,
  onRemove,
  removing,
}: {
  pilot: Pilot;
  onRemove: () => void;
  removing: boolean;
}) {
  return (
    <li className="flex items-center gap-2 rounded-md border bg-card px-3 py-2 text-sm">
      <div className="min-w-0 flex-1">
        <div className="truncate font-medium text-foreground">{pilot.project_id}</div>
        <div className="text-[11px] text-muted-foreground">
          started {formatRelative(pilot.created_at)}
        </div>
      </div>
      <Badge variant="outline" className="font-mono text-[10px]">
        {pilot.stream}
      </Badge>
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
            <p className="text-sm text-destructive">
              {add.error instanceof Error ? add.error.message : "Could not start pilot."}
            </p>
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

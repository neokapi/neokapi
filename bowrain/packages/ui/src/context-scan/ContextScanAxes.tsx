import {
  Badge,
  Button,
  Card,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@neokapi/ui-primitives";
import { useQuery } from "@tanstack/react-query";
import { useCallback, useMemo, useState } from "react";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type { ContextScanAxis } from "../types/api";
import { Loader2 } from "../components/icons";
import { TEST_IDS } from "../test-ids";

/**
 * The two axes derived from a collection's `channel:` rather than declared on
 * the project's default point. Approving one is a claim about a particular
 * collection, so the reviewer must name it; every other axis applies to the
 * whole project and naming a collection would be a narrower claim than the
 * axis makes.
 *
 * The server holds the same list and refuses the wrong shape, so this only
 * decides which control to draw — a stale copy here costs a 409, never a bad
 * write.
 */
const STRUCTURAL_AXES = new Set(["product", "channel"]);

export interface ContextScanAxesProps {
  axes: ContextScanAxis[];
}

/**
 * The axes a scan proposed, each approvable on its own.
 *
 * An axis is a claim about the shape of a project's context space, and it is a
 * different decision from the artefacts that sit on it: approving one edits the
 * recipe, approving an artefact does not. So each row is the claim written out
 * — this value, on this project, for this collection — with the blanks the scan
 * could not fill. A scan reads a corpus of pasted text, links and uploads; it
 * never sees the project's collections, so which one a product or channel
 * applies to is the reviewer's to say.
 */
export function ContextScanAxes({ axes }: ContextScanAxesProps) {
  if (axes.length === 0) return null;
  return (
    <Card className="p-5 space-y-4" data-testid={TEST_IDS.contextScan.axes}>
      <div className="space-y-1">
        <h2 className="text-sm font-semibold">Where this content varies</h2>
        <p className="text-xs text-muted-foreground">
          Dimensions the corpus distinguishes. Approving one proposes a coordinate for your recipe;
          it lands in <code>kapi.yaml</code> on your next pull, where you review it in git like any
          other change.
        </p>
      </div>
      <ul className="space-y-4">
        {axes.map((axis) => (
          <li key={axis.axis}>
            <AxisRow axis={axis} />
          </li>
        ))}
      </ul>
    </Card>
  );
}

function AxisRow({ axis }: { axis: ContextScanAxis }) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const structural = STRUCTURAL_AXES.has(axis.axis);

  const [value, setValue] = useState(axis.values[0] ?? "");
  const [projectId, setProjectId] = useState("");
  const [collection, setCollection] = useState("");
  const [proposing, setProposing] = useState(false);
  const [proposed, setProposed] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const projects = useQuery({
    queryKey: ["projects", ws],
    queryFn: () => api.listProjects(ws),
    enabled: ws !== "",
  });

  // A collection list belongs to one project, so it is only asked for once a
  // project is chosen — and only for the axes that need one.
  const collections = useQuery({
    queryKey: ["collections", ws, projectId],
    queryFn: () => api.listCollections(ws, projectId),
    enabled: ws !== "" && projectId !== "" && structural,
  });

  const chooseProject = useCallback((id: string) => {
    setProjectId(id);
    setCollection("");
    setError(null);
    setProposed(null);
  }, []);

  const ready = value !== "" && projectId !== "" && (!structural || collection !== "");

  const propose = useCallback(async () => {
    if (!ready || proposing) return;
    setProposing(true);
    setError(null);
    try {
      const change = await api.approveAxis(ws, projectId, {
        axis: axis.axis,
        value,
        ...(structural ? { collection } : {}),
      });
      setProposed(`${change.path} = ${JSON.stringify(change.value)}`);
    } catch (err) {
      // The server's refusals here are written for the reviewer — "name the
      // collection this applies to", "this collection has no product yet" —
      // and say what to do next, so they are shown rather than summarised.
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setProposing(false);
    }
  }, [api, ws, projectId, axis.axis, value, structural, collection, ready, proposing]);

  const confidence = useMemo(() => Math.round(axis.confidence * 100), [axis.confidence]);

  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2 flex-wrap">
        <Badge variant="secondary" className="text-[10px] uppercase tracking-wide">
          {axis.axis}
        </Badge>
        {axis.values.map((v) => (
          <button
            key={v}
            type="button"
            onClick={() => {
              setValue(v);
              setProposed(null);
              setError(null);
            }}
            aria-pressed={v === value}
            className={
              v === value
                ? "text-xs rounded border border-primary bg-primary/10 px-1.5 py-0.5 font-medium"
                : "text-xs rounded border px-1.5 py-0.5 text-muted-foreground hover:border-foreground/40"
            }
          >
            {v}
          </button>
        ))}
        <span className="text-[10px] text-muted-foreground tabular-nums ml-auto">
          {confidence}% confident
        </span>
      </div>

      {axis.evidence && axis.evidence.length > 0 && (
        <p className="text-[11px] text-muted-foreground truncate">{axis.evidence[0]}</p>
      )}

      <div className="flex items-end gap-2 flex-wrap">
        <Select value={projectId} onValueChange={chooseProject}>
          <SelectTrigger className="h-8 w-[180px]" aria-label={`Project for ${axis.axis}`}>
            <SelectValue placeholder="Choose a project" />
          </SelectTrigger>
          <SelectContent>
            {(projects.data ?? []).map((p) => (
              <SelectItem key={p.id} value={p.id}>
                {p.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {structural && (
          <Select
            value={collection}
            onValueChange={(c) => {
              setCollection(c);
              setError(null);
              setProposed(null);
            }}
            disabled={projectId === ""}
          >
            <SelectTrigger className="h-8 w-[180px]" aria-label={`Collection for ${axis.axis}`}>
              <SelectValue placeholder="Choose a collection" />
            </SelectTrigger>
            <SelectContent>
              {(collections.data ?? []).map((c) => (
                <SelectItem key={c.id} value={c.name}>
                  {c.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}

        <Button size="sm" onClick={propose} disabled={!ready || proposing}>
          {proposing && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
          Propose in kapi.yaml
        </Button>
      </div>

      {structural && projectId !== "" && (collections.data ?? []).length === 0 && (
        <p className="text-[11px] text-muted-foreground">
          This project has no collections yet. {axis.axis} is derived from a collection&apos;s
          channel, so there is nowhere to put it.
        </p>
      )}

      {proposed && (
        <p className="text-[11px] text-muted-foreground">
          Waiting for your next pull: <code>{proposed}</code>
        </p>
      )}
      {error && (
        <p role="alert" className="text-[11px] text-destructive">
          {error}
        </p>
      )}
    </div>
  );
}

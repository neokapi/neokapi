import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Button,
  Input,
  Label,
  Alert,
  AlertDescription,
  Loader2,
  CircleCheck,
  useApi,
  useWorkspace,
  type SlugCheckResponse,
  type Workspace,
} from "@neokapi/ui";
import { workspacesQueryOptions } from "../queries";

const SLUG_PATTERN = /^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$/;

type SlugState =
  | { kind: "idle" }
  | { kind: "checking" }
  | { kind: "ok" }
  | { kind: "invalid"; message: string }
  | { kind: "taken" }
  | { kind: "reserved" };

function reasonToMessage(reason: string | undefined): string {
  switch (reason) {
    case "invalid":
      return "Use 2–64 lowercase letters, numbers, and hyphens.";
    case "taken":
      return "That handle is already in use.";
    case "reserved":
      return "That handle was used recently and is reserved for a few weeks.";
    default:
      return "Handle is unavailable.";
  }
}

/**
 * ProfileHandleCard — rename the user's personal-workspace slug.
 *
 * The slug doubles as the user's public handle in URLs. Renaming reserves
 * the old slug for 30 days (server-side) so it can't be reused for
 * impersonation, then redirects the user to the new workspace URL.
 */
export function ProfileHandleCard() {
  const api = useApi();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { workspaces } = useWorkspace();
  const { data: liveWorkspaces } = useQuery(workspacesQueryOptions(api));

  const personal = useMemo<Workspace | undefined>(() => {
    const list: Workspace[] = liveWorkspaces ?? workspaces ?? [];
    return list.find((w) => w.type === "personal");
  }, [liveWorkspaces, workspaces]);

  const [editing, setEditing] = useState(false);
  const [slug, setSlug] = useState("");
  const [slugState, setSlugState] = useState<SlugState>({ kind: "idle" });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const checkSeq = useRef(0);

  // Reset draft when the active personal workspace changes.
  useEffect(() => {
    if (personal && !editing) {
      setSlug(personal.slug);
    }
  }, [personal, editing]);

  // Debounced availability check (skip when value matches current slug).
  useEffect(() => {
    if (!editing) return;
    if (!slug || slug === personal?.slug) {
      setSlugState({ kind: "idle" });
      return;
    }
    if (!SLUG_PATTERN.test(slug)) {
      setSlugState({ kind: "invalid", message: reasonToMessage("invalid") });
      return;
    }
    setSlugState({ kind: "checking" });
    const seq = ++checkSeq.current;
    const handle = setTimeout(() => {
      void (async () => {
        try {
          const result: SlugCheckResponse = await api.checkSlug(slug);
          if (seq !== checkSeq.current) return;
          if (result.available) {
            setSlugState({ kind: "ok" });
          } else if (result.reason === "taken") {
            setSlugState({ kind: "taken" });
          } else if (result.reason === "reserved") {
            setSlugState({ kind: "reserved" });
          } else {
            setSlugState({ kind: "invalid", message: reasonToMessage(result.reason) });
          }
        } catch {
          if (seq !== checkSeq.current) return;
          setSlugState({ kind: "idle" });
        }
      })();
    }, 250);
    return () => clearTimeout(handle);
  }, [slug, api, personal?.slug, editing]);

  if (!personal) {
    return null;
  }

  const submitDisabled =
    submitting || !editing || slug === personal.slug || slugState.kind !== "ok";

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      const updated = await api.updateWorkspace(personal.slug, { slug });
      void queryClient.invalidateQueries({ queryKey: ["workspaces"] });
      setEditing(false);
      void navigate({ to: "/$workspace", params: { workspace: updated.slug }, replace: true });
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Failed to rename workspace.");
    } finally {
      setSubmitting(false);
    }
  };

  const slugHint = (() => {
    if (!editing || slug === personal.slug) return null;
    switch (slugState.kind) {
      case "checking":
        return (
          <span className="flex items-center gap-1 text-muted-foreground">
            <Loader2 className="h-3 w-3 animate-spin" /> Checking…
          </span>
        );
      case "ok":
        return (
          <span className="flex items-center gap-1 text-emerald-600 dark:text-emerald-400">
            <CircleCheck className="h-3 w-3" /> Available
          </span>
        );
      case "invalid":
        return <span className="text-destructive">{slugState.message}</span>;
      case "taken":
        return <span className="text-destructive">{reasonToMessage("taken")}</span>;
      case "reserved":
        return <span className="text-destructive">{reasonToMessage("reserved")}</span>;
      default:
        return null;
    }
  })();

  return (
    <Card>
      <CardHeader>
        <CardTitle>Handle</CardTitle>
        <CardDescription>
          The slug for your personal workspace and your public handle in URLs. Renaming reserves the
          old handle for 30 days to prevent impersonation.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex items-center justify-between gap-3">
          <div className="text-sm">
            <div className="font-medium">{personal.slug}</div>
            <div className="text-muted-foreground text-xs">
              <code>app.bowrain.cloud/{personal.slug}</code>
            </div>
          </div>
          {!editing && (
            <Button variant="outline" size="sm" onClick={() => setEditing(true)}>
              Change handle
            </Button>
          )}
        </div>

        {editing && (
          <form className="space-y-3 border-t pt-4" onSubmit={handleSubmit}>
            <div className="space-y-1.5">
              <Label htmlFor="profile-handle">New handle</Label>
              <Input
                id="profile-handle"
                value={slug}
                onChange={(e) => setSlug(e.target.value.toLowerCase())}
                placeholder="your-new-handle"
                autoComplete="off"
                spellCheck={false}
              />
              <p className="text-xs">{slugHint}</p>
            </div>
            {error && (
              <Alert variant="destructive">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
            <div className="flex gap-2">
              <Button type="submit" disabled={submitDisabled}>
                {submitting ? (
                  <>
                    <Loader2 className="mr-2 h-4 w-4 animate-spin" /> Renaming…
                  </>
                ) : (
                  "Save"
                )}
              </Button>
              <Button
                type="button"
                variant="ghost"
                onClick={() => {
                  setEditing(false);
                  setSlug(personal.slug);
                  setError("");
                }}
                disabled={submitting}
              >
                Cancel
              </Button>
            </div>
          </form>
        )}
      </CardContent>
    </Card>
  );
}

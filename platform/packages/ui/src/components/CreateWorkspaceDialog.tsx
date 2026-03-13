import { useState } from "react";
import type { Workspace } from "../types/api";
import { useApi } from "../context/ApiContext";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "./ui/dialog";
import { AlertGlass, AlertGlassDescription } from "./ui/alert";

export interface CreateWorkspaceDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreate: (ws: Workspace) => void;
}

export function CreateWorkspaceDialog({
  open,
  onOpenChange,
  onCreate,
}: CreateWorkspaceDialogProps) {
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState("");
  const adapter = useApi();

  const handleNameChange = (value: string) => {
    setName(value);
    setSlug(
      value
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, "-")
        .replace(/^-|-$/g, ""),
    );
  };

  const handleCreate = async () => {
    if (!name.trim() || !slug.trim()) return;
    setCreating(true);
    setError("");
    try {
      const ws = await adapter.createWorkspace(name.trim(), slug.trim());
      setName("");
      setSlug("");
      onCreate(ws);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to create workspace");
    } finally {
      setCreating(false);
    }
  };

  const handleOpenChange = (v: boolean) => {
    if (!v) {
      setName("");
      setSlug("");
      setError("");
    }
    onOpenChange(v);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent size="sm" onInteractOutside={(e: Event) => e.preventDefault()}>
        <DialogHeader>
          <DialogTitle>Create Workspace</DialogTitle>
        </DialogHeader>

        <div className="flex flex-col gap-4 py-2">
          <div>
            <Label className="text-muted-foreground">Name</Label>
            <Input
              value={name}
              onChange={(e) => handleNameChange(e.target.value)}
              placeholder="My Workspace"
              autoFocus
              className="mt-1"
            />
          </div>
          <div>
            <Label className="text-muted-foreground">Slug</Label>
            <Input
              value={slug}
              onChange={(e) => setSlug(e.target.value)}
              placeholder="my-workspace"
              className="mt-1"
            />
          </div>
          {error && (
            <AlertGlass variant="destructive">
              <AlertGlassDescription>{error}</AlertGlassDescription>
            </AlertGlass>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={handleCreate} disabled={creating || !name.trim() || !slug.trim()}>
            {creating ? "Creating..." : "Create"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

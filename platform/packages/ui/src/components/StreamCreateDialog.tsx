import { useState, useEffect } from "react";
import type { StreamInfo, StreamVisibility } from "../types/api";
import { Button } from "@neokapi/ui-primitives/components/ui/button";
import { Input } from "@neokapi/ui-primitives/components/ui/input";
import { Label } from "@neokapi/ui-primitives/components/ui/label";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "@neokapi/ui-primitives/components/ui/select";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@neokapi/ui-primitives/components/ui/dialog";

export interface StreamCreateDialogProps {
  /** Existing streams for parent selection. */
  streams: StreamInfo[];
  /** Called when the user submits the form. */
  onSubmit: (data: {
    name: string;
    parent: string;
    visibility: StreamVisibility;
    description: string;
  }) => void;
  /** Called to close the dialog. */
  onClose: () => void;
  /** Whether the dialog is open. */
  open: boolean;
}

/** Modal dialog for creating a new stream. */
export function StreamCreateDialog({ streams, onSubmit, onClose, open }: StreamCreateDialogProps) {
  const [name, setName] = useState("");
  const [parent, setParent] = useState("");
  const [visibility, setVisibility] = useState<StreamVisibility>("private");
  const [description, setDescription] = useState("");

  // Set parent to a sensible default when dialog opens or streams change
  useEffect(() => {
    if (open) {
      const parentStreams = streams.filter((s) => !s.archived);
      const mainStream = parentStreams.find((s) => s.name === "main");
      setParent(mainStream?.name ?? parentStreams[0]?.name ?? "");
    }
  }, [open, streams]);

  const handleSubmit = () => {
    if (!name.trim() || !parent) return;
    onSubmit({
      name: name.trim(),
      parent,
      visibility,
      description: description.trim(),
    });
    resetForm();
  };

  const resetForm = () => {
    setName("");
    setParent(streams[0]?.name ?? "");
    setVisibility("private");
    setDescription("");
  };

  const handleOpenChange = (v: boolean) => {
    if (!v) {
      resetForm();
      onClose();
    }
  };

  const parentStreams = streams.filter((s) => !s.archived);

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        className="sm:max-w-[480px]"
        onInteractOutside={(e: Event) => e.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>Create Stream</DialogTitle>
        </DialogHeader>

        <div className="flex flex-col gap-4 py-2">
          <div>
            <Label className="text-muted-foreground">Name</Label>
            <Input
              value={name}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => setName(e.target.value)}
              placeholder="feature/translations"
              autoFocus
              className="mt-1"
            />
          </div>

          <div>
            <Label className="text-muted-foreground">Parent Stream</Label>
            <Select value={parent} onValueChange={setParent}>
              <SelectTrigger className="mt-1">
                <SelectValue placeholder="Select parent stream" />
              </SelectTrigger>
              <SelectContent>
                {parentStreams.map((s) => (
                  <SelectItem key={s.name} value={s.name}>
                    {s.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div>
            <Label className="text-muted-foreground">Visibility</Label>
            <Select
              value={visibility}
              onValueChange={(v: string) => setVisibility(v as StreamVisibility)}
            >
              <SelectTrigger className="mt-1">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="public">Public</SelectItem>
                <SelectItem value="shared">Shared</SelectItem>
                <SelectItem value="private">Private</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div>
            <Label className="text-muted-foreground">Description</Label>
            <Input
              value={description}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => setDescription(e.target.value)}
              placeholder="Optional description"
              className="mt-1"
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={handleSubmit} disabled={!name.trim() || !parent}>
            Create
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

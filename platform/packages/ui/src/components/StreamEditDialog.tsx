import { useState, useEffect } from "react";
import type { StreamInfo, StreamVisibility } from "../types/api";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from "./ui/select";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "./ui/dialog";

export interface StreamEditDialogProps {
  stream: StreamInfo | null;
  onSubmit: (data: { description: string; visibility: StreamVisibility }) => void;
  onClose: () => void;
  open: boolean;
}

export function StreamEditDialog({ stream, onSubmit, onClose, open }: StreamEditDialogProps) {
  const [description, setDescription] = useState("");
  const [visibility, setVisibility] = useState<StreamVisibility>("private");

  useEffect(() => {
    if (stream && open) {
      setDescription(stream.description);
      setVisibility(stream.visibility);
    }
  }, [stream, open]);

  const handleSubmit = () => {
    onSubmit({ description: description.trim(), visibility });
  };

  const handleOpenChange = (v: boolean) => {
    if (!v) onClose();
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent size="sm" onInteractOutside={(e: Event) => e.preventDefault()}>
        <DialogHeader>
          <DialogTitle>Edit Stream — {stream?.name}</DialogTitle>
        </DialogHeader>

        <div className="flex flex-col gap-4 py-2">
          <div>
            <Label className="text-muted-foreground">Description</Label>
            <Input
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="What is this stream for?"
              autoFocus
              className="mt-1"
            />
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
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={handleSubmit}>
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Button,
  Label,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@neokapi/ui";
import { addNote } from "../api";

interface AddNoteDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string;
}

export function AddNoteDialog({ open, onOpenChange, workspaceId }: AddNoteDialogProps) {
  const queryClient = useQueryClient();
  const [content, setContent] = useState("");

  const mutation = useMutation({
    mutationFn: () => addNote(workspaceId, content),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ["admin", "workspace", workspaceId, "notes"],
      });
      onOpenChange(false);
      setContent("");
    },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle>Add Internal Note</DialogTitle>
          <DialogDescription>
            Add a note visible only to admins. Useful for tracking support interactions and
            decisions.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <Label htmlFor="note-content">Note</Label>
            <textarea
              id="note-content"
              className="w-full min-h-[100px] rounded-md border bg-background px-3 py-2 text-sm resize-y"
              placeholder="e.g. Customer called about upgrade options. Offered 2-week Pro trial."
              value={content}
              onChange={(e) => setContent(e.target.value)}
            />
          </div>
          {mutation.error && (
            <p className="text-sm text-destructive">
              {mutation.error instanceof Error ? mutation.error.message : "Failed to add note"}
            </p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={() => mutation.mutate()}
            disabled={!content.trim() || mutation.isPending}
          >
            Add Note
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

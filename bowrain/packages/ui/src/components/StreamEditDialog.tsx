import {
  Button,
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
import { useState, useEffect } from "react";
import type { StreamInfo, StreamVisibility } from "../types/api";
import type { VoiceProfile } from "../voice/types";

/** Binding key stored inside a stream's properties map. */
const BRAND_VOICE_KEY = "voice_profile_id";

export interface StreamEditDialogProps {
  stream: StreamInfo | null;
  onSubmit: (data: {
    description: string;
    visibility: StreamVisibility;
    properties?: Record<string, string>;
  }) => void;
  onClose: () => void;
  open: boolean;
  /** Workspace voice profiles; when non-empty the dialog offers a voice picker. */
  voiceProfiles?: VoiceProfile[];
}

export function StreamEditDialog({
  stream,
  onSubmit,
  onClose,
  open,
  voiceProfiles,
}: StreamEditDialogProps) {
  const [description, setDescription] = useState("");
  const [visibility, setVisibility] = useState<StreamVisibility>("private");
  const [voiceProfileId, setVoiceProfileId] = useState("");

  const showVoicePicker = !!voiceProfiles && voiceProfiles.length > 0;

  useEffect(() => {
    if (stream && open) {
      setDescription(stream.description);
      setVisibility(stream.visibility);
      setVoiceProfileId(stream.properties?.[BRAND_VOICE_KEY] ?? "");
    }
  }, [stream, open]);

  const handleSubmit = () => {
    onSubmit({
      description: description.trim(),
      visibility,
      // Always send the binding when the picker is shown so clearing back to
      // "Inherit" persists (the server merges properties key-by-key).
      ...(showVoicePicker ? { properties: { [BRAND_VOICE_KEY]: voiceProfileId } } : {}),
    });
  };

  const handleOpenChange = (v: boolean) => {
    if (!v) onClose();
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        className="sm:max-w-[480px]"
        onInteractOutside={(e: Event) => e.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>Edit Stream · {stream?.name}</DialogTitle>
        </DialogHeader>

        <div className="flex flex-col gap-4 py-2">
          <div>
            <Label className="text-muted-foreground">Description</Label>
            <Input
              value={description}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => setDescription(e.target.value)}
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

          {showVoicePicker && (
            <div>
              <Label className="text-muted-foreground">Voice</Label>
              <select
                className="mt-1 h-9 w-full rounded-md border border-input bg-background px-2 text-sm"
                value={voiceProfileId}
                onChange={(e: React.ChangeEvent<HTMLSelectElement>) =>
                  setVoiceProfileId(e.target.value)
                }
                aria-label="Stream voice profile"
              >
                <option value="">Inherit (project)</option>
                {voiceProfiles?.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </select>
              <p className="text-[11px] text-muted-foreground mt-1">
                Overrides the project voice for content in this stream
              </p>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={handleSubmit}>Save</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

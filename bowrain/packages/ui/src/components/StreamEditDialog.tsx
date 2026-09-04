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
  VoiceBindingSelect,
} from "@neokapi/ui-primitives";
import { useState, useEffect } from "react";
import type { StreamInfo, StreamVisibility } from "../types/api";
import type { VoiceProfile } from "../voice/types";
import { VOICE_PROFILE_KEY, voiceProfileOptions } from "../voice/binding";

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
      setVoiceProfileId(stream.properties?.[VOICE_PROFILE_KEY] ?? "");
    }
  }, [stream, open]);

  const handleSubmit = () => {
    onSubmit({
      description: description.trim(),
      visibility,
      // Always send the binding when the picker is shown so clearing back to
      // "Inherit" persists (the server merges properties key-by-key).
      ...(showVoicePicker ? { properties: { [VOICE_PROFILE_KEY]: voiceProfileId } } : {}),
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
            <VoiceBindingSelect
              label="Voice"
              value={voiceProfileId || undefined}
              onChange={(next) => setVoiceProfileId(next ?? "")}
              options={voiceProfileOptions(voiceProfiles)}
              inheritLabel="Inherit (project)"
              help="Overrides the project voice for content in this stream"
              className="w-full max-w-none"
            />
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

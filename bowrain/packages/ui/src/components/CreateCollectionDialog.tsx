import {
  Button,
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
  VoiceBindingSelect,
} from "@neokapi/ui-primitives";
import { useState, useEffect } from "react";
import type { CollectionKind, CollectionInfo } from "../types/api";
import type { VoiceProfile } from "../voice/types";
import { VOICE_PROFILE_KEY, voiceProfileOptions } from "../voice/binding";
import { Upload, Plug } from "./icons";

export interface CreateCollectionDialogProps {
  open: boolean;
  onClose: () => void;
  onSubmit: (data: {
    name: string;
    kind: CollectionKind;
    item_label: string;
    connector_config?: Record<string, string>;
  }) => void;
  /** When set, the dialog operates in edit mode. */
  editCollection?: CollectionInfo;
  /** Workspace voice profiles; when non-empty the dialog offers a voice picker. */
  voiceProfiles?: VoiceProfile[];
}

export function CreateCollectionDialog({
  open,
  onClose,
  onSubmit,
  editCollection,
  voiceProfiles,
}: CreateCollectionDialogProps) {
  const [name, setName] = useState("");
  const [kind, setKind] = useState<CollectionKind>("uploaded");
  const [itemLabel, setItemLabel] = useState("");
  const [voiceProfileId, setVoiceProfileId] = useState("");

  const isEdit = !!editCollection;
  const showVoicePicker = !!voiceProfiles && voiceProfiles.length > 0;

  // Populate fields when editing
  useEffect(() => {
    if (editCollection && open) {
      setName(editCollection.name);
      setKind(editCollection.kind);
      setItemLabel(editCollection.item_label === "item" ? "" : editCollection.item_label);
      setVoiceProfileId(editCollection.connector_config?.[VOICE_PROFILE_KEY] ?? "");
    }
  }, [editCollection, open]);

  const resetForm = () => {
    setName("");
    setKind("uploaded");
    setItemLabel("");
    setVoiceProfileId("");
  };

  const handleOpenChange = (v: boolean) => {
    if (!v) {
      resetForm();
      onClose();
    }
  };

  const handleSubmit = () => {
    if (!name.trim()) return;
    // Preserve any existing connector_config (secrets are re-added server-side)
    // and layer the voice binding on top. On create, only send the map
    // when a profile is picked; on edit, always send it so clearing back to
    // "Inherit" persists an empty binding.
    let connectorConfig: Record<string, string> | undefined;
    if (showVoicePicker) {
      if (isEdit) {
        connectorConfig = {
          ...editCollection?.connector_config,
          [VOICE_PROFILE_KEY]: voiceProfileId,
        };
      } else if (voiceProfileId) {
        connectorConfig = { [VOICE_PROFILE_KEY]: voiceProfileId };
      }
    }
    onSubmit({
      name: name.trim(),
      kind,
      item_label: itemLabel.trim() || "item",
      ...(connectorConfig ? { connector_config: connectorConfig } : {}),
    });
    resetForm();
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        className="sm:max-w-[480px]"
        onInteractOutside={(e: Event) => e.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>{isEdit ? "Edit Collection" : "Create Collection"}</DialogTitle>
        </DialogHeader>

        <div className="flex flex-col gap-4 py-2">
          <div>
            <Label className="text-muted-foreground">Name</Label>
            <Input
              value={name}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => setName(e.target.value)}
              placeholder="e.g. Marketing, Documentation"
              autoFocus
              className="mt-1"
            />
          </div>

          <div>
            <Label className="text-muted-foreground">Type</Label>
            <div className="grid grid-cols-2 gap-2 mt-1.5">
              <button
                type="button"
                onClick={() => setKind("uploaded")}
                disabled={isEdit}
                className={`
                  flex flex-col items-center gap-2 p-3 rounded-lg border cursor-pointer
                  transition-all duration-150 bg-transparent
                  ${isEdit ? "opacity-60 cursor-not-allowed" : ""}
                  ${
                    kind === "uploaded"
                      ? "border-primary/50 bg-primary/5 ring-1 ring-primary/20"
                      : "border-border/50 hover:border-border hover:bg-accent/30"
                  }
                `}
              >
                <Upload
                  className={`w-5 h-5 ${kind === "uploaded" ? "text-primary" : "text-muted-foreground"}`}
                />
                <div className="text-center">
                  <div
                    className={`text-sm font-medium ${kind === "uploaded" ? "text-foreground" : "text-muted-foreground"}`}
                  >
                    Uploaded
                  </div>
                  <div className="text-[11px] text-muted-foreground mt-0.5">
                    Upload files manually
                  </div>
                </div>
              </button>

              <button
                type="button"
                onClick={() => setKind("connected")}
                disabled={isEdit}
                className={`
                  flex flex-col items-center gap-2 p-3 rounded-lg border cursor-pointer
                  transition-all duration-150 bg-transparent
                  ${isEdit ? "opacity-60 cursor-not-allowed" : ""}
                  ${
                    kind === "connected"
                      ? "border-primary/50 bg-primary/5 ring-1 ring-primary/20"
                      : "border-border/50 hover:border-border hover:bg-accent/30"
                  }
                `}
              >
                <Plug
                  className={`w-5 h-5 ${kind === "connected" ? "text-primary" : "text-muted-foreground"}`}
                />
                <div className="text-center">
                  <div
                    className={`text-sm font-medium ${kind === "connected" ? "text-foreground" : "text-muted-foreground"}`}
                  >
                    Connected
                  </div>
                  <div className="text-[11px] text-muted-foreground mt-0.5">Sync from a source</div>
                </div>
              </button>
            </div>
            {isEdit && (
              <p className="text-[11px] text-muted-foreground mt-1">
                Collection type cannot be changed after creation
              </p>
            )}
          </div>

          <div>
            <Label className="text-muted-foreground">Item label</Label>
            <Input
              value={itemLabel}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => setItemLabel(e.target.value)}
              placeholder="item (e.g. page, document, post)"
              className="mt-1"
            />
            <p className="text-[11px] text-muted-foreground mt-1">
              How items in this collection are referred to in the UI
            </p>
          </div>

          {showVoicePicker && (
            <VoiceBindingSelect
              label="Voice"
              value={voiceProfileId || undefined}
              onChange={(next) => setVoiceProfileId(next ?? "")}
              options={voiceProfileOptions(voiceProfiles)}
              inheritLabel="Inherit (stream/project)"
              help="Overrides the stream and project voice for content in this collection"
              className="w-full max-w-none"
            />
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={handleSubmit} disabled={!name.trim()}>
            {isEdit ? "Save" : "Create"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StreamCreateDialog } from "../components/StreamCreateDialog";
import { StreamEditDialog } from "../components/StreamEditDialog";
import { CreateCollectionDialog } from "../components/CreateCollectionDialog";
import type { CollectionInfo, StreamInfo } from "../types/api";
import type { VoiceProfile } from "../voice/types";
import { VOICE_PROFILE_KEY, voiceProfileOptions } from "../voice/binding";

const profiles = [
  { id: "vp-1", name: "Northsea support" },
  { id: "vp-2", name: "Northsea campaigns" },
] as VoiceProfile[];

const main: StreamInfo = {
  name: "main",
  parent: "",
  visibility: "public",
  description: "",
  archived: false,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  properties: { [VOICE_PROFILE_KEY]: "vp-1" },
} as StreamInfo;

describe("the voice a stream or collection binds", () => {
  it("maps the workspace profiles to the shared picker's options", () => {
    expect(voiceProfileOptions(profiles)).toEqual([
      { value: "vp-1", label: "Northsea support" },
      { value: "vp-2", label: "Northsea campaigns" },
    ]);
    expect(voiceProfileOptions(undefined)).toEqual([]);
  });

  it("binds a voice on a new stream only when one is picked", async () => {
    const onSubmit = vi.fn();
    render(
      <StreamCreateDialog
        streams={[main]}
        onSubmit={onSubmit}
        onClose={() => {}}
        open
        voiceProfiles={profiles}
      />,
    );
    await userEvent.type(screen.getByPlaceholderText("feature/translations"), "feature/x");
    expect(screen.getByLabelText("Voice")).toHaveTextContent("Inherit (project)");
    await userEvent.click(screen.getByLabelText("Voice"));
    await userEvent.click(screen.getByRole("option", { name: "Northsea campaigns" }));
    await userEvent.click(screen.getByRole("button", { name: "Create" }));
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ properties: { [VOICE_PROFILE_KEY]: "vp-2" } }),
    );
  });

  it("shows the stream's bound voice and persists clearing it back to inherit", async () => {
    const onSubmit = vi.fn();
    render(
      <StreamEditDialog
        stream={main}
        onSubmit={onSubmit}
        onClose={() => {}}
        open
        voiceProfiles={profiles}
      />,
    );
    expect(screen.getByLabelText("Voice")).toHaveTextContent("Northsea support");
    await userEvent.click(screen.getByLabelText("Voice"));
    await userEvent.click(screen.getByRole("option", { name: "Inherit (project)" }));
    await userEvent.click(screen.getByRole("button", { name: /Save|Update/ }));
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ properties: { [VOICE_PROFILE_KEY]: "" } }),
    );
  });

  it("layers a collection's voice over its connector config", async () => {
    const onSubmit = vi.fn();
    const coll = {
      id: "c1",
      name: "Docs",
      kind: "uploaded",
      item_label: "page",
      connector_config: { token: "keep" },
    } as unknown as CollectionInfo;
    render(
      <CreateCollectionDialog
        open
        onClose={() => {}}
        onSubmit={onSubmit}
        editCollection={coll}
        voiceProfiles={profiles}
      />,
    );
    await userEvent.click(screen.getByLabelText("Voice"));
    await userEvent.click(screen.getByRole("option", { name: "Northsea support" }));
    await userEvent.click(screen.getByRole("button", { name: /Save|Update/ }));
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        connector_config: { token: "keep", [VOICE_PROFILE_KEY]: "vp-1" },
      }),
    );
  });
});

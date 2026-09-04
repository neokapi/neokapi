// A voice bound at a point on the platform: the profile id, under one property
// key at every level (project properties, stream properties, a collection's
// connector config). The shared VoiceBindingSelect takes the workspace's
// profiles as options and hands the id back.

import type { VoiceBindingOption } from "@neokapi/ui-primitives";
import type { VoiceProfile } from "./types";

/** The property a voice binding is stored under, at every level. */
export const VOICE_PROFILE_KEY = "voice_profile_id";

/** The workspace's profiles as the shared picker's options. */
export function voiceProfileOptions(
  profiles: Pick<VoiceProfile, "id" | "name">[] | undefined,
): VoiceBindingOption[] {
  return (profiles ?? []).map((p) => ({ value: p.id, label: p.name }));
}

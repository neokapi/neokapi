// The recipe's voice binding, as the shared picker's opaque key.
//
// `defaults.voice` takes one of three forms: a profile file in the tree, a
// starter pack, or a named profile in the store. The shared VoiceBindingSelect
// hands keys back and forth without reading them, so the three forms are
// spelt into one key here and read back out.

import { t } from "@neokapi/i18n-react/runtime";
import type { VoiceBindingOption } from "@neokapi/ui-primitives";
import type { VoiceBindingSpec } from "../types/api";
import type { RecipeGovernance } from "./GovernanceSettings";

export function encodeVoiceBinding(binding: VoiceBindingSpec | undefined): string | undefined {
  if (binding?.profile_file) return `file:${binding.profile_file}`;
  if (binding?.pack) return `pack:${binding.pack}`;
  if (binding?.profile) return `store:${binding.profile}`;
  return undefined;
}

export function decodeVoiceBinding(key: string | undefined): VoiceBindingSpec | undefined {
  if (!key) return undefined;
  const sep = key.indexOf(":");
  const kind = sep === -1 ? "" : key.slice(0, sep);
  const value = sep === -1 ? key : key.slice(sep + 1);
  if (kind === "file") return { profile_file: value };
  if (kind === "pack") return { pack: value };
  return { profile: value };
}

/** The profiles a recipe can bind: its voice files, then the starter packs. */
export function voiceBindingOptions(governance: RecipeGovernance): VoiceBindingOption[] {
  return [
    ...governance.voice_files.map((f) => ({
      value: `file:${f}`,
      label: f,
      group: t("Files"),
    })),
    ...governance.packs.map((p) => ({
      value: `pack:${p}`,
      label: p,
      group: t("Starter packs"),
      hint: t("read-only"),
    })),
  ];
}

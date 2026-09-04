import { describe, it, expect } from "vitest";
import {
  decodeVoiceBinding,
  encodeVoiceBinding,
  voiceBindingOptions,
} from "../components/voiceBinding";

describe("voice binding keys", () => {
  it("round-trips the three forms of a binding", () => {
    for (const spec of [
      { profile_file: ".kapi/voice.yaml" },
      { pack: "technical-docs" },
      { profile: "support" },
    ]) {
      expect(decodeVoiceBinding(encodeVoiceBinding(spec))).toEqual(spec);
    }
  });

  it("spells nothing bound as no key, both ways", () => {
    expect(encodeVoiceBinding(undefined)).toBeUndefined();
    expect(encodeVoiceBinding({})).toBeUndefined();
    expect(decodeVoiceBinding(undefined)).toBeUndefined();
    expect(decodeVoiceBinding("")).toBeUndefined();
  });

  it("keeps a colon inside a file path", () => {
    expect(decodeVoiceBinding("file:C:/voice.yaml")).toEqual({ profile_file: "C:/voice.yaml" });
  });

  it("offers the recipe's files first, then the packs as read-only", () => {
    const options = voiceBindingOptions({
      axes: [],
      channels: [],
      profiles: ["support"],
      voice_files: [".kapi/voice.yaml"],
      packs: ["technical-docs"],
    });
    expect(options.map((o) => o.value)).toEqual(["file:.kapi/voice.yaml", "pack:technical-docs"]);
    expect(options[1].hint).toBe("read-only");
  });
});

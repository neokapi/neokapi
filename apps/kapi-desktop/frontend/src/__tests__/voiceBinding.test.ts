import { describe, it, expect } from "vitest";
import {
  decodeVoiceBinding,
  encodeVoiceBinding,
  voiceBindingOptions,
} from "../components/voiceBinding";

describe("voice binding keys", () => {
  it("round-trips the two forms of a binding", () => {
    for (const spec of [{ pack: "technical-docs" }, { profile: "support" }]) {
      expect(decodeVoiceBinding(encodeVoiceBinding(spec))).toEqual(spec);
    }
  });

  it("spells nothing bound as no key, both ways", () => {
    expect(encodeVoiceBinding(undefined)).toBeUndefined();
    expect(encodeVoiceBinding({})).toBeUndefined();
    expect(decodeVoiceBinding(undefined)).toBeUndefined();
    expect(decodeVoiceBinding("")).toBeUndefined();
  });

  it("keeps a colon inside a profile name", () => {
    expect(decodeVoiceBinding("store:acme:docs")).toEqual({ profile: "acme:docs" });
  });

  it("offers the stored profiles first, then the packs as read-only", () => {
    const options = voiceBindingOptions({
      axes: [],
      channels: [],
      profiles: ["support"],
      voice_profiles: ["northsea"],
      packs: ["technical-docs"],
    });
    expect(options.map((o) => o.value)).toEqual(["store:northsea", "pack:technical-docs"]);
    expect(options[1].hint).toBe("read-only");
  });
});

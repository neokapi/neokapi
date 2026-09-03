import { render, screen } from "./testUtils";
import { describe, it, expect } from "vitest";
import type { VoicePoint } from "../types/voice";
import { ResolutionHeader } from "../components/voice/resolution-header";

const clean: VoicePoint = {
  label: "project default",
  point: { default: true, ref: "defaults.voice" },
  collections: ["App"],
  field: "defaults.voice",
  source: "/w/northsea/.kapi/voice.yaml",
  binding: { kind: "profile_file", value: ".kapi/voice.yaml" },
  edit: { writable: true, exists: true, inherited: false },
  profile: { name: "Northsea" },
};

const fellThrough: VoicePoint = {
  label: "campaign",
  point: { profile: "campaign", default: false, ref: "defaults.voice" },
  collections: [],
  field: "defaults.voice",
  binding: { kind: "profile_file", value: ".kapi/voice.yaml" },
  validity: { to: "2026-08-29T00:00:00Z", state: "expired" },
  fallback: {
    profile: "campaign",
    expired: true,
    boundary: "2026-08-29T00:00:00Z",
    governing: "",
    message: "expired",
  },
  edit: { writable: true, exists: false, inherited: true },
  profile: { name: "Northsea" },
};

describe("voice resolution header", () => {
  it("emphasizes the governing voice on a clean binding, plumbing on its own line", () => {
    render(<ResolutionHeader point={clean} />);
    expect(screen.getByTestId("voice-chain")).toHaveTextContent("Northsea");

    const plumbing = screen.getByTestId("voice-plumbing");
    expect(plumbing).toHaveTextContent("defaults.voice");
    expect(plumbing).toHaveTextContent("profile_file");
    expect(plumbing).toHaveTextContent(".kapi/voice.yaml");
  });

  it("marks the superseded binding and the date its window closed", () => {
    render(<ResolutionHeader point={fellThrough} />);
    const chain = screen.getByTestId("voice-chain");
    expect(chain).toHaveTextContent("campaign");
    expect(chain).toHaveTextContent("window closed");
    expect(chain).toHaveTextContent("2026-08-29");
    // The voice that governs instead is named.
    expect(chain).toHaveTextContent("project default");
    expect(screen.getByTestId("voice-validity")).toHaveTextContent("expired");
  });
});

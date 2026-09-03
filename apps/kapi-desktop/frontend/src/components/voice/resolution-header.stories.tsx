import type { Meta, StoryObj } from "@storybook/react-vite";
import type { VoicePoint } from "../../types/voice";
import { ResolutionHeader } from "./resolution-header";

const meta: Meta = {
  title: "Voice/Resolution header",
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj;

const clean: VoicePoint = {
  label: "project default",
  point: { default: true, ref: "defaults.voice" },
  coordinates: { brand: "northsea" },
  collections: ["App", "Promo"],
  field: "defaults.voice",
  source: "/w/northsea/.kapi/voice.yaml",
  binding: { kind: "profile_file", value: ".kapi/voice.yaml" },
  termstore: ".kapi/terms.json",
  edit: { writable: true, exists: true, inherited: false },
  profile: { name: "Northsea" },
};

const fellThrough: VoicePoint = {
  label: "campaign",
  point: { profile: "campaign", default: false, ref: "defaults.voice" },
  coordinates: { brand: "northsea", product: "campaign" },
  channels: ["promo"],
  collections: [],
  field: "defaults.voice",
  source: "/w/northsea/.kapi/voice.yaml",
  binding: { kind: "profile_file", value: ".kapi/voice.yaml" },
  validity: { to: "2026-08-29T00:00:00Z", state: "expired" },
  fallback: {
    profile: "campaign",
    expired: true,
    boundary: "2026-08-29T00:00:00Z",
    governing: "",
    message: 'profile "campaign" expired 2026-08-29; governing with the project default',
  },
  edit: { writable: true, exists: false, inherited: true },
  profile: { name: "Northsea" },
};

export const CleanBinding: Story = {
  name: "Clean binding",
  render: () => (
    <div className="max-w-2xl">
      <ResolutionHeader point={clean} />
    </div>
  ),
};

export const FellThrough: Story = {
  name: "Fell through (window closed)",
  render: () => (
    <div className="max-w-2xl">
      <ResolutionHeader point={fellThrough} />
    </div>
  ),
};

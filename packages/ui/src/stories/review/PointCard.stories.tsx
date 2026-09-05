import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReviewPoint } from "@neokapi/contract-types";
import { PointCard } from "../../components/review";

/** One point as the host serialises it (core/review.Point). */
const governed: ReviewPoint = {
  ref: "retail/web",
  default: false,
  path: "content/checkout/en.json",
  collection: "Product UI",
  coordinates: { product: "kapimart", channel: "web", brand: "northsea" },
  voice: {
    name: "Kapimart retail",
    source: "pack:retail",
    guide:
      "Voice profile (personality: precise, plain; formality: neutral; use active voice). " +
      "Tone guidance: say what the product does for the reader, in their words. " +
      'Never use these terms (use the replacement): "cart" → "basket"; "sign in" → "log in".',
  },
  term_rules: [
    { term: "cart", replacement: "basket", severity: "major", note: "The store's own word." },
    { term: "sign in", replacement: "log in", severity: "minor" },
    { term: "checkout", replacement: "pay" },
    { term: "Kapimart", do_not_translate: true },
  ],
  terms_total: 40,
  profiles: [{ name: "retail", state: "active", valid_from: "2026-01-01" }],
  notes: ["The channel was derived from the collection's channel."],
};

/** The platform's rows beside the point: the terms the source matches, and the unit's score against the bar. */
const platformRows = {
  termHits: [{ term: "password", renderings: ["mot de passe"], domain: "account" }],
  voiceScore: 62,
  voiceBar: 90,
};

const meta: Meta<typeof PointCard> = {
  title: "Review/PointCard",
  component: PointCard,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Where the unit's file sits and what governs it there. The summary is the address as coordinate chips; behind it the voice in force with its guidance and the unit's score against the bar, the term rules bearing on the wording, the terms the source matches, the validity windows and the resolution's notes. Kapi desktop and the platform draw this one card.",
      },
    },
  },
  render: (args) => (
    <div className="w-[28rem]">
      <PointCard {...args} />
    </div>
  ),
};

export default meta;
type Story = StoryObj<typeof PointCard>;

export const Governed: Story = {
  name: "Governed point, score below the bar (platform)",
  args: { point: governed, ...platformRows },
};

export const GovernedOnTheDesktop: Story = {
  name: "Governed point as the desktop draws it: no score, no term hits",
  args: { point: governed },
};

export const DefaultPoint: Story = {
  name: "The project's default point, no voice bound",
  args: { point: { default: true, terms_total: 12 } },
};

export const Ungoverned: Story = {
  name: "Nothing resolved (platform, no term hits)",
  args: { point: { default: false, terms_total: 0 }, termHits: [] },
};

export const Loading: Story = {
  args: { loading: true },
};

export const Dark: Story = {
  globals: { theme: "dark" },
  args: { point: governed, ...platformRows },
};

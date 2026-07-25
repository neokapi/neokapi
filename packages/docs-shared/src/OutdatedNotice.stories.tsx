import type { Meta, StoryObj } from "@storybook/react-vite";
import OutdatedNotice from "./OutdatedNotice";
import { withDiagramTheme } from "./diagram/storyEnv";

/*
  The outdated-recording banner as it renders above a walkthrough player.

  The stories stand the banner on a stand-in player rather than a real
  `ThemedVideo`: that component calls Docusaurus hooks (`useBaseUrl`,
  `useDocusaurusContext`) which have no provider in Storybook. The banner itself
  is plain presentational markup, so what is on review here — the copy, the
  warning treatment, and the light/dark pairing — is exactly what ships.

  Both kinds appear side by side on purpose. Their difference is a claim about
  the footage, not a shade of severity, and the two must stay distinguishable at
  a glance: `interface` says the screens no longer exist, `wording` says only
  the narration's terminology is retired.
*/
const meta: Meta<typeof OutdatedNotice> = {
  title: "Docs/Outdated recording banner",
  component: OutdatedNotice,
  decorators: [withDiagramTheme],
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj<typeof OutdatedNotice>;

// A neutral rectangle standing in for the <video> the banner sits above, so the
// spacing and the shared width read as they do on the page. Tinted from
// `currentColor` rather than infima emphasis tokens, which Storybook does not
// load — a literal light grey would read as a white slab on the dark story.
function PlayerStandIn({ label }: { label: string }) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        aspectRatio: "16 / 9",
        borderRadius: 8,
        border: "1px solid color-mix(in srgb, currentColor 22%, transparent)",
        background: "color-mix(in srgb, currentColor 8%, transparent)",
        opacity: 0.75,
        fontSize: 13,
      }}
    >
      {label}
    </div>
  );
}

function Embed({
  kind,
  note,
  caption,
}: {
  kind: "interface" | "wording";
  note?: string;
  caption: string;
}) {
  return (
    <div style={{ maxWidth: 800, marginBottom: 40 }}>
      <div style={{ marginBottom: "0.7rem" }}>
        <OutdatedNotice kind={kind} note={note} />
      </div>
      <PlayerStandIn label={caption} />
    </div>
  );
}

export const InterfaceChanged: Story = {
  name: "Interface changed",
  render: () => <Embed kind="interface" caption="walkthrough player (kapi-desktop-projects)" />,
};

export const WordingOnly: Story = {
  name: "Wording only",
  render: () => <Embed kind="wording" caption="walkthrough player (bowrain-web-governance)" />,
};

export const WithNote: Story = {
  name: "Interface changed, with a specific note",
  render: () => (
    <Embed
      kind="interface"
      note="The Termbases section is now Terms."
      caption="walkthrough player (kapi-desktop-explorer)"
    />
  ),
};

export const BothKinds: Story = {
  name: "Both kinds, for comparison",
  render: () => (
    <>
      <Embed kind="interface" caption="interface changed" />
      <Embed kind="wording" caption="wording only" />
    </>
  ),
};

import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StoryPreview } from "../../components/editor/StoryPreview";
import type { BlockInfo } from "../../types/api";

/**
 * Reading an item inside the component that ships it.
 *
 * The frame is a published Storybook's own; the strings in it are the review
 * surface's, posted in — so what is on screen is the component as it will ship
 * if the reviewer approves what they are holding, including a draft no built
 * catalog has ever seen.
 *
 * These stories point at the Storybook serving them, so the preview renders a
 * real story from this very build.
 */
const meta: Meta<typeof StoryPreview> = {
  title: "Editor/StoryPreview",
  component: StoryPreview,
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <QueryClientProvider
        client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}
      >
        <div className="h-[680px] w-full bg-background p-4">
          <Story />
        </div>
      </QueryClientProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof StoryPreview>;

/** The credits-warning email's blocks, each carrying its message key. */
function emailBlocks(): BlockInfo[] {
  const b = (hash: string, source: string, nb: string): BlockInfo => ({
    id: hash,
    source,
    translatable: true,
    has_spans: false,
    properties: { hash, file: "src/credits-warning.tsx", component: "CreditsWarningEmail" },
    targets: { nb: { text: nb, status: "draft" } },
  });
  return [
    b(
      "90j0egM1IoD",
      "Your AI credits are running low",
      "Dine AI-kreditter er i ferd med å ta slutt",
    ),
    b("eXjOLgmiqT6", "Upgrade Plan", "Oppgrader planen"),
    b("clVnq5E37Ir", "The context graph for your content", "Kontekstgrafen for innholdet ditt"),
    b(
      "e3nbQxeay21",
      "Button not working? Copy and paste this link into your browser:",
      "Fungerer ikke knappen? Kopier og lim inn denne lenken i nettleseren:",
    ),
  ];
}

/** The reviewer's pending Norwegian, rendered in the email it ships in. */
export const PendingTarget: Story = {
  args: {
    storybookURL: typeof window === "undefined" ? "" : window.location.origin,
    blocks: emailBlocks(),
    locale: "nb",
    source: false,
  },
};

/** The same component read as authored — no dictionary at all. */
export const Source: Story = {
  args: {
    storybookURL: typeof window === "undefined" ? "" : window.location.origin,
    blocks: emailBlocks(),
    locale: "nb",
    source: true,
  },
};

/** An item whose components no story renders: said plainly, not left blank. */
export const NoStory: Story = {
  args: {
    storybookURL: typeof window === "undefined" ? "" : window.location.origin,
    blocks: [
      {
        id: "x",
        source: "Somewhere else",
        translatable: true,
        has_spans: false,
        properties: { hash: "zz", file: "src/NotAComponentWithStories.tsx" },
        targets: {},
      },
    ],
    locale: "nb",
    source: false,
  },
};

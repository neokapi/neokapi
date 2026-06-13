import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Decorator } from "@storybook/react";
import { BlastRadiusPanel } from "./BlastRadiusPanel";
import type { ChangeSetImpact } from "../../types/brand-graph";

const impact: ChangeSetImpact = {
  total_blocks: 1280,
  affected_blocks: 34,
  new_violations: 12,
  resolved: 7,
  words: 210,
  projects: [
    {
      project_id: "p-web",
      project_name: "Marketing Website",
      affected_blocks: 22,
      new_violations: 8,
      resolved: 5,
      words: 140,
      collections: [
        {
          collection_id: "col-1",
          collection_name: "Pages",
          affected_blocks: 22,
          new_violations: 8,
          resolved: 5,
          words: 140,
          locales: [
            {
              stream: "main",
              locale: "de-DE",
              affected_blocks: 14,
              new_violations: 5,
              resolved: 3,
              words: 90,
            },
            {
              stream: "main",
              locale: "fr-FR",
              affected_blocks: 8,
              new_violations: 3,
              resolved: 2,
              words: 50,
            },
          ],
        },
      ],
    },
    {
      project_id: "p-app",
      project_name: "Mobile App",
      affected_blocks: 12,
      new_violations: 4,
      resolved: 2,
      words: 70,
      collections: [
        {
          collection_id: "col-2",
          collection_name: "Strings",
          affected_blocks: 12,
          new_violations: 4,
          resolved: 2,
          words: 70,
          locales: [
            {
              stream: "main",
              locale: "de-DE",
              affected_blocks: 7,
              new_violations: 3,
              resolved: 1,
              words: 40,
            },
            {
              stream: "main",
              locale: "fr-FR",
              affected_blocks: 5,
              new_violations: 1,
              resolved: 1,
              words: 30,
            },
          ],
        },
      ],
    },
  ],
  samples: [
    {
      project_id: "p-web",
      stream: "main",
      collection_id: "col-1",
      collection_name: "Pages",
      locale: "de-DE",
      item_name: "pricing.de.json",
      block_id: "b-1",
      text: "Wir utilize modernste Technologie für Ihren Checkout.",
      new_violations: 1,
    },
  ],
};

const pad: Decorator = (Story) => (
  <div style={{ maxWidth: 640, padding: 24 }}>
    <Story />
  </div>
);

const meta: Meta<typeof BlastRadiusPanel> = {
  title: "Brand Hub/Experiments/BlastRadiusPanel",
  component: BlastRadiusPanel,
  tags: ["autodocs"],
  decorators: [pad],
};

export default meta;
type Story = StoryObj<typeof BlastRadiusPanel>;

export const Default: Story = { args: { impact } };

export const Loading: Story = { args: { isLoading: true } };

export const NoImpact: Story = {
  args: {
    impact: {
      ...impact,
      affected_blocks: 0,
      new_violations: 0,
      resolved: 0,
      words: 0,
      samples: [],
    },
  },
};

export const Pending: Story = { args: {} };

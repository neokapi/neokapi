import type { Meta, StoryObj } from "@storybook/react-vite";
import { TranslationStatusPanel, type ProjectStatus } from "../components/TranslationStatusPanel";

const meta: Meta<typeof TranslationStatusPanel> = {
  title: "Project/TranslationStatusPanel",
  component: TranslationStatusPanel,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 640 }}>
        <Story />
      </div>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof TranslationStatusPanel>;

const mixedStatus: ProjectStatus = {
  projectPath: "/Users/dev/app/translation.kapi",
  projectName: "My App Localization",
  collections: [
    {
      name: "ui",
      blockCount: 1007,
      coverage: { fr: 987, ja: 1007 },
      targetLanguages: ["fr", "de", "ja"],
    },
    {
      name: "marketing",
      blockCount: 42,
      coverage: {},
      targetLanguages: ["fr", "de"],
    },
    {
      name: "docs",
      targetLanguages: ["fr", "de"],
    },
  ],
};

export const Default: Story = {
  args: { tabID: "storybook", status: mixedStatus },
};

export const AllComplete: Story = {
  args: {
    tabID: "storybook",
    status: {
      projectPath: "/Users/dev/app/translation.kapi",
      projectName: "Polyglot",
      collections: [
        {
          name: "ui",
          blockCount: 250,
          coverage: { fr: 250, de: 250, ja: 250 },
          targetLanguages: ["fr", "de", "ja"],
        },
      ],
    },
  },
};

export const Empty: Story = {
  args: {
    tabID: "storybook",
    status: {
      projectPath: "/Users/dev/app/translation.kapi",
      projectName: "New Project",
      collections: [],
    },
  },
};

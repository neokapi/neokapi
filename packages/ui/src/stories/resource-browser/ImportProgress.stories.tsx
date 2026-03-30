import type { Meta, StoryObj } from "@storybook/react-vite";
import { ImportProgress } from "../../components/resource-browser/ImportProgress";

const meta: Meta<typeof ImportProgress> = {
  title: "Resource Browser/ImportProgress",
  component: ImportProgress,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Modal overlay with an indeterminate progress bar shown during import operations. Displays optional file name and imported entry count.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof ImportProgress>;

export const Active: Story = {
  args: {
    active: true,
  },
};

export const WithFileName: Story = {
  args: {
    active: true,
    fileName: "project-memory.tmx",
  },
};

export const WithCount: Story = {
  args: {
    active: true,
    fileName: "enterprise-translations.tmx",
    importedCount: 1847,
  },
};

export const WithCloseButton: Story = {
  args: {
    active: true,
    fileName: "glossary-export.csv",
    importedCount: 523,
    onClose: () => {},
  },
};

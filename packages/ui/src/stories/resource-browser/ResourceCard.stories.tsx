import type { Meta, StoryObj } from "@storybook/react-vite";
import { ResourceCard } from "../../components/resource-browser/ResourceCard";
import { Database, BookOpen } from "lucide-react";

const meta: Meta<typeof ResourceCard> = {
  title: "Resource Browser/ResourceCard",
  component: ResourceCard,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 400, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
  parameters: {
    docs: {
      description: {
        component:
          "Card for the resource picker landing page. Shows resource name, path, entry count, last modified time, and file size. Used for both TM databases and termbases.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof ResourceCard>;

export const TMCard: Story = {
  args: {
    name: "my-project",
    path: "~/.config/kapi/tm/my-project.db",
    entryCount: 1284,
    size: 524288,
    modified: new Date(Date.now() - 3600000).toISOString(),
    icon: <Database size={18} />,
    onClick: () => {},
  },
};

export const TermbaseCard: Story = {
  args: {
    name: "brand-glossary",
    path: "~/.config/kapi/termbases/brand-glossary.db",
    entryCount: 347,
    size: 131072,
    modified: new Date(Date.now() - 172800000).toISOString(),
    icon: <BookOpen size={18} />,
    onClick: () => {},
  },
};

export const LargeFile: Story = {
  args: {
    name: "enterprise-tm",
    path: "/shared/localization/enterprise-tm.db",
    entryCount: 284619,
    size: 157286400,
    modified: new Date(Date.now() - 86400000).toISOString(),
    icon: <Database size={18} />,
    onClick: () => {},
  },
};

export const Loading: Story = {
  args: {
    name: "",
    path: "",
    loading: true,
    onClick: () => {},
  },
};

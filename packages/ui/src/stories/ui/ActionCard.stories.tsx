import type { Meta, StoryObj } from "@storybook/react-vite";
import { ActionCard } from "../../components/ui/action-card";
import { Badge } from "../../components/ui/badge";
import {
  FolderInput,
  FolderOutput,
  FileBox,
  Sparkles,
  FileText,
  Workflow,
  Wrench,
  Settings2,
} from "lucide-react";

const meta: Meta<typeof ActionCard> = {
  title: "Foundations/ActionCard",
  component: ActionCard,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Clickable card for templates, presets, and quick actions. Supports icon, title, description, badge, loading, and highlighted states.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof ActionCard>;

export const Template: Story = {
  name: "Template Card",
  render: () => (
    <div className="max-w-lg space-y-3">
      <ActionCard
        icon={
          <div className="flex items-center gap-1.5">
            <FolderInput size={20} />
            <span className="text-xs text-muted-foreground">&rarr;</span>
            <FolderOutput size={20} />
          </div>
        }
        title="Input → Output"
        description="Source files in ./input/, translations written to ./output/{lang}/."
        onClick={() => {}}
      />
      <ActionCard
        icon={<FileBox size={20} />}
        title="Start empty"
        description="Blank project — configure everything yourself."
        onClick={() => {}}
      />
    </div>
  ),
};

export const PresetCards: Story = {
  name: "Preset Cards (with highlighted)",
  render: () => (
    <div className="max-w-lg space-y-3">
      <ActionCard
        icon={<Sparkles size={20} />}
        title="nextjs"
        description="Next.js App Router with next-intl"
        badge={
          <Badge variant="secondary" className="text-[10px]">
            detected
          </Badge>
        }
        highlighted
        onClick={() => {}}
      />
      <ActionCard
        icon={<Sparkles size={20} />}
        title="react-intl"
        description="React with react-intl (FormatJS)"
        onClick={() => {}}
      />
      <ActionCard
        icon={<Sparkles size={20} />}
        title="angular"
        description="Angular with @angular/localize"
        onClick={() => {}}
      />
    </div>
  ),
};

export const QuickActions: Story = {
  name: "Quick Action Grid",
  render: () => (
    <div className="grid max-w-2xl grid-cols-2 gap-3 lg:grid-cols-4">
      <ActionCard
        icon={<FileText size={16} />}
        title="Content"
        description="2 collections, 4 patterns"
        onClick={() => {}}
      />
      <ActionCard
        icon={<Workflow size={16} />}
        title="Flows"
        description="3 flows defined"
        onClick={() => {}}
      />
      <ActionCard
        icon={<Wrench size={16} />}
        title="Tools"
        description="Run individual tools"
        onClick={() => {}}
      />
      <ActionCard
        icon={<Settings2 size={16} />}
        title="Settings"
        description="Languages, plugins"
        onClick={() => {}}
      />
    </div>
  ),
};

export const Loading: Story = {
  name: "Loading State",
  args: {
    icon: <Sparkles size={20} />,
    title: "Applying preset...",
    description: "Setting up project configuration.",
    loading: true,
    onClick: () => {},
  },
};

export const Disabled: Story = {
  name: "Disabled State",
  args: {
    icon: <FileBox size={20} />,
    title: "Unavailable",
    description: "This option is not available.",
    disabled: true,
    onClick: () => {},
  },
};

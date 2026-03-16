import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  DashboardSkeleton,
  ProjectDetailSkeleton,
  EditorSkeleton,
  TablePageSkeleton,
  BrandProfilesSkeleton,
  SettingsSkeleton,
  ExplorerSkeleton,
} from "../../components/skeletons";

const meta: Meta = {
  title: "UI/Skeletons",
  tags: ["autodocs"],
  parameters: {
    layout: "fullscreen",
  },
};

export default meta;
type Story = StoryObj;

export const Dashboard: Story = {
  render: () => <DashboardSkeleton />,
};

export const ProjectDetail: Story = {
  render: () => <ProjectDetailSkeleton />,
};

export const Editor: Story = {
  render: () => (
    <div style={{ height: 600 }}>
      <EditorSkeleton />
    </div>
  ),
};

export const TablePage: Story = {
  render: () => <TablePageSkeleton />,
};

export const BrandProfiles: Story = {
  render: () => <BrandProfilesSkeleton />,
};

export const Settings: Story = {
  render: () => <SettingsSkeleton />,
};

export const Explorer: Story = {
  render: () => <ExplorerSkeleton />,
};

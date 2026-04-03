import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  DashboardSkeleton,
  ProjectDetailSkeleton,
  EditorSkeleton,
  TablePageSkeleton,
  BrandProfilesSkeleton,
  SettingsSkeleton,
  ExplorerSkeleton,
  TranslationDashboardSkeleton,
  ActivityFeedSkeleton,
  TaskBoardSkeleton,
} from "../../components/skeletons";

const meta: Meta = {
  title: "Foundations/Skeletons",
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

export const TranslationDashboard: Story = {
  render: () => <TranslationDashboardSkeleton />,
};

export const ActivityFeed: Story = {
  render: () => (
    <div style={{ maxWidth: 480 }}>
      <ActivityFeedSkeleton />
    </div>
  ),
};

export const TaskBoard: Story = {
  render: () => (
    <div style={{ maxWidth: 960 }}>
      <TaskBoardSkeleton />
    </div>
  ),
};

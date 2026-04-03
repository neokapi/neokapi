import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { Plus, FolderOpen, X, Database, FileText, Inbox, ArrowLeft } from "lucide-react";
import {
  Button,
  PageHeader,
  EmptyState,
  SkeletonCard,
  PanelHeader,
  LoadingSpinner,
} from "@neokapi/ui-primitives";

const meta: Meta = {
  title: "Foundations/Layout Components",
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj;

// ─── PageHeader ─────────────────────────────────────────────────

export const PageHeaderBasic: Story = {
  name: "PageHeader / Basic",
  render: () => (
    <PageHeader title="Translation Memories" />
  ),
};

export const PageHeaderWithActions: Story = {
  name: "PageHeader / With Actions",
  render: () => (
    <PageHeader
      title="Project Flows"
      actions={
        <>
          <Button variant="outline" size="sm">
            <FolderOpen size={12} /> Open File...
          </Button>
          <Button size="sm">
            <Plus size={12} /> New Flow
          </Button>
        </>
      }
    />
  ),
};

export const PageHeaderWithSubtitle: Story = {
  name: "PageHeader / With Subtitle",
  render: () => (
    <PageHeader
      title="AI Credentials"
      subtitle="Manage API keys for translation providers"
      actions={
        <Button size="sm">
          <Plus size={12} /> Add Provider
        </Button>
      }
    />
  ),
};

export const PageHeaderWithBackButton: Story = {
  name: "PageHeader / With Back Button",
  render: () => (
    <PageHeader
      title="my-glossary"
      backButton={
        <Button variant="ghost" size="icon-xs" onClick={fn()}>
          <X size={16} />
        </Button>
      }
      actions={
        <Button variant="outline" size="sm">Export</Button>
      }
    />
  ),
};

// ─── EmptyState ─────────────────────────────────────────────────

export const EmptyStateBasic: Story = {
  name: "EmptyState / Basic",
  render: () => (
    <div style={{ maxWidth: 500 }}>
      <EmptyState
        title="No flows yet"
        description="Create a flow to start processing files."
      />
    </div>
  ),
};

export const EmptyStateWithIcon: Story = {
  name: "EmptyState / With Icon",
  render: () => (
    <div style={{ maxWidth: 500 }}>
      <EmptyState
        icon={<Database size={32} />}
        title="No translation memories"
        description="Import a TMX file or create a new TM to get started."
        action={
          <Button size="sm">
            <Plus size={12} /> Create TM
          </Button>
        }
      />
    </div>
  ),
};

export const EmptyStateWithAction: Story = {
  name: "EmptyState / With Action",
  render: () => (
    <div style={{ maxWidth: 500 }}>
      <EmptyState
        icon={<Inbox size={32} />}
        title="No projects open"
        description="Open an existing project or create a new one."
        action={
          <div className="flex gap-2 justify-center">
            <Button variant="outline" size="sm">
              <FolderOpen size={12} /> Open...
            </Button>
            <Button size="sm">
              <Plus size={12} /> New Project
            </Button>
          </div>
        }
      />
    </div>
  ),
};

// ─── SkeletonCard ───────────────────────────────────────────────

export const SkeletonCardDefault: Story = {
  name: "SkeletonCard / Default",
  render: () => (
    <div className="grid grid-cols-2 gap-3" style={{ maxWidth: 500 }}>
      <SkeletonCard />
      <SkeletonCard />
      <SkeletonCard />
    </div>
  ),
};

export const SkeletonCardVariants: Story = {
  name: "SkeletonCard / Line Variants",
  render: () => (
    <div className="space-y-3" style={{ maxWidth: 300 }}>
      <SkeletonCard lines={2} />
      <SkeletonCard lines={3} />
      <SkeletonCard lines={5} />
    </div>
  ),
};

// ─── PanelHeader ────────────────────────────────────────────────

export const PanelHeaderBasic: Story = {
  name: "PanelHeader / Basic",
  render: () => (
    <div className="w-80 border border-border rounded-lg overflow-hidden">
      <PanelHeader title="Configuration" />
      <div className="p-3 text-xs text-muted-foreground">Panel content here...</div>
    </div>
  ),
};

export const PanelHeaderWithActions: Story = {
  name: "PanelHeader / With Actions",
  render: () => (
    <div className="w-80 border border-border rounded-lg overflow-hidden">
      <PanelHeader
        title="Part Inspector"
        actions={
          <Button variant="ghost" size="icon-xs" onClick={fn()}>
            <X size={12} />
          </Button>
        }
      />
      <div className="p-3 text-xs text-muted-foreground">Inspector content...</div>
    </div>
  ),
};

export const PanelHeaderWithChildren: Story = {
  name: "PanelHeader / With Custom Content",
  render: () => (
    <div className="w-96 border border-border rounded-lg overflow-hidden">
      <PanelHeader>
        <span className="text-xs font-semibold text-muted-foreground">Preview</span>
        <span className="text-[10px] text-muted-foreground">source.json</span>
      </PanelHeader>
      <div className="p-3 text-xs text-muted-foreground">Preview content...</div>
    </div>
  ),
};

// ─── LoadingSpinner ─────────────────────────────────────────────

export const LoadingSpinnerSizes: Story = {
  name: "LoadingSpinner / Sizes",
  render: () => (
    <div className="space-y-4 p-4">
      <LoadingSpinner size="sm" text="Loading..." />
      <LoadingSpinner size="md" text="Loading tools..." />
      <LoadingSpinner size="lg" text="Initializing..." />
    </div>
  ),
};

export const LoadingSpinnerNoText: Story = {
  name: "LoadingSpinner / No Text",
  render: () => (
    <div className="flex gap-6 p-4">
      <LoadingSpinner size="sm" />
      <LoadingSpinner size="md" />
      <LoadingSpinner size="lg" />
    </div>
  ),
};

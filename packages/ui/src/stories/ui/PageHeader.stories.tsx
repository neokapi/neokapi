import type { Meta, StoryObj } from "@storybook/react-vite";
import { Globe, ArrowLeft, Plus } from "lucide-react";
import { PageHeader } from "../../components/PageHeader";
import { SectionHeading } from "../../components/SectionHeading";
import { Button } from "../../components/ui/button";

// The two shared headings every app surface uses: PageHeader owns the page
// title (h1) and its actions; SectionHeading is the small uppercase eyebrow
// (h2) that labels a group beneath it. Using them keeps every page and section
// label the same size and weight across kapi desktop and the platform.

const meta: Meta = {
  title: "Foundations/Headings",
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "PageHeader (the page title, subtitle and actions) and SectionHeading (the in-page section eyebrow). Shared across surfaces so headings never drift.",
      },
    },
  },
};

export default meta;
type Story = StoryObj;

export const Page: Story = {
  name: "PageHeader",
  render: () => (
    <div className="max-w-3xl p-4">
      <PageHeader title="Content memory" />
    </div>
  ),
};

export const PageWithSubtitleAndActions: Story = {
  render: () => (
    <div className="max-w-3xl p-4">
      <PageHeader
        title="Content memory"
        subtitle="Approved wording, reused across projects"
        actions={
          <Button size="sm">
            <Plus className="mr-1 size-3.5" />
            New entry
          </Button>
        }
      />
    </div>
  ),
};

export const PageWithBackButton: Story = {
  render: () => (
    <div className="max-w-3xl p-4">
      <PageHeader
        title="Support · docs"
        backButton={
          <Button variant="ghost" size="icon-xs" aria-label="Back">
            <ArrowLeft className="size-4" />
          </Button>
        }
      />
    </div>
  ),
};

// The empty state's opening title: the same h1, at hero scale, centred above a
// lead paragraph and the first thing to do.
export const PageHero: Story = {
  render: () => (
    <div className="p-8">
      <PageHeader
        variant="hero"
        eyebrow="Acme"
        title="Set up your workspace"
        subtitle="Bowrain keeps translated content converging on your brand. Start with your AI assistant, with your files, or with your team."
        actions={
          <Button size="sm">
            <Plus className="mr-1 size-3.5" />
            New project
          </Button>
        }
      />
    </div>
  ),
};

export const PageHeroDark: Story = {
  globals: { theme: "dark" },
  render: () => (
    <div className="p-8">
      <PageHeader
        variant="hero"
        eyebrow="Acme"
        title="Set up your workspace"
        subtitle="Bowrain keeps translated content converging on your brand. Start with your AI assistant, with your files, or with your team."
      />
    </div>
  ),
};

export const Section: Story = {
  name: "SectionHeading",
  render: () => (
    <div className="max-w-3xl space-y-2 p-4">
      <SectionHeading icon={<Globe size={14} />} count={12}>
        Where content sits
      </SectionHeading>
      <p className="text-sm text-muted-foreground">A section body sits beneath the eyebrow.</p>
    </div>
  ),
};

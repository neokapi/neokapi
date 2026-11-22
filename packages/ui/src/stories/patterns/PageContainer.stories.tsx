import type { Meta, StoryObj } from "@storybook/react-vite";
import { PageContainer, SettingsRow, type PageWidth } from "../../components/ui/page-container";
import { PageHeader } from "../../components/PageHeader";
import { Input } from "../../components/ui/input";

/**
 * The column a page sits in. A page names its width once, by its shape, and
 * the container centres it with the shared side padding.
 */
const meta: Meta<typeof PageContainer> = {
  title: "Patterns/PageContainer",
  component: PageContainer,
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj<typeof PageContainer>;

const WIDTHS: PageWidth[] = ["narrow", "standard", "wide"];

/** The three bounded widths, one above the other, on a wide window. */
export const Widths: Story = {
  render: () => (
    <div className="min-h-screen bg-background text-foreground">
      {WIDTHS.map((w) => (
        <PageContainer key={w} width={w}>
          <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
            {w}
          </div>
        </PageContainer>
      ))}
    </div>
  ),
};

/** A settings page: labels on the left, controls filling the column. */
export const SettingsPage: Story = {
  render: () => (
    <div className="min-h-screen bg-background text-foreground">
      <PageContainer width="standard">
        <PageHeader title="Project settings" />
        <div className="divide-y">
          <SettingsRow label="Source language" htmlFor="src">
            <Input id="src" defaultValue="English" />
          </SettingsRow>
          <SettingsRow label="Default encoding" help="Used when a file does not declare one.">
            <Input defaultValue="Auto-detect" />
          </SettingsRow>
        </div>
      </PageContainer>
    </div>
  ),
};

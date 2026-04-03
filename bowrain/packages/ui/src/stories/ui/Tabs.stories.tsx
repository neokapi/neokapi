import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
} from "@neokapi/ui-primitives/components/ui/tabs";

const meta: Meta<typeof Tabs> = {
  title: "Foundations/Tabs",
  component: Tabs,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 500, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof Tabs>;

export const Default: Story = {
  render: () => (
    <Tabs defaultValue="source">
      <TabsList>
        <TabsTrigger value="source">Source</TabsTrigger>
        <TabsTrigger value="target">Target</TabsTrigger>
        <TabsTrigger value="preview">Preview</TabsTrigger>
      </TabsList>
      <TabsContent value="source">
        <p className="text-sm text-muted-foreground p-4">Source text content</p>
      </TabsContent>
      <TabsContent value="target">
        <p className="text-sm text-muted-foreground p-4">Target translation content</p>
      </TabsContent>
      <TabsContent value="preview">
        <p className="text-sm text-muted-foreground p-4">Formatted preview</p>
      </TabsContent>
    </Tabs>
  ),
};

export const Glass: Story = {
  render: () => (
    <Tabs defaultValue="tm">
      <TabsList>
        <TabsTrigger value="tm">Translation Memory</TabsTrigger>
        <TabsTrigger value="terms">Terminology</TabsTrigger>
      </TabsList>
      <TabsContent value="tm">
        <p className="text-sm text-muted-foreground p-4">TM matches will appear here</p>
      </TabsContent>
      <TabsContent value="terms">
        <p className="text-sm text-muted-foreground p-4">Term suggestions will appear here</p>
      </TabsContent>
    </Tabs>
  ),
};

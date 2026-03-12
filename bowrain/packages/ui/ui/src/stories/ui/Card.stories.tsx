import type { Meta, StoryObj } from "@storybook/react-vite";
import { Card, CardHeader, CardTitle, CardDescription, CardContent, CardFooter, GlassCard } from "../../components/ui/card";
import { Button } from "../../components/ui/button";

const meta: Meta<typeof Card> = {
  title: "UI/Card",
  component: Card,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 400, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof Card>;

export const Default: Story = {
  render: () => (
    <Card>
      <CardHeader>
        <CardTitle>Project Settings</CardTitle>
        <CardDescription>Configure your localization project</CardDescription>
      </CardHeader>
      <CardContent>
        <p className="text-sm text-muted-foreground">
          Source locale: en-US. Target locales: fr-FR, de-DE, ja-JP.
        </p>
      </CardContent>
      <CardFooter>
        <Button>Save Changes</Button>
      </CardFooter>
    </Card>
  ),
};

export const Glass: Story = {
  render: () => (
    <GlassCard>
      <CardHeader>
        <CardTitle>Translation Memory</CardTitle>
        <CardDescription>1,234 entries across 3 locale pairs</CardDescription>
      </CardHeader>
      <CardContent>
        <p className="text-sm text-muted-foreground">
          Last updated 2 hours ago. 98% fuzzy match rate.
        </p>
      </CardContent>
    </GlassCard>
  ),
};

export const Minimal: Story = {
  render: () => (
    <Card>
      <CardContent className="pt-6">
        <p className="text-sm">A simple card with only content.</p>
      </CardContent>
    </Card>
  ),
};

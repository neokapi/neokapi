import type { Meta, StoryObj } from "@storybook/react-vite";
import { TriangleAlert } from "lucide-react";
import { Badge } from "../../components/ui/badge";

/**
 * The small label a row carries: a count, a state, a kind. `warning` is the
 * chip for something waiting on a person, such as "1 needs you": the amber
 * border and the `--warning-ink` amber, on the page's own ground.
 */
const meta: Meta<typeof Badge> = {
  title: "Foundations/Badge",
  component: Badge,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
  args: { children: "1 needs you", variant: "warning" },
  argTypes: {
    variant: {
      control: "inline-radio",
      options: ["default", "secondary", "destructive", "outline", "warning", "ghost", "link"],
    },
  },
};
export default meta;

type Story = StoryObj<typeof Badge>;

/** One badge, with the variant from the controls. */
export const Playground: Story = {};

function Variants() {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Badge>default</Badge>
      <Badge variant="secondary">secondary</Badge>
      <Badge variant="destructive">destructive</Badge>
      <Badge variant="outline" className="text-muted-foreground">
        3 names
      </Badge>
      <Badge variant="warning">1 needs you</Badge>
      <Badge variant="warning">
        <TriangleAlert aria-hidden />
        Drifting
      </Badge>
    </div>
  );
}

/** Every variant side by side, as the home's chips set them beside each other. */
export const AllVariants: Story = { render: () => <Variants /> };

/** The same in the dark theme, where the warning ink is the amber as it is. */
export const AllVariantsDark: Story = {
  render: () => <Variants />,
  globals: { theme: "dark" },
};

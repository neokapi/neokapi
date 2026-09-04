import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { ViewTab, ViewTabGroup } from "../../components/ui/view-tab";

const meta: Meta<typeof ViewTabGroup> = {
  title: "Foundations/ViewTab",
  component: ViewTabGroup,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "A compact view switch: a pill group of small toggle buttons for choosing which reading of one thing to show. Used by the data preview (Keys / File) and the flow editor (Steps / Diagram / Run).",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof ViewTabGroup>;

function Demo({ choices }: { choices: string[] }) {
  const [active, setActive] = useState(choices[0]);
  return (
    <ViewTabGroup aria-label="View">
      {choices.map((c) => (
        <ViewTab key={c} active={active === c} onClick={() => setActive(c)}>
          {c}
        </ViewTab>
      ))}
    </ViewTabGroup>
  );
}

export const TwoChoices: Story = {
  render: () => <Demo choices={["Keys", "File"]} />,
};

export const ThreeChoices: Story = {
  render: () => <Demo choices={["Steps", "Diagram", "Run"]} />,
};

export const Dark: Story = {
  globals: { theme: "dark" },
  render: () => <Demo choices={["Steps", "Diagram", "Run"]} />,
};

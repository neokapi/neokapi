import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { TMSearchBar } from "../../components/resource-browser/TMSearchBar";
import { Button } from "../../components/ui/button";

const meta: Meta<typeof TMSearchBar> = {
  title: "Resource Browser/TMSearchBar",
  component: TMSearchBar,
  tags: ["autodocs"],
  decorators: [
    (Story: React.ComponentType) => (
      <div style={{ maxWidth: 600, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
  parameters: {
    docs: {
      description: {
        component:
          "Combined search bar with inline entity annotation. Select text to mark entities; press Enter to trigger fuzzy TM lookup.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof TMSearchBar>;

export const Default: Story = {
  render: () => {
    const [value, setValue] = useState("");
    return (
      <TMSearchBar
        value={value}
        onChange={setValue}
        sourceLocale="en-US"
        targetLocale="fr-FR"
      />
    );
  },
};

export const WithActions: Story = {
  render: () => {
    const [value, setValue] = useState("");
    return (
      <TMSearchBar
        value={value}
        onChange={setValue}
        sourceLocale="en-US"
        targetLocale="fr-FR"
        actions={
          <Button size="sm">Add Entry</Button>
        }
      />
    );
  },
};

export const WithLookup: Story = {
  render: () => {
    const [value, setValue] = useState("John works at Acme Corp");
    return (
      <TMSearchBar
        value={value}
        onChange={setValue}
        sourceLocale="en-US"
        targetLocale="fr-FR"
        onLookup={async () => [
          {
            entry: {
              id: "1",
              source_text: "Bob works at Widget Inc",
              target_text: "Bob travaille chez Widget Inc",
              source_coded: "",
              target_coded: "",
              source_spans: [],
              target_spans: [],
              source_locale: "en-US",
              target_locale: "fr-FR",
              project_id: "",
              created_at: new Date().toISOString(),
              updated_at: new Date().toISOString(),
            },
            score: 0.85,
            match_type: "generalized-fuzzy",
          },
        ]}
      />
    );
  },
  parameters: {
    docs: {
      description: {
        story: "Select text to mark entities, then press Enter to trigger lookup. Try selecting 'John' and marking as Person.",
      },
    },
  },
};

export const CustomPlaceholder: Story = {
  render: () => {
    const [value, setValue] = useState("");
    return (
      <TMSearchBar
        value={value}
        onChange={setValue}
        sourceLocale="en-US"
        targetLocale="fr-FR"
        placeholder="Type to search translations..."
      />
    );
  },
};

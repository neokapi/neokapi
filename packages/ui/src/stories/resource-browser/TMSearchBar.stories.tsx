import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { TMSearchBar } from "../../components/resource-browser/TMSearchBar";

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
      <TMSearchBar value={value} onChange={setValue} sourceLocale="en-US" targetLocale="fr-FR" />
    );
  },
};

/** Search bar with filter tokens for language and project. */
export const WithFilterTokens: Story = {
  render: () => {
    const [value, setValue] = useState("");
    const [filters, setFilters] = useState([
      { key: "language", value: "fr-FR" },
      { key: "project", value: "webapp" },
    ]);
    return (
      <TMSearchBar
        value={value}
        onChange={setValue}
        filters={filters}
        onFiltersChange={setFilters}
        filterFields={[
          {
            key: "language",
            label: "Target Language",
            values: [
              { value: "fr-FR", label: "French (fr-FR)" },
              { value: "de-DE", label: "German (de-DE)" },
              { value: "ja-JP", label: "Japanese (ja-JP)" },
            ],
          },
          {
            key: "project",
            label: "Project",
            values: [
              { value: "webapp", label: "Web App" },
              { value: "mobile", label: "Mobile" },
            ],
          },
        ]}
        sourceLocale="en-US"
        targetLocale="fr-FR"
      />
    );
  },
  parameters: {
    docs: {
      description: {
        story:
          "Filter tokens appear as inline badges at the left of the input. Click the chevron to add more filters. Backspace on an empty input removes the last token.",
      },
    },
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
              project_id: "",
              hint_src_lang: "en-US",
              variants: {
                "en-US": {
                  locale: "en-US",
                  text: "Bob works at Widget Inc",
                  runs: [{ text: "Bob works at Widget Inc" }],
                },
                "fr-FR": {
                  locale: "fr-FR",
                  text: "Bob travaille chez Widget Inc",
                  runs: [{ text: "Bob travaille chez Widget Inc" }],
                },
              },
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
        story:
          "Select text to mark entities, then press Enter to trigger lookup. Try selecting 'John' and marking as Person.",
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

/**
 * Demonstrates the entity-value filter flow. When the user marks text in
 * the search bar as an entity, the parent component receives the marked
 * entities via onEntitiesChange and converts them to search filters.
 * This gives precise entity-aware browser filtering — "find all entries
 * where Acme Corp is tagged as an Organization" — distinct from plain
 * text search.
 *
 * Select text in the input and mark it with an entity type; the Filter
 * state panel below shows how the parent would build a search filter.
 */
export const EntityValueFilter: Story = {
  render: () => {
    const [value, setValue] = useState("Acme Corp hired John");
    const [entities, setEntities] = useState<
      Array<{ text: string; type: string; start: number; end: number }>
    >([]);
    return (
      <div>
        <TMSearchBar
          value={value}
          onChange={setValue}
          onEntitiesChange={setEntities}
          onLookup={async () => []}
          sourceLocale="en-US"
          targetLocale="fr-FR"
        />
        <div className="mt-4 rounded-lg border bg-muted/30 p-3">
          <div className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground mb-1">
            Search filter state (from onEntitiesChange)
          </div>
          <pre className="text-[11px] text-foreground">
            {JSON.stringify(
              {
                entity_values: entities.map((e) => ({ value: e.text, type: e.type })),
              },
              null,
              2,
            )}
          </pre>
        </div>
      </div>
    );
  },
  parameters: {
    docs: {
      description: {
        story:
          "Select 'Acme Corp' and mark as Organization, then 'John' as Person. The Filter state panel updates live, showing how the parent component would build a TMSearchFilter to pass to the backend.",
      },
    },
  },
};

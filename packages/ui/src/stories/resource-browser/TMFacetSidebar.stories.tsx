import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { TMFacetSidebar, EMPTY_FACETS, type FacetSelection } from "../../components/resource-browser/TMFacetSidebar";
import type { TMFacets } from "../../components/resource-browser/types";

const SAMPLE_FACETS: TMFacets = {
  locale_pairs: [
    { source_locale: "en-US", target_locale: "fr-FR", count: 42 },
    { source_locale: "en-US", target_locale: "de-DE", count: 38 },
    { source_locale: "en-US", target_locale: "ja-JP", count: 25 },
    { source_locale: "en-US", target_locale: "es-ES", count: 15 },
    { source_locale: "en-US", target_locale: "zh-CN", count: 8 },
  ],
  projects: [
    { project_id: "webapp", count: 80 },
    { project_id: "mobile", count: 35 },
    { project_id: "", count: 13 },
  ],
  entity_types: [
    { type: "entity:person", count: 12 },
    { type: "entity:organization", count: 7 },
    { type: "entity:product", count: 5 },
    { type: "entity:date", count: 3 },
  ],
  has_codes: 45,
  no_codes: 83,
};

const meta: Meta<typeof TMFacetSidebar> = {
  title: "Resource Browser/TMFacetSidebar",
  component: TMFacetSidebar,
  tags: ["autodocs"],
  decorators: [
    (Story: React.ComponentType) => (
      <div style={{ width: 240, padding: 16, borderLeft: "1px solid var(--border)" }}>
        <Story />
      </div>
    ),
  ],
  parameters: {
    docs: {
      description: {
        component:
          "Right sidebar showing faceted filters for the TM browser. Sections: target locale, project, entity types, inline codes.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof TMFacetSidebar>;

export const Default: Story = {
  args: {
    facets: SAMPLE_FACETS,
    selection: EMPTY_FACETS,
  },
};

export const WithActiveFilters: Story = {
  args: {
    facets: SAMPLE_FACETS,
    selection: {
      targetLocales: ["fr-FR", "de-DE"],
      projects: ["webapp"],
      entityTypes: [],
      codeFilter: "all",
    },
  },
};

export const Interactive: Story = {
  render: () => {
    const [selection, setSelection] = useState<FacetSelection>(EMPTY_FACETS);
    return (
      <div>
        <TMFacetSidebar
          facets={SAMPLE_FACETS}
          selection={selection}
          onSelectionChange={setSelection}
        />
        <pre className="mt-4 text-[10px] text-muted-foreground">
          {JSON.stringify(selection, null, 2)}
        </pre>
      </div>
    );
  },
};

export const Empty: Story = {
  args: {
    facets: null,
    selection: EMPTY_FACETS,
  },
};

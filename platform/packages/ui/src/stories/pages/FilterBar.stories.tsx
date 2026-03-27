import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { FilterBar } from "../../components/FilterBar";
import type { FilterToken, FilterField, FilterPreset } from "../../components/FilterBar";
import { withProviders } from "../decorators";

const sampleFields: FilterField[] = [
  {
    key: "project",
    label: "Project",
    hint: "filter by project",
    values: [
      { value: "marketing-website", label: "Marketing Website" },
      { value: "mobile-app", label: "Mobile App" },
      { value: "docs", label: "Documentation" },
    ],
  },
  {
    key: "type",
    label: "Event type",
    hint: "filter by event category",
    values: [
      { value: "project", label: "Project" },
      { value: "block", label: "Block" },
      { value: "stream", label: "Stream" },
      { value: "collection", label: "Collection" },
      { value: "item", label: "Item" },
      { value: "connector", label: "Connector" },
    ],
  },
  {
    key: "actor",
    label: "Actor",
    hint: "filter by who performed the action",
    values: [
      { value: "alice@example.com", label: "Alice" },
      { value: "bob@example.com", label: "Bob" },
      { value: "ci-bot", label: "CI Bot" },
    ],
  },
  {
    key: "locale",
    label: "Locale",
    hint: "filter by language",
    values: [
      { value: "en-US", label: "English (US)" },
      { value: "fr-FR", label: "French (France)" },
      { value: "de-DE", label: "German (Germany)" },
      { value: "ja", label: "Japanese" },
    ],
  },
  {
    key: "stream",
    label: "Stream",
    hint: "filter by content stream",
    values: [
      { value: "main", label: "main" },
      { value: "feature/translations", label: "feature/translations" },
    ],
  },
];

const samplePresets: FilterPreset[] = [
  { label: "Content changes", filters: [{ key: "type", value: "block" }] },
  { label: "Project activity", filters: [{ key: "type", value: "project" }] },
  { label: "Stream operations", filters: [{ key: "type", value: "stream" }] },
  { label: "Push & sync events", filters: [{ key: "type", value: "connector" }] },
];

function FilterBarWrapper({
  fields,
  presets,
  initialFilters = [],
  initialSearch = "",
  placeholder,
}: {
  fields: FilterField[];
  presets?: FilterPreset[];
  initialFilters?: FilterToken[];
  initialSearch?: string;
  placeholder?: string;
}) {
  const [filters, setFilters] = useState<FilterToken[]>(initialFilters);
  const [search, setSearch] = useState(initialSearch);

  return (
    <div className="space-y-4">
      <FilterBar
        filters={filters}
        onFiltersChange={setFilters}
        search={search}
        onSearchChange={setSearch}
        fields={fields}
        presets={presets}
        placeholder={placeholder}
      />
      <div className="text-xs text-muted-foreground font-mono p-3 rounded-md bg-muted/30 border border-border/30">
        <div>filters: {JSON.stringify(filters)}</div>
        <div>search: {JSON.stringify(search)}</div>
      </div>
    </div>
  );
}

const meta: Meta<typeof FilterBar> = {
  title: "Pages/Translation/FilterBar",
  component: FilterBar,
  tags: ["autodocs"],
  decorators: [
    withProviders,
    (Story) => (
      <div style={{ maxWidth: 640, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof FilterBar>;

export const Default: Story = {
  render: () => <FilterBarWrapper fields={sampleFields} presets={samplePresets} />,
};

export const WithActiveFilters: Story = {
  render: () => (
    <FilterBarWrapper
      fields={sampleFields}
      presets={samplePresets}
      initialFilters={[
        { key: "project", value: "marketing-website" },
        { key: "type", value: "block" },
      ]}
    />
  ),
};

export const WithSearch: Story = {
  render: () => (
    <FilterBarWrapper
      fields={sampleFields}
      presets={samplePresets}
      initialSearch="translation error"
    />
  ),
};

export const NoPresets: Story = {
  render: () => <FilterBarWrapper fields={sampleFields} />,
};

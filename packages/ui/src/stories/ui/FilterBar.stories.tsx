import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  FilterBar,
  type FilterToken,
  type FilterField,
  type FilterPreset,
} from "../../components/ui/filter-bar";

const tmFields: FilterField[] = [
  {
    key: "source",
    label: "Source Language",
    hint: "filter by source locale",
    values: [
      { value: "en-US", label: "English (US)" },
      { value: "en-GB", label: "English (UK)" },
    ],
  },
  {
    key: "target",
    label: "Target Language",
    hint: "filter by target locale",
    values: [
      { value: "fr-FR", label: "French" },
      { value: "de-DE", label: "German" },
      { value: "ja", label: "Japanese" },
      { value: "ko", label: "Korean" },
      { value: "zh-Hans", label: "Simplified Chinese" },
    ],
  },
  {
    key: "age",
    label: "Age",
    hint: "filter by entry age",
    values: [
      { value: "today", label: "Today" },
      { value: "this-week", label: "This week" },
      { value: "this-month", label: "This month" },
      { value: "older", label: "Older than 30 days" },
    ],
  },
];

const tmPresets: FilterPreset[] = [
  {
    label: "Recent French",
    filters: [
      { key: "target", value: "fr-FR" },
      { key: "age", value: "this-week" },
    ],
  },
  { label: "Japanese entries", filters: [{ key: "target", value: "ja" }] },
  { label: "Today's work", filters: [{ key: "age", value: "today" }] },
];

const termbaseFields: FilterField[] = [
  {
    key: "locale",
    label: "Language",
    hint: "filter by term locale",
    values: [
      { value: "en", label: "English" },
      { value: "fr", label: "French" },
      { value: "de", label: "German" },
      { value: "ja", label: "Japanese" },
    ],
  },
  {
    key: "status",
    label: "Term Status",
    hint: "filter by approval status",
    values: [
      { value: "preferred", label: "Preferred" },
      { value: "approved", label: "Approved" },
      { value: "proposed", label: "Proposed" },
      { value: "deprecated", label: "Deprecated" },
      { value: "forbidden", label: "Forbidden" },
    ],
  },
  {
    key: "domain",
    label: "Domain",
    hint: "filter by subject domain",
    values: [
      { value: "ui", label: "User Interface" },
      { value: "legal", label: "Legal" },
      { value: "marketing", label: "Marketing" },
      { value: "technical", label: "Technical" },
    ],
  },
];

function TMFilterDemo() {
  const [filters, setFilters] = useState<FilterToken[]>([]);
  const [search, setSearch] = useState("");
  return (
    <div className="max-w-3xl space-y-3">
      <FilterBar
        filters={filters}
        onFiltersChange={setFilters}
        search={search}
        onSearchChange={setSearch}
        fields={tmFields}
        presets={tmPresets}
        placeholder="Search translation memory..."
      />
      <pre className="rounded bg-muted p-2 font-mono text-xs">
        filters: {JSON.stringify(filters)}
        {"\n"}search: {JSON.stringify(search)}
      </pre>
    </div>
  );
}

function TermbaseFilterDemo() {
  const [filters, setFilters] = useState<FilterToken[]>([{ key: "status", value: "preferred" }]);
  const [search, setSearch] = useState("");
  return (
    <div className="max-w-3xl space-y-3">
      <FilterBar
        filters={filters}
        onFiltersChange={setFilters}
        search={search}
        onSearchChange={setSearch}
        fields={termbaseFields}
        placeholder="Search terminology..."
      />
      <pre className="rounded bg-muted p-2 font-mono text-xs">
        filters: {JSON.stringify(filters)}
        {"\n"}search: {JSON.stringify(search)}
      </pre>
    </div>
  );
}

const meta: Meta<typeof FilterBar> = {
  title: "Foundations/FilterBar",
  component: FilterBar,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "GitHub-style filter bar with key:value syntax, autocomplete, presets, and free-text search. Use for TM, termbase, and any list filtering.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof FilterBar>;

export const TranslationMemory: Story = {
  name: "Translation Memory Filters",
  render: () => <TMFilterDemo />,
};

export const Termbase: Story = {
  name: "Termbase Filters",
  render: () => <TermbaseFilterDemo />,
};

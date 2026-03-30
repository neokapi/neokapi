import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { FormatConfigEditor } from "../components/FormatConfigEditor";
import type { ComponentSchema } from "../types/api";
import okapiData from "./fixtures/okapi-metadata.json";

// Extract a few representative filter schemas for stories
const filterSchemas = okapiData.filters as unknown as ComponentSchema[];

function findFilter(idFragment: string): ComponentSchema | undefined {
  return filterSchemas.find(
    (f) =>
      (f as unknown as Record<string, unknown>)["$id"]?.toString().includes(idFragment) ||
      f.title?.toLowerCase().includes(idFragment.toLowerCase()),
  );
}

function FormatConfigWrapper({
  schema,
  initialValues = {},
  title,
}: {
  schema: ComponentSchema;
  initialValues?: Record<string, unknown>;
  title?: string;
}) {
  const [values, setValues] = useState<Record<string, unknown>>(initialValues);
  return (
    <div style={{ maxWidth: 420 }}>
      <FormatConfigEditor
        schema={schema}
        values={values}
        onChange={setValues}
        title={title}
      />
      <pre className="mt-4 rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-40">
        {JSON.stringify(values, null, 2)}
      </pre>
    </div>
  );
}

const meta: Meta<typeof FormatConfigWrapper> = {
  title: "Components/FormatConfigEditor",
  component: FormatConfigWrapper,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
};

export default meta;
type Story = StoryObj<typeof FormatConfigWrapper>;

const jsonSchema = findFilter("json");
const htmlSchema = findFilter("html");
const xliffSchema = findFilter("xliff");
const xmlSchema = findFilter("xml-stream") || findFilter("xml");
const propertiesSchema = findFilter("properties") || findFilter("regex");
const poSchema = findFilter("po") || findFilter("gettext");

export const JSONFilter: Story = {
  args: {
    schema: jsonSchema || { title: "JSON Filter", type: "object", properties: {} },
    title: "JSON Format Configuration",
  },
};

export const HTMLFilter: Story = {
  args: {
    schema: htmlSchema || { title: "HTML Filter", type: "object", properties: {} },
    title: "HTML Format Configuration",
  },
};

export const XLIFFFilter: Story = {
  args: {
    schema: xliffSchema || { title: "XLIFF Filter", type: "object", properties: {} },
    title: "XLIFF Format Configuration",
  },
};

export const XMLFilter: Story = {
  args: {
    schema: xmlSchema || { title: "XML Filter", type: "object", properties: {} },
    title: "XML Format Configuration",
  },
};

export const PropertiesFilter: Story = {
  args: {
    schema: propertiesSchema || { title: "Properties Filter", type: "object", properties: {} },
    title: "Properties Format Configuration",
  },
};

export const POFilter: Story = {
  args: {
    schema: poSchema || { title: "PO (Gettext) Filter", type: "object", properties: {} },
    title: "PO Format Configuration",
  },
};

// Story showing all available filters
export const FilterCatalog: Story = {
  render: () => {
    const filters = filterSchemas.slice(0, 20);
    return (
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16, maxWidth: 900 }}>
        {filters.map((schema, i) => (
          <div
            key={i}
            className="rounded-lg border border-border p-4"
          >
            <FormatConfigWrapper schema={schema} />
          </div>
        ))}
      </div>
    );
  },
};

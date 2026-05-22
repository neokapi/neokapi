import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Card } from "@neokapi/ui-primitives";
import { FormatConfigEditor } from "../components/FormatConfigEditor";
import type { ComponentSchema } from "../types/api";
import { okapiMetadata } from "./_lib/reference-data";

// Extract a few representative filter schemas for stories
const filterSchemas = okapiMetadata.filters as unknown as ComponentSchema[];

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
      <FormatConfigEditor schema={schema} values={values} onChange={setValues} title={title} />
      <pre className="mt-4 rounded bg-muted p-3 text-xs text-muted-foreground overflow-auto max-h-40">
        {JSON.stringify(values, null, 2)}
      </pre>
    </div>
  );
}

const meta: Meta<typeof FormatConfigWrapper> = {
  title: "Formats & Tools/Formats/Format Config Editor",
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
const csvSchema = findFilter("csv") || findFilter("table");

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

export const HTMLFilterWithValues: Story = {
  name: "HTML Filter (with values)",
  args: {
    schema: htmlSchema || { title: "HTML Filter", type: "object", properties: {} },
    title: "HTML Format Configuration",
    initialValues: {
      parser: { assumeWellformed: true },
      elements: {
        meta: { ruleTypes: ["ATTRIBUTES_ONLY"] },
        script: { ruleTypes: ["EXCLUDE"] },
      },
      attributes: {
        title: { ruleTypes: ["ATTRIBUTE_TRANS"] },
        alt: { ruleTypes: ["ATTRIBUTE_TRANS"] },
      },
    },
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

export const CSVFilter: Story = {
  name: "CSV/Table Filter (arrays)",
  args: {
    schema: csvSchema || { title: "CSV Filter", type: "object", properties: {} },
    title: "CSV Format Configuration",
  },
};

// Story showing ALL available filters as a scrollable catalog
export const FilterCatalog: Story = {
  name: "All Filters (Catalog)",
  render: () => {
    return (
      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16, maxWidth: 1000 }}>
        {filterSchemas.map((schema, i) => (
          <Card key={i} className="p-4">
            <FormatConfigWrapper schema={schema} />
          </Card>
        ))}
      </div>
    );
  },
};

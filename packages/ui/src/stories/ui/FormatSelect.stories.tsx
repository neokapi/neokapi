import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { FormatSelect, type FormatInfo } from "../../components/ui/format-select";

const sampleFormats: FormatInfo[] = [
  { name: "json", display_name: "JSON", extensions: [".json"] },
  { name: "xliff", display_name: "XLIFF 1.2", extensions: [".xlf", ".xliff"] },
  { name: "xliff2", display_name: "XLIFF 2.0", extensions: [".xlf"] },
  { name: "po", display_name: "Gettext PO", extensions: [".po", ".pot"] },
  { name: "properties", display_name: "Java Properties", extensions: [".properties"] },
  { name: "markdown", display_name: "Markdown", extensions: [".md"] },
  { name: "html", display_name: "HTML", extensions: [".html", ".htm"] },
  { name: "xml", display_name: "XML", extensions: [".xml"] },
  { name: "csv", display_name: "CSV", extensions: [".csv"] },
  { name: "yaml", display_name: "YAML", extensions: [".yaml", ".yml"] },
  { name: "resx", display_name: "ResX (.NET)", extensions: [".resx"] },
  { name: "strings", display_name: "Apple Strings", extensions: [".strings"] },
  { name: "arb", display_name: "Flutter ARB", extensions: [".arb"] },
  { name: "android", display_name: "Android XML", extensions: [".xml"] },
  {
    name: "okf_html",
    display_name: "HTML (Okapi)",
    extensions: [".html", ".htm"],
    source: "okapi",
  },
  { name: "okf_xml", display_name: "XML (Okapi)", extensions: [".xml"], source: "okapi" },
  { name: "okf_xliff", display_name: "XLIFF (Okapi)", extensions: [".xlf"], source: "okapi" },
  { name: "okf_po", display_name: "PO (Okapi)", extensions: [".po"], source: "okapi" },
  {
    name: "okf_properties",
    display_name: "Properties (Okapi)",
    extensions: [".properties"],
    source: "okapi",
  },
  { name: "okf_dtd", display_name: "DTD (Okapi)", extensions: [".dtd"], source: "okapi" },
  { name: "okf_ts", display_name: "Qt TS (Okapi)", extensions: [".ts"], source: "okapi" },
];

function Wrapper({
  initial = "",
  formats = sampleFormats,
}: {
  initial?: string;
  formats?: FormatInfo[];
}) {
  const [value, setValue] = useState(initial);
  return (
    <div className="max-w-sm space-y-2">
      <FormatSelect value={value} onChange={(v) => setValue(v ?? "")} formats={formats} />
      <pre className="rounded bg-muted p-2 font-mono text-xs">
        value: {JSON.stringify(value || null)}
      </pre>
    </div>
  );
}

const meta: Meta<typeof FormatSelect> = {
  title: "Foundations/FormatSelect",
  component: FormatSelect,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Searchable format selector with built-in/plugin grouping, file extensions, and clear-to-auto-detect. Built on shadcn Combobox.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof FormatSelect>;

export const Default: Story = {
  render: () => <Wrapper />,
};

export const WithValue: Story = {
  name: "With Pre-selected Format",
  render: () => <Wrapper initial="xliff" />,
};

export const BuiltInOnly: Story = {
  name: "Built-in Formats Only",
  render: () => (
    <Wrapper formats={sampleFormats.filter((f) => !f.source || f.source === "built-in")} />
  ),
};

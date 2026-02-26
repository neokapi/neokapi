import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react";
import { FilterConfigEditor } from "../../components/filter";
import type { FilterSchema, FilterParamsValue } from "../../components/filter";

const htmlFilterSchema: FilterSchema = {
  $id: "okf_html",
  $version: "1.0",
  title: "HTML Filter",
  description: "Configures how HTML files are parsed for translatable content.",
  type: "object",
  "x-filter": {
    id: "okf_html",
    class: "net.sf.okapi.filters.html.HtmlFilter",
    extensions: [".html", ".htm"],
    mimeTypes: ["text/html"],
  },
  "x-groups": [
    {
      id: "extraction",
      label: "Extraction",
      description: "Control which elements are extracted for translation.",
      fields: ["extractMetaTitle", "extractComments", "extractAltText"],
    },
    {
      id: "advanced",
      label: "Advanced",
      description: "Advanced parsing options.",
      collapsed: true,
      fields: ["preserveWhitespace", "maxSegmentLength"],
    },
  ],
  properties: {
    extractMetaTitle: {
      type: "boolean",
      description: "Extract the <title> meta tag for translation.",
      default: true,
    },
    extractComments: {
      type: "boolean",
      description: "Extract HTML comments as translatable content.",
      default: false,
    },
    extractAltText: {
      type: "boolean",
      description: "Extract alt attributes from image tags.",
      default: true,
    },
    preserveWhitespace: {
      type: "boolean",
      description: "Preserve original whitespace in extracted text.",
      default: false,
    },
    maxSegmentLength: {
      type: "integer",
      description: "Maximum segment length (0 = no limit).",
      default: 0,
      "x-placeholder": "0",
    },
  },
};

const meta: Meta<typeof FilterConfigEditor> = {
  title: "Pages/FilterConfigEditor",
  component: FilterConfigEditor,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 520, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof FilterConfigEditor>;

export const HTMLFilter: Story = {
  render: () => {
    const [value, setValue] = useState<FilterParamsValue>({
      extractMetaTitle: true,
      extractComments: false,
      extractAltText: true,
      preserveWhitespace: false,
      maxSegmentLength: 0,
    });
    return (
      <FilterConfigEditor
        schema={htmlFilterSchema}
        value={value}
        onChange={setValue}
      />
    );
  },
};

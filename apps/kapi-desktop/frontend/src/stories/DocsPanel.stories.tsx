import type { Meta, StoryObj } from "@storybook/react-vite";
import { DocsPanel, ParamHelp } from "../components/DocsPanel";
import { pluginDocs } from "./_lib/reference-data";
import type { FilterDoc, StepDoc, PluginDocs } from "../types/api";

const docs = pluginDocs as unknown as PluginDocs;

const meta: Meta<typeof DocsPanel> = {
  title: "Components/DocsPanel",
  component: DocsPanel,
  tags: ["autodocs"],
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 640 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof DocsPanel>;

// --- Filter documentation stories ---

export const JSONFilter: Story = {
  name: "JSON Filter",
  args: {
    doc: docs.filters.okf_json as FilterDoc,
  },
};

export const HTMLFilter: Story = {
  name: "HTML Filter",
  args: {
    doc: docs.filters.okf_html as FilterDoc,
  },
};

export const XLIFFFilter: Story = {
  name: "XLIFF Filter",
  args: {
    doc: docs.filters.okf_xliff as FilterDoc,
  },
};

export const PropertiesFilter: Story = {
  name: "Properties Filter",
  args: {
    doc: docs.filters.okf_properties as FilterDoc,
  },
};

export const POFilter: Story = {
  name: "PO (Gettext) Filter",
  args: {
    doc: docs.filters.okf_po as FilterDoc,
  },
};

// --- Step documentation stories ---

const firstStep = Object.values(docs.steps)[0] as StepDoc;
const secondStep = Object.values(docs.steps)[1] as StepDoc;

export const PipelineStep: Story = {
  name: "Pipeline Step",
  args: {
    doc: firstStep,
  },
};

export const SecondStep: Story = {
  name: "Second Step",
  args: {
    doc: secondStep,
  },
};

// --- Inline mode ---

export const InlineMode: Story = {
  name: "Inline (no card wrapper)",
  args: {
    doc: docs.filters.okf_json as FilterDoc,
    inline: true,
  },
};

// --- Filtered parameters ---

export const FilteredParams: Story = {
  name: "Filtered Parameters (extraction only)",
  args: {
    doc: docs.filters.okf_json as FilterDoc,
    visibleParams: ["extraction"],
  },
};

// --- ParamHelp inline component ---

export const ParameterHelp: StoryObj = {
  name: "ParamHelp (inline tooltip)",
  render: () => (
    <div className="space-y-3 p-4">
      <p className="text-sm text-foreground">Click the info icons to see parameter help:</p>
      <div className="flex items-center gap-2 text-sm">
        <span className="text-foreground">inlineCodes</span>
        <ParamHelp paramKey="inlineCodes" doc={docs.filters.okf_json as FilterDoc} />
      </div>
      <div className="flex items-center gap-2 text-sm">
        <span className="text-foreground">extraction.extractAll</span>
        <ParamHelp paramKey="extraction.extractAll" doc={docs.filters.okf_json as FilterDoc} />
      </div>
      <div className="flex items-center gap-2 text-sm">
        <span className="text-foreground">nonexistent (no tooltip)</span>
        <ParamHelp paramKey="nonexistent" doc={docs.filters.okf_json as FilterDoc} />
      </div>
    </div>
  ),
};

// --- All filters gallery ---

export const FilterGallery: StoryObj = {
  name: "All Filters Gallery",
  render: () => (
    <div className="space-y-6">
      {Object.entries(docs.filters).map(([id, filterDoc]) => (
        <div key={id}>
          <h3 className="text-sm font-semibold text-foreground mb-2">{id}</h3>
          <DocsPanel doc={filterDoc as FilterDoc} />
        </div>
      ))}
    </div>
  ),
};

// --- All steps gallery ---

export const StepGallery: StoryObj = {
  name: "All Steps Gallery",
  render: () => (
    <div className="space-y-6">
      {Object.entries(docs.steps).map(([id, stepDoc]) => (
        <div key={id}>
          <h3 className="text-sm font-semibold text-foreground mb-2">{id}</h3>
          <DocsPanel doc={stepDoc as StepDoc} />
        </div>
      ))}
    </div>
  ),
};

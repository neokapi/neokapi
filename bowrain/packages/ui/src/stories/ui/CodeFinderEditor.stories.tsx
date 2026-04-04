import { CodeFinderEditor, type CodeFinderRulesValue } from "@neokapi/ui-primitives";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";

function Wrapper(props: {
  initial?: CodeFinderRulesValue;
  presets?: Record<string, unknown>;
  label?: string;
  description?: string;
  disabled?: boolean;
}) {
  const [value, setValue] = useState<CodeFinderRulesValue>(
    props.initial ?? { rules: [], sample: "" },
  );
  return (
    <div className="max-w-lg">
      <CodeFinderEditor
        value={value}
        onChange={setValue}
        presets={props.presets}
        label={props.label}
        description={props.description}
        disabled={props.disabled}
      />
      <pre className="mt-4 p-2 rounded bg-muted text-xs font-mono overflow-auto">
        {JSON.stringify(value, null, 2)}
      </pre>
    </div>
  );
}

const meta: Meta<typeof CodeFinderEditor> = {
  title: "UI/CodeFinderEditor",
  component: CodeFinderEditor,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Editor for inline code detection rules. Supports numbered regex patterns, preset selection, validation feedback, and live match highlighting against sample text.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof CodeFinderEditor>;

export const Empty: Story = {
  render: () => (
    <Wrapper
      label="Inline Code Rules"
      description="Define regex patterns to detect inline codes in translatable text."
    />
  ),
};

export const WithRules: Story = {
  render: () => (
    <Wrapper
      label="Inline Code Rules"
      initial={{
        rules: [{ pattern: "</?\\w[^>]*>" }, { pattern: "\\{\\d+\\}" }, { pattern: "%[ds]" }],
        sample: "Click <b>OK</b> to format {0} with %d items",
      }}
    />
  ),
};

export const WithPresets: Story = {
  render: () => (
    <Wrapper
      label="Inline Code Rules"
      description="Choose a preset or define custom patterns."
      presets={{
        "HTML Tags": {
          rules: [{ pattern: "</?\\w[^>]*>" }],
          sample: "<b>Bold</b> and <i>italic</i>",
        },
        "Printf Codes": {
          rules: [
            { pattern: "%[diouxXeEfgGaAcspn%]" },
            { pattern: "%[-+ #0]*\\d*\\.?\\d*[diouxXeEfgGaAcspn%]" },
          ],
          sample: "Found %d items in %s",
        },
        "ICU Placeholders": {
          rules: [{ pattern: "\\{[^}]+\\}" }],
          sample: "Hello {name}, you have {count} messages",
        },
      }}
    />
  ),
};

export const InvalidRegex: Story = {
  render: () => (
    <Wrapper
      label="Inline Code Rules"
      initial={{
        rules: [{ pattern: "</?\\w[^>]*>" }, { pattern: "[invalid(" }, { pattern: "\\{\\d+\\}" }],
        sample: "Test <b>text</b> with {0}",
      }}
    />
  ),
};

export const Disabled: Story = {
  render: () => (
    <Wrapper
      label="Inline Code Rules"
      disabled
      initial={{
        rules: [{ pattern: "</?\\w[^>]*>" }, { pattern: "\\{\\d+\\}" }],
        sample: "Click <b>OK</b> to format {0}",
      }}
    />
  ),
};

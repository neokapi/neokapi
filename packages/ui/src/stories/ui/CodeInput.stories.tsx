import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { CodeInput, type CodeLanguage } from "../../components/ui/code-input";

function Wrapper({
  initial = "",
  language = "plain",
  placeholder,
  singleLine,
  disabled,
  minHeight,
}: {
  initial?: string;
  language?: CodeLanguage;
  placeholder?: string;
  singleLine?: boolean;
  disabled?: boolean;
  minHeight?: number;
}) {
  const [value, setValue] = useState(initial);
  return (
    <div className="max-w-lg space-y-2">
      <CodeInput
        value={value}
        onChange={setValue}
        language={language}
        placeholder={placeholder}
        singleLine={singleLine}
        disabled={disabled}
        minHeight={minHeight}
      />
      <pre className="p-2 rounded bg-muted text-xs font-mono overflow-auto max-h-[100px]">
        {JSON.stringify(value)}
      </pre>
    </div>
  );
}

const meta: Meta<typeof CodeInput> = {
  title: "Foundations/CodeInput",
  component: CodeInput,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "CodeMirror 6 based code editor with syntax highlighting. Supports JavaScript, JSON, regex, and plain text modes. Used by SchemaForm code-editor widget and CodeFinderEditor regex inputs.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof CodeInput>;

export const JavaScript: Story = {
  render: () => (
    <Wrapper
      language="javascript"
      placeholder="// Enter JavaScript code..."
      initial={`function transform(segment) {\n  const text = segment.target || segment.source;\n  return text.toUpperCase();\n}`}
      minHeight={160}
    />
  ),
};

export const JsonMode: Story = {
  name: "JSON",
  render: () => (
    <Wrapper
      language="json"
      placeholder="{}"
      initial={`{\n  "extractAll": true,\n  "pathRules": "$.messages[*].text",\n  "useCodeFinder": false\n}`}
      minHeight={140}
    />
  ),
};

export const Regex: Story = {
  render: () => (
    <Wrapper language="regex" placeholder="Regex pattern..." initial="</?\\w[^>]*>" singleLine />
  ),
};

export const RegexMultiplePatterns: Story = {
  name: "Regex — Multiple Patterns",
  render: () => {
    const patterns = [
      "</?\\w[^>]*>",
      "\\{\\d+\\}",
      "%[-+0 #]*\\d*\\.?\\d*[diouxXeEfgGaAcspn%]",
      "\\$\\{[^}]+\\}",
    ];
    return (
      <div className="max-w-lg space-y-1.5">
        {patterns.map((p, i) => (
          <div key={i} className="flex items-center gap-2">
            <span className="text-xs text-muted-foreground w-4 text-right">{i + 1}</span>
            <Wrapper initial={p} language="regex" singleLine />
          </div>
        ))}
      </div>
    );
  },
};

export const PlainText: Story = {
  render: () => (
    <Wrapper
      language="plain"
      placeholder="Enter text..."
      initial="One rule per line\nAnother rule here"
      minHeight={100}
    />
  ),
};

export const SingleLine: Story = {
  render: () => (
    <Wrapper
      language="javascript"
      placeholder="Single line expression..."
      initial="segment.source.replace(/\\s+/g, ' ')"
      singleLine
    />
  ),
};

export const Disabled: Story = {
  render: () => <Wrapper language="javascript" initial="const x = 42;" disabled singleLine />,
};

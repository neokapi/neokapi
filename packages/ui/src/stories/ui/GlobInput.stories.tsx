import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { GlobInput } from "../../components/ui/glob-input";
import { TargetPathInput } from "../../components/ui/target-path-input";

function GlobWrapper({ initial = "", placeholder }: { initial?: string; placeholder?: string }) {
  const [value, setValue] = useState(initial);
  return (
    <div className="max-w-lg space-y-2">
      <GlobInput value={value} onChange={setValue} placeholder={placeholder} />
      <pre className="max-h-[60px] overflow-auto rounded bg-muted p-2 font-mono text-xs">
        {JSON.stringify(value)}
      </pre>
    </div>
  );
}

function TargetWrapper({ initial = "", placeholder }: { initial?: string; placeholder?: string }) {
  const [value, setValue] = useState(initial);
  return (
    <div className="max-w-lg space-y-2">
      <TargetPathInput value={value} onChange={setValue} placeholder={placeholder} />
      <pre className="max-h-[60px] overflow-auto rounded bg-muted p-2 font-mono text-xs">
        {JSON.stringify(value)}
      </pre>
    </div>
  );
}

const meta: Meta<typeof GlobInput> = {
  title: "Foundations/GlobInput",
  component: GlobInput,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Single-line input with glob pattern syntax highlighting. Highlights **, *, ?, {braces}, [classes], and path separators.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof GlobInput>;

export const BasicGlob: Story = {
  name: "Glob — Basic",
  render: () => <GlobWrapper initial="src/locales/en/*.json" placeholder="src/**/*.json" />,
};

export const Globstar: Story = {
  name: "Glob — Globstar",
  render: () => <GlobWrapper initial="docs/**/*.md" />,
};

export const BraceExpansion: Story = {
  name: "Glob — Brace Expansion",
  render: () => <GlobWrapper initial="src/**/*.{ts,tsx,js,jsx}" />,
};

export const CharacterClass: Story = {
  name: "Glob — Character Class",
  render: () => <GlobWrapper initial="data/[0-9]*-report.csv" />,
};

export const ComplexPattern: Story = {
  name: "Glob — Complex",
  render: () => <GlobWrapper initial="src/**/i18n/{en,fr}/**/*.{json,yaml}" />,
};

export const TargetPathBasic: Story = {
  name: "Target Path — Basic",
  render: () => (
    <TargetWrapper initial="src/locales/{lang}/*.json" placeholder="output/{lang}/**/*" />
  ),
};

export const TargetPathMultipleVars: Story = {
  name: "Target Path — Multiple Variables",
  render: () => <TargetWrapper initial="output/{lang}/{region}/**/*.json" />,
};

export const TargetPathWithWildcards: Story = {
  name: "Target Path — With Wildcards",
  render: () => <TargetWrapper initial="dist/{lang}/**/*" />,
};

export const SideBySide: Story = {
  name: "Glob + Target Path — Side by Side",
  render: () => (
    <div className="max-w-2xl space-y-3">
      <div>
        <label className="mb-1 block text-xs text-muted-foreground">Path pattern (glob)</label>
        <GlobWrapper initial="src/i18n/en/**/*.json" />
      </div>
      <div>
        <label className="mb-1 block text-xs text-muted-foreground">Target path</label>
        <TargetWrapper initial="src/i18n/{lang}/**/*.json" />
      </div>
    </div>
  ),
};

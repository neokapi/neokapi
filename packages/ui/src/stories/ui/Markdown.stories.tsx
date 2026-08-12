import type { Meta, StoryObj } from "@storybook/react-vite";
import { Markdown } from "../../components/ui/markdown";

const meta: Meta<typeof Markdown> = {
  title: "Foundations/Markdown",
  component: Markdown,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "The single typeset prose primitive for neokapi UIs. Every markdown-bearing " +
          "metadata field (tool/format/flow/plugin descriptions, overviews, parameter " +
          "help, long-form docs) renders through this component rather than being dropped " +
          "into JSX as a raw string. Wraps react-markdown + remark-gfm; use `inline` for " +
          "compact list rows, table cells, tooltips, and chips. " +
          "See web/docs/contribute/implementation/repo/markdown-in-ui.md.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof Markdown>;

const SAMPLE = `Translate blocks with an **AI provider**. Supports \`--target-lang\`, brand
voice, and glossary enforcement.

- Streams results block by block
- Falls back to source on provider error
- See the [providers guide](https://neokapi.org) for setup

| Provider | Streaming |
| --- | --- |
| Anthropic | yes |
| OpenAI | yes |
`;

export const Block: Story = {
  name: "Block prose (detail view)",
  render: () => (
    <div className="max-w-lg rounded-md border border-border p-4">
      <Markdown>{SAMPLE}</Markdown>
    </div>
  ),
};

export const Inline: Story = {
  name: "Inline (clamped list row)",
  render: () => (
    <div className="max-w-xs rounded-md border border-border p-3">
      <div className="line-clamp-2 text-[11px] text-muted-foreground">
        <Markdown inline>
          Translate blocks with an **AI provider** — supports `--target-lang` and a [brand
          voice](https://neokapi.org).
        </Markdown>
      </div>
    </div>
  ),
};

export const RawStringComparison: Story = {
  name: "Why: raw string vs Markdown",
  render: () => {
    const src = "Rewrites the source with **redaction** — see `redact.rules`.";
    return (
      <div className="max-w-lg space-y-3 text-sm">
        <div className="rounded-md border border-destructive/30 p-3">
          <div className="mb-1 text-xs font-semibold text-destructive">✗ Raw string in JSX</div>
          <p className="text-muted-foreground">{src}</p>
        </div>
        <div className="rounded-md border border-border p-3">
          <div className="mb-1 text-xs font-semibold text-muted-foreground">✓ &lt;Markdown&gt;</div>
          <Markdown className="text-muted-foreground">{src}</Markdown>
        </div>
      </div>
    );
  },
};

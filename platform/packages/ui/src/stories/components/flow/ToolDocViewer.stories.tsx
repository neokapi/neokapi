import type { Meta, StoryObj } from "@storybook/react-vite";
import { ToolDocViewer } from "../../../components/flow/ToolDocViewer";

const meta: Meta<typeof ToolDocViewer> = {
  title: "Workspace/Flow/ToolDocViewer",
  component: ToolDocViewer,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 640, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof ToolDocViewer>;

const sampleDoc = `# Quality Check

Configurable quality assurance checks for bilingual content: whitespace, inline codes, patterns, length, characters, terminology, and LanguageTool grammar verification.

## Parameters

### Whitespace

#### Check Leading Whitespace
Flag text units where the leading whitespace characters differ between source and target.

#### Check Trailing Whitespace
Flag text units where the trailing whitespace characters differ between source and target.

### Length

#### Check Maximum Character Length
Flag target text longer than a given percentage of source text character length.

- **Long text threshold**: Character count above which text is considered "long"
- **Percentage for long text**: Maximum allowed percentage for long text
- **Percentage for short text**: Maximum allowed percentage for short text

### Patterns

#### Source/Target Pattern Rules
Regex pattern pairs for verifying source-target consistency. Each pattern defines:
- A **source regex** to match in the source text
- A **target regex** (or \`<same>\` to reuse the source match)
- **Severity** level (Low, Medium, High)
- **Direction**: source→target or target→source

\`\`\`yaml
patterns:
  - source: "[\\\\(\\\\uFF08]"
    target: "[\\\\(\\\\uFF08]"
    severity: low
    description: "Opening parenthesis"
\`\`\`

## Limitations

- LanguageTool integration requires a running server instance
- Pattern checking may increase processing time with many patterns

## Notes

- The quality check step uses the same configuration as the CheckMate application
- Session files (.qcs) can be used to persist check results across runs
`;

export const Default: Story = {
  args: {
    content: sampleDoc,
    title: "Quality Check",
    wikiUrl: "https://okapiframework.org/wiki/index.php/Quality_Check_Step",
  },
};

export const WithoutTitle: Story = {
  args: {
    content: sampleDoc,
  },
};

export const ShortDoc: Story = {
  args: {
    content: `# Segmentation

Apply SRX segmentation rules to split text units into sentences.

## Parameters

#### Segment Source Text
Segment the source text of text units using SRX rules.

#### Segment Target Text
Segment existing target text using SRX rules for the processed locales.
`,
    title: "Segmentation",
    wikiUrl: "https://okapiframework.org/wiki/index.php/Segmentation_Step",
  },
};

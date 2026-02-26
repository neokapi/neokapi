/**
 * TranslationEditor with the split-view document preview.
 *
 * This demonstrates the renderPreview prop that the desktop app (Bowrain)
 * uses to embed a live document preview alongside the translation grid.
 */
import type { Meta, StoryObj } from "@storybook/react";
import { fn } from "@storybook/test";
import { TranslationEditor } from "../../components/TranslationEditor";
import type { BlockInfo } from "../../types/api";
import { sampleProject } from "../fixtures";
import { withProviders } from "../decorators";

/**
 * A lightweight mock preview component for Storybook.
 * In the real desktop app this is the DocumentPreview component which
 * renders the translated document inside an iframe.
 */
function MockDocumentPreview({
  itemName,
  targetLocale,
  selectedBlockId,
  onBlockSelect,
  blocks,
}: {
  projectId: string;
  itemName: string;
  targetLocale: string;
  selectedBlockId?: string;
  onBlockSelect: (blockId: string) => void;
  blocks: BlockInfo[];
}) {
  return (
    <div
      style={{
        width: "100%",
        height: "100%",
        display: "flex",
        flexDirection: "column",
        border: "1px solid hsl(var(--border))",
        borderRadius: 8,
        overflow: "auto",
      }}
      className="bg-card"
    >
      <div
        style={{
          padding: "8px 12px",
          fontSize: 11,
          fontWeight: 600,
          textTransform: "uppercase",
          letterSpacing: 0.5,
          borderBottom: "1px solid hsl(var(--border))",
        }}
        className="text-muted-foreground"
      >
        Preview: {itemName} ({targetLocale})
      </div>
      <div style={{ padding: 16, flex: 1 }}>
        {blocks.map((block) => (
          <div
            key={block.id}
            onClick={() => onBlockSelect(block.id)}
            style={{
              padding: "6px 10px",
              marginBottom: 4,
              borderRadius: 4,
              cursor: "pointer",
              border: block.id === selectedBlockId
                ? "2px solid hsl(var(--primary))"
                : "1px solid transparent",
              transition: "border-color 0.15s",
            }}
            className={block.id === selectedBlockId ? "bg-accent" : "hover:bg-accent/50"}
          >
            <span style={{ fontSize: 13 }}>
              {block.targets[targetLocale] || block.source}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

const meta: Meta<typeof TranslationEditor> = {
  title: "Editor/TranslationEditor (Split Preview)",
  component: TranslationEditor,
  tags: ["autodocs"],
  decorators: [
    withProviders,
    (Story) => (
      <div style={{ width: "100vw", height: "100vh", overflow: "auto" }}>
        <Story />
      </div>
    ),
  ],
  parameters: {
    layout: "fullscreen",
  },
};

export default meta;
type Story = StoryObj<typeof TranslationEditor>;

export const WithPreview: Story = {
  args: {
    project: sampleProject,
    fileName: "messages.json",
    onBack: fn(),
    renderPreview: (props) => <MockDocumentPreview {...props} />,
  },
};

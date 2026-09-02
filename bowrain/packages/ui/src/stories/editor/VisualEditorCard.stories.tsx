import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { VisualEditorCard } from "../../components/editor/VisualEditorCard";
import type { VisualEditorMode } from "../../components/editor/visual-editor-types";
import {
  sampleBlocks,
  sampleProject,
  sampleMemoryMatches,
  sampleTermMatches,
  sampleQAIssues,
  sampleBlockNotes,
  sampleBlockHistory,
} from "../fixtures";

const baseBlock = sampleBlocks[1]; // "Click here to continue" — has spans

// ---------------------------------------------------------------------------
// Interactive wrapper — manages isEditing + editorMode state
// ---------------------------------------------------------------------------

type CardOverrides = Partial<React.ComponentProps<typeof VisualEditorCard>>;

function InteractiveCard(props: CardOverrides) {
  const [isEditing, setIsEditing] = useState(false);
  const [editorMode, setEditorMode] = useState<VisualEditorMode>(
    (props.editorMode as VisualEditorMode) ?? "translate",
  );

  const block = props.block ?? baseBlock;

  return (
    <VisualEditorCard
      block={block}
      blockIndex={props.blockIndex ?? 1}
      totalBlocks={props.totalBlocks ?? sampleBlocks.length}
      targetLocale={props.targetLocale ?? "fr-FR"}
      editorMode={editorMode}
      onEditorModeChange={setEditorMode}
      isEditing={isEditing}
      onStartEditing={() => setIsEditing(true)}
      onSave={() => setIsEditing(false)}
      onCancel={() => setIsEditing(false)}
      onApprove={() => setIsEditing(false)}
      onReject={() => {}}
      memoryMatches={props.memoryMatches ?? []}
      termMatches={props.termMatches ?? []}
      onApplyMemory={props.onApplyMemory ?? (() => {})}
      onInsertTerm={props.onInsertTerm ?? (() => {})}
      project={props.project ?? sampleProject}
      referenceLocales={props.referenceLocales}
      qaIssues={props.qaIssues}
      history={props.history}
      onRevertHistory={props.onRevertHistory}
      notes={props.notes}
      onAddNote={props.onAddNote}
      onDeleteNote={props.onDeleteNote}
      onTermCreate={props.onTermCreate}
    />
  );
}

// ---------------------------------------------------------------------------
// Meta
// ---------------------------------------------------------------------------

const meta: Meta<typeof VisualEditorCard> = {
  title: "Editor/Visual/VisualEditorCard",
  component: VisualEditorCard,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 700, padding: 24 }}>
        <Story />
      </div>
    ),
  ],
  args: {
    block: baseBlock,
    blockIndex: 1,
    totalBlocks: sampleBlocks.length,
    targetLocale: "fr-FR",
    editorMode: "translate",
    onEditorModeChange: fn(),
    isEditing: false,
    onStartEditing: fn(),
    onSave: fn(),
    onCancel: fn(),
    onApprove: fn(),
    onReject: fn(),
    memoryMatches: [],
    termMatches: [],
    onApplyMemory: fn(),
    onInsertTerm: fn(),
    project: sampleProject,
  },
};

export default meta;
type Story = StoryObj<typeof VisualEditorCard>;

// ---------------------------------------------------------------------------
// Interactive stories — click target to edit, type, Enter/save, Escape/cancel
// ---------------------------------------------------------------------------

/**
 * Interactive card — click the target area to start editing, type text,
 * press Enter or click Save to confirm, Escape to cancel.
 * Use the mode switcher to toggle between translate / review / enrich.
 */
export const Interactive: Story = {
  render: () => <InteractiveCard />,
};

/** Interactive card with content-memory matches panel visible */
export const WithMemoryMatches: Story = {
  render: () => <InteractiveCard memoryMatches={sampleMemoryMatches} />,
};

/** Interactive card in enrich mode with notes */
export const EnrichMode: Story = {
  render: () => (
    <InteractiveCard
      editorMode="enrich"
      notes={sampleBlockNotes}
      onAddNote={fn()}
      onDeleteNote={fn()}
      onTermCreate={fn()}
    />
  ),
};

/** Interactive card in review mode — approve/reject are functional */
export const ReviewMode: Story = {
  render: () => <InteractiveCard editorMode="review" block={sampleBlocks[0]} blockIndex={0} />,
};

/**
 * Interactive card with all panels: content memory, findings, history, terms, ref locales.
 * Full editing flow is functional.
 */
export const FullFeatured: Story = {
  render: () => (
    <InteractiveCard
      block={sampleBlocks[0]}
      blockIndex={0}
      memoryMatches={sampleMemoryMatches}
      termMatches={sampleTermMatches}
      qaIssues={sampleQAIssues}
      history={sampleBlockHistory}
      onRevertHistory={fn()}
      referenceLocales={["de-DE"]}
      notes={sampleBlockNotes}
      onAddNote={fn()}
      onDeleteNote={fn()}
      onTermCreate={fn()}
    />
  ),
};

// ---------------------------------------------------------------------------
// Static snapshots — design review oriented, not interactive
// ---------------------------------------------------------------------------

/** Static snapshot: translate mode with editing active */
export const TranslateModeEditing: Story = {
  args: {
    isEditing: true,
  },
};

/** Static snapshot: finding badge display */
export const WithQAIssues: Story = {
  args: {
    qaIssues: sampleQAIssues,
  },
};

/** Static snapshot: block history entries */
export const WithHistory: Story = {
  args: {
    history: sampleBlockHistory,
    onRevertHistory: fn(),
  },
};

/** Static snapshot: reference locales display */
export const WithReferenceLocales: Story = {
  args: {
    referenceLocales: ["de-DE"],
    block: sampleBlocks[0], // has de-DE target
    blockIndex: 0,
  },
};

/** Static snapshot: not-started block (no target text) */
export const NotStartedBlock: Story = {
  args: {
    block: sampleBlocks[2], // empty targets
    blockIndex: 2,
  },
};

/** Static snapshot: reviewed block status */
export const ReviewedBlock: Story = {
  args: {
    block: {
      ...sampleBlocks[0],
      properties: { "translation-status": "reviewed" },
    },
    blockIndex: 0,
  },
};

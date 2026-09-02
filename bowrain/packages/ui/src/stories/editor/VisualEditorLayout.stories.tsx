import { useState, useCallback } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { VisualEditorLayout } from "../../components/editor/VisualEditorLayout";
import {
  sampleBlocks,
  sampleProject,
  sampleMemoryMatches,
  sampleTermMatches,
  sampleQAIssues,
  sampleFileQAResults,
  sampleBlockNotes,
  sampleBlockHistory,
  navigationBlocks,
} from "../fixtures";
import { withProviders, createProvidersDecorator } from "../decorators";

// ---------------------------------------------------------------------------
// Interactive wrapper — manages selection, editing, and mode state
// ---------------------------------------------------------------------------

type LayoutOverrides = Partial<React.ComponentProps<typeof VisualEditorLayout>>;

function InteractiveLayout(overrides: LayoutOverrides) {
  const blocks = overrides.blocks ?? sampleBlocks;
  const [selectedIndex, setSelectedIndex] = useState(overrides.selectedIndex ?? 0);
  const [editingIndex, setEditingIndex] = useState<number | null>(null);

  const handleNavigate = useCallback((idx: number) => {
    setSelectedIndex(idx);
    setEditingIndex(null);
  }, []);

  const handleStartEditing = useCallback(() => {
    setEditingIndex(selectedIndex);
  }, [selectedIndex]);

  const handleSave = useCallback(() => {
    setEditingIndex(null);
    setSelectedIndex((i) => Math.min(i + 1, blocks.length - 1));
  }, [blocks.length]);

  const handleCancelEditing = useCallback(() => {
    setEditingIndex(null);
  }, []);

  return (
    <VisualEditorLayout
      project={overrides.project ?? sampleProject}
      fileName={overrides.fileName ?? "messages.json"}
      blocks={blocks}
      selectedIndex={selectedIndex}
      editingIndex={editingIndex}
      targetLocale={overrides.targetLocale ?? "fr-FR"}
      onNavigate={handleNavigate}
      onStartEditing={handleStartEditing}
      onSave={handleSave}
      onCancelEditing={handleCancelEditing}
      onApprove={() => setSelectedIndex((i) => Math.min(i + 1, blocks.length - 1))}
      onReject={() => {}}
      memoryMatches={overrides.memoryMatches ?? []}
      termMatches={overrides.termMatches ?? []}
      onApplyMemory={overrides.onApplyMemory ?? (() => {})}
      onInsertTerm={overrides.onInsertTerm ?? (() => {})}
      presenceSlot={overrides.presenceSlot}
      qaIssues={overrides.qaIssues}
      fileQAResults={overrides.fileQAResults}
      qaLoading={overrides.qaLoading}
      onRunFileQA={overrides.onRunFileQA}
      history={overrides.history}
      onRevertHistory={overrides.onRevertHistory}
      notes={overrides.notes}
      onAddNote={overrides.onAddNote}
      onDeleteNote={overrides.onDeleteNote}
      onTermCreate={overrides.onTermCreate}
    />
  );
}

// ---------------------------------------------------------------------------
// Meta
// ---------------------------------------------------------------------------

const meta: Meta<typeof VisualEditorLayout> = {
  title: "Editor/Visual/VisualEditorLayout",
  component: VisualEditorLayout,
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
  args: {
    project: sampleProject,
    fileName: "messages.json",
    blocks: sampleBlocks,
    selectedIndex: 0,
    editingIndex: null,
    targetLocale: "fr-FR",
    onNavigate: fn(),
    onStartEditing: fn(),
    onSave: fn(),
    onCancelEditing: fn(),
    onApprove: fn(),
    onReject: fn(),
    memoryMatches: [],
    termMatches: [],
    onApplyMemory: fn(),
    onInsertTerm: fn(),
  },
};

export default meta;
type Story = StoryObj<typeof VisualEditorLayout>;

// ---------------------------------------------------------------------------
// Interactive stories — click blocks, Enter to edit, navigate, switch modes
// ---------------------------------------------------------------------------

/**
 * Interactive layout — click blocks in the card list or preview to navigate,
 * Enter to start editing, Escape to cancel, switch modes via the toolbar.
 */
export const Default: Story = {
  render: () => <InteractiveLayout />,
};

/** Interactive layout with content-memory matches */
export const WithMemoryMatches: Story = {
  render: () => <InteractiveLayout memoryMatches={sampleMemoryMatches} />,
};

/** Interactive layout — reach review mode from the toolbar */
export const ReviewMode: Story = {
  render: () => <InteractiveLayout />,
};

/** Enrich-mode inputs — notes and term creation; reach the mode from the toolbar */
export const EnrichMode: Story = {
  render: () => (
    <InteractiveLayout
      notes={sampleBlockNotes}
      onAddNote={fn()}
      onDeleteNote={fn()}
      onTermCreate={fn()}
    />
  ),
};

/**
 * Interactive layout with all panels: content memory, terms, findings, history, notes,
 * presence slot. Full editing flow is functional.
 */
export const FullFeatured: Story = {
  render: () => (
    <InteractiveLayout
      selectedIndex={1}
      memoryMatches={sampleMemoryMatches}
      termMatches={sampleTermMatches}
      qaIssues={sampleQAIssues}
      fileQAResults={sampleFileQAResults}
      onRunFileQA={fn()}
      history={sampleBlockHistory}
      onRevertHistory={fn()}
      notes={sampleBlockNotes}
      onAddNote={fn()}
      onDeleteNote={fn()}
      onTermCreate={fn()}
      presenceSlot={
        <div style={{ display: "flex", gap: 4 }}>
          <div
            style={{
              width: 24,
              height: 24,
              borderRadius: "50%",
              background: "#6366f1",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              color: "#fff",
              fontSize: 11,
              fontWeight: 600,
            }}
          >
            JD
          </div>
          <div
            style={{
              width: 24,
              height: 24,
              borderRadius: "50%",
              background: "#f59e0b",
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              color: "#fff",
              fontSize: 11,
              fontWeight: 600,
            }}
          >
            MK
          </div>
        </div>
      }
    />
  ),
};

// ---------------------------------------------------------------------------
// Static snapshots — design review oriented, not interactive
// ---------------------------------------------------------------------------

/** Static snapshot: second block selected */
export const SecondBlockSelected: Story = {
  args: {
    selectedIndex: 1,
  },
};

/** Static snapshot: term sidebar visible */
export const WithTermSidebar: Story = {
  args: {
    termMatches: sampleTermMatches,
  },
};

/** Static snapshot: block findings and file findings */
export const WithQAIssues: Story = {
  args: {
    qaIssues: sampleQAIssues,
    fileQAResults: sampleFileQAResults,
    onRunFileQA: fn(),
  },
};

/** Static snapshot: block history */
export const WithHistory: Story = {
  args: {
    history: sampleBlockHistory,
    onRevertHistory: fn(),
  },
};

/** Interactive preview modes — target and pseudo are toolbar choices */
export const PreviewModes: Story = {
  render: () => <InteractiveLayout />,
};

// ---------------------------------------------------------------------------
// Navigation — interactive story for testing keyboard navigation
// ---------------------------------------------------------------------------

function NavigationDemo() {
  const blocks = navigationBlocks;
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [editingIndex, setEditingIndex] = useState<number | null>(null);

  const handleNavigate = useCallback((idx: number) => {
    setSelectedIndex(idx);
    setEditingIndex(null);
  }, []);

  const handleStartEditing = useCallback(() => {
    setEditingIndex(selectedIndex);
  }, [selectedIndex]);

  const handleSave = useCallback(() => {
    setEditingIndex(null);
    setSelectedIndex((i) => Math.min(i + 1, blocks.length - 1));
  }, [blocks.length]);

  const handleCancelEditing = useCallback(() => {
    setEditingIndex(null);
  }, []);

  return (
    <VisualEditorLayout
      project={sampleProject}
      fileName="getting-started.md"
      blocks={blocks}
      selectedIndex={selectedIndex}
      editingIndex={editingIndex}
      targetLocale="fr-FR"
      onNavigate={handleNavigate}
      onStartEditing={handleStartEditing}
      onSave={handleSave}
      onCancelEditing={handleCancelEditing}
      onApprove={() => setSelectedIndex((i) => Math.min(i + 1, blocks.length - 1))}
      onReject={() => {}}
      memoryMatches={[]}
      termMatches={[]}
      onApplyMemory={() => {}}
      onInsertTerm={() => {}}
    />
  );
}

/**
 * Interactive navigation story — use keyboard shortcuts to move between blocks:
 * - **j / ArrowDown** — next block
 * - **k / ArrowUp** — previous block
 * - **Enter** — start editing
 * - **Escape** — cancel editing
 * - **n / N** — next / previous untranslated block
 *
 * Click blocks in the preview to jump directly.
 */
export const Navigation: Story = {
  decorators: [
    createProvidersDecorator(navigationBlocks),
    (Story) => (
      <div style={{ width: "100vw", height: "100vh", overflow: "auto" }}>
        <Story />
      </div>
    ),
  ],
  render: () => <NavigationDemo />,
};

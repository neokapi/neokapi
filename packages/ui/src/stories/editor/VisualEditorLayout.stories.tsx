import type { Meta, StoryObj } from "@storybook/react";
import { fn } from "@storybook/test";
import { VisualEditorLayout } from "../../components/editor/VisualEditorLayout";
import {
  sampleBlocks, sampleProject,
  sampleTMMatches, sampleTermMatches,
  sampleQAIssues, sampleFileQAResults,
  sampleBlockNotes, sampleBlockHistory,
} from "../fixtures";
import { withProviders } from "../decorators";

const meta: Meta<typeof VisualEditorLayout> = {
  title: "Editor/VisualEditorLayout",
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
    editorMode: "translate",
    onEditorModeChange: fn(),
    previewContentMode: "source",
    onPreviewContentModeChange: fn(),
    onNavigate: fn(),
    onStartEditing: fn(),
    onSave: fn(),
    onCancelEditing: fn(),
    onApprove: fn(),
    onReject: fn(),
    tmMatches: [],
    termMatches: [],
    onApplyTM: fn(),
    onInsertTerm: fn(),
  },
};

export default meta;
type Story = StoryObj<typeof VisualEditorLayout>;

/** Default layout in translate mode */
export const Default: Story = {};

/** Second block selected */
export const SecondBlockSelected: Story = {
  args: {
    selectedIndex: 1,
  },
};

/** Translate mode with TM matches */
export const WithTMMatches: Story = {
  args: {
    tmMatches: sampleTMMatches,
  },
};

/** Layout with term sidebar visible */
export const WithTermSidebar: Story = {
  args: {
    termMatches: sampleTermMatches,
  },
};

/** Review mode */
export const ReviewMode: Story = {
  args: {
    editorMode: "review",
  },
};

/** Enrich mode with notes and term creation */
export const EnrichMode: Story = {
  args: {
    editorMode: "enrich",
    notes: sampleBlockNotes,
    onAddNote: fn(),
    onDeleteNote: fn(),
    onTermCreate: fn(),
  },
};

/** Layout with QA issues and file QA results */
export const WithQAIssues: Story = {
  args: {
    qaIssues: sampleQAIssues,
    fileQAResults: sampleFileQAResults,
    onRunFileQA: fn(),
  },
};

/** Layout with block history */
export const WithHistory: Story = {
  args: {
    history: sampleBlockHistory,
    onRevertHistory: fn(),
  },
};

/** Target preview content mode */
export const TargetPreview: Story = {
  args: {
    previewContentMode: "target",
  },
};

/** Full featured: all panels, TM, terms, QA, history, notes */
export const FullFeatured: Story = {
  args: {
    selectedIndex: 1,
    tmMatches: sampleTMMatches,
    termMatches: sampleTermMatches,
    qaIssues: sampleQAIssues,
    fileQAResults: sampleFileQAResults,
    onRunFileQA: fn(),
    history: sampleBlockHistory,
    onRevertHistory: fn(),
    notes: sampleBlockNotes,
    onAddNote: fn(),
    onDeleteNote: fn(),
    onTermCreate: fn(),
    presenceSlot: (
      <div style={{ display: "flex", gap: 4 }}>
        <div style={{ width: 24, height: 24, borderRadius: "50%", background: "#6366f1", display: "flex", alignItems: "center", justifyContent: "center", color: "#fff", fontSize: 11, fontWeight: 600 }}>JD</div>
        <div style={{ width: 24, height: 24, borderRadius: "50%", background: "#f59e0b", display: "flex", alignItems: "center", justifyContent: "center", color: "#fff", fontSize: 11, fontWeight: 600 }}>MK</div>
      </div>
    ),
  },
};

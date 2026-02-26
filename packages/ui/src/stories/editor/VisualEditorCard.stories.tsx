import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { VisualEditorCard } from "../../components/editor/VisualEditorCard";
import {
  sampleBlocks, sampleProject, simpleBoldSpans,
  sampleTMMatches, sampleTermMatches,
  sampleQAIssues, sampleBlockNotes, sampleBlockHistory,
} from "../fixtures";

const baseBlock = sampleBlocks[1]; // "Click here to continue" — has spans

const meta: Meta<typeof VisualEditorCard> = {
  title: "Editor/VisualEditorCard",
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
    tmMatches: [],
    termMatches: [],
    onApplyTM: fn(),
    onInsertTerm: fn(),
    project: sampleProject,
  },
};

export default meta;
type Story = StoryObj<typeof VisualEditorCard>;

/** Default translate mode — not editing, no TM matches */
export const TranslateMode: Story = {};

/** Translate mode with editing active */
export const TranslateModeEditing: Story = {
  args: {
    isEditing: true,
  },
};

/** Translate mode with TM matches shown */
export const WithTMMatches: Story = {
  args: {
    tmMatches: sampleTMMatches,
  },
};

/** Review mode with approve/reject buttons */
export const ReviewMode: Story = {
  args: {
    editorMode: "review",
    block: sampleBlocks[0],
    blockIndex: 0,
  },
};

/** Enrich mode with notes section */
export const EnrichMode: Story = {
  args: {
    editorMode: "enrich",
    notes: sampleBlockNotes,
    onAddNote: fn(),
    onDeleteNote: fn(),
    onTermCreate: fn(),
  },
};

/** Enrich mode with no existing notes */
export const EnrichModeEmpty: Story = {
  args: {
    editorMode: "enrich",
    notes: [],
    onAddNote: fn(),
    onDeleteNote: fn(),
    onTermCreate: fn(),
  },
};

/** Card showing QA issues (errors + warnings) */
export const WithQAIssues: Story = {
  args: {
    qaIssues: sampleQAIssues,
  },
};

/** Card with block history entries */
export const WithHistory: Story = {
  args: {
    history: sampleBlockHistory,
    onRevertHistory: fn(),
  },
};

/** Card with reference locales displayed */
export const WithReferenceLocales: Story = {
  args: {
    referenceLocales: ["de-DE"],
    block: sampleBlocks[0], // has de-DE target
    blockIndex: 0,
  },
};

/** Not-started block (no target text) */
export const NotStartedBlock: Story = {
  args: {
    block: sampleBlocks[2], // empty targets
    blockIndex: 2,
  },
};

/** Reviewed block status */
export const ReviewedBlock: Story = {
  args: {
    block: {
      ...sampleBlocks[0],
      properties: { "translation-status": "reviewed" },
    },
    blockIndex: 0,
  },
};

/** Full featured: TM matches, QA, history, ref locales, term matches */
export const FullFeatured: Story = {
  args: {
    block: sampleBlocks[0],
    blockIndex: 0,
    tmMatches: sampleTMMatches,
    termMatches: sampleTermMatches,
    qaIssues: sampleQAIssues,
    history: sampleBlockHistory,
    onRevertHistory: fn(),
    referenceLocales: ["de-DE"],
  },
};

import type { Meta, StoryObj } from "@storybook/react-vite";
import { TMGroupedEntry } from "../../components/resource-browser/TMGroupedEntry";
import type { TMGroupedResult } from "../../components/resource-browser/types";
import { fn } from "storybook/test";

const now = new Date().toISOString();

const basicGroup: TMGroupedResult = {
  source_text: "The client library handles token refresh and session management.",
  source_coded: "The client library handles token refresh and session management.",
  source_spans: [],
  source_locale: "en-US",
  targets: [
    {
      id: "t1",
      target_text: "La biblioth\u00e8que cliente g\u00e8re le renouvellement des jetons et la gestion des sessions.",
      target_coded: "La biblioth\u00e8que cliente g\u00e8re le renouvellement des jetons et la gestion des sessions.",
      target_spans: [],
      target_locale: "fr-FR",
      project_id: "webapp",
      updated_at: now,
    },
    {
      id: "t2",
      target_text: "Die Client-Bibliothek \u00fcbernimmt die Token-Erneuerung und Sitzungsverwaltung.",
      target_coded: "Die Client-Bibliothek \u00fcbernimmt die Token-Erneuerung und Sitzungsverwaltung.",
      target_spans: [],
      target_locale: "de-DE",
      project_id: "webapp",
      updated_at: now,
    },
    {
      id: "t3",
      target_text: "\u30af\u30e9\u30a4\u30a2\u30f3\u30c8\u30e9\u30a4\u30d6\u30e9\u30ea\u306f\u30c8\u30fc\u30af\u30f3\u306e\u66f4\u65b0\u3068\u30bb\u30c3\u30b7\u30e7\u30f3\u7ba1\u7406\u3092\u81ea\u52d5\u7684\u306b\u51e6\u7406\u3057\u307e\u3059\u3002",
      target_coded: "\u30af\u30e9\u30a4\u30a2\u30f3\u30c8\u30e9\u30a4\u30d6\u30e9\u30ea\u306f\u30c8\u30fc\u30af\u30f3\u306e\u66f4\u65b0\u3068\u30bb\u30c3\u30b7\u30e7\u30f3\u7ba1\u7406\u3092\u81ea\u52d5\u7684\u306b\u51e6\u7406\u3057\u307e\u3059\u3002",
      target_spans: [],
      target_locale: "ja-JP",
      project_id: "",
      updated_at: now,
    },
  ],
};

const singleTarget: TMGroupedResult = {
  source_text: "This action cannot be undone.",
  source_coded: "This action cannot be undone.",
  source_spans: [],
  source_locale: "en-US",
  targets: [
    {
      id: "t4",
      target_text: "Cette action ne peut pas \u00eatre annul\u00e9e.",
      target_coded: "Cette action ne peut pas \u00eatre annul\u00e9e.",
      target_spans: [],
      target_locale: "fr-FR",
      project_id: "",
      updated_at: now,
    },
  ],
};

const meta: Meta<typeof TMGroupedEntry> = {
  title: "Resource Browser/TMGroupedEntry",
  component: TMGroupedEntry,
  tags: ["autodocs"],
  decorators: [
    (Story: React.ComponentType) => (
      <div style={{ maxWidth: 700, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
  parameters: {
    docs: {
      description: {
        component:
          "Expandable card for multi-language view. Shows source text with translation count; expands to reveal all target translations.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof TMGroupedEntry>;

export const Collapsed: Story = {
  args: {
    group: basicGroup,
    selected: false,
    onToggleSelect: fn(),
    onEditTarget: fn(),
    onDeleteTarget: fn(),
  },
};

export const Selected: Story = {
  args: {
    group: basicGroup,
    selected: true,
    onToggleSelect: fn(),
    onEditTarget: fn(),
    onDeleteTarget: fn(),
  },
};

export const SingleTranslation: Story = {
  args: {
    group: singleTarget,
    selected: false,
    onToggleSelect: fn(),
    onEditTarget: fn(),
    onDeleteTarget: fn(),
  },
};

export const MultipleEntries: Story = {
  render: () => (
    <div className="space-y-2">
      <TMGroupedEntry
        group={basicGroup}
        selected={false}
        onToggleSelect={fn()}
        onEditTarget={fn()}
        onDeleteTarget={fn()}
      />
      <TMGroupedEntry
        group={singleTarget}
        selected={true}
        onToggleSelect={fn()}
        onEditTarget={fn()}
        onDeleteTarget={fn()}
      />
    </div>
  ),
};

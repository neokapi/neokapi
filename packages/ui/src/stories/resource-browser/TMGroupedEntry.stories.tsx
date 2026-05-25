import type { Meta, StoryObj } from "@storybook/react-vite";
import { TMGroupedEntry } from "../../components/resource-browser/TMGroupedEntry";
import type { TMEntryDTO, VariantDTO } from "../../components/resource-browser/types";
import { fn } from "storybook/test";

const now = new Date().toISOString();

function v(locale: string, text: string): VariantDTO {
  return { locale, text, runs: [{ text }] };
}

const basicEntry: TMEntryDTO = {
  id: "tm-basic",
  project_id: "webapp",
  hint_src_lang: "en-US",
  variants: {
    "en-US": v("en-US", "The client library handles token refresh and session management."),
    "fr-FR": v(
      "fr-FR",
      "La biblioth\u00e8que cliente g\u00e8re le renouvellement des jetons et la gestion des sessions.",
    ),
    "de-DE": v(
      "de-DE",
      "Die Client-Bibliothek \u00fcbernimmt die Token-Erneuerung und Sitzungsverwaltung.",
    ),
    "ja-JP": v(
      "ja-JP",
      "\u30af\u30e9\u30a4\u30a2\u30f3\u30c8\u30e9\u30a4\u30d6\u30e9\u30ea\u306f\u30c8\u30fc\u30af\u30f3\u306e\u66f4\u65b0\u3068\u30bb\u30c3\u30b7\u30e7\u30f3\u7ba1\u7406\u3092\u81ea\u52d5\u7684\u306b\u51e6\u7406\u3057\u307e\u3059\u3002",
    ),
  },
  created_at: now,
  updated_at: now,
};

const singleTarget: TMEntryDTO = {
  id: "tm-single",
  project_id: "",
  hint_src_lang: "en-US",
  variants: {
    "en-US": v("en-US", "This action cannot be undone."),
    "fr-FR": v("fr-FR", "Cette action ne peut pas \u00eatre annul\u00e9e."),
  },
  created_at: now,
  updated_at: now,
};

const manyTargets: TMEntryDTO = {
  ...basicEntry,
  id: "tm-many",
  variants: {
    ...basicEntry.variants,
    "it-IT": v("it-IT", "Il client gestisce..."),
    "pt-BR": v("pt-BR", "O cliente gerencia..."),
    "ar-SA": v("ar-SA", "\u0645\u0643\u062a\u0628\u0629 \u0627\u0644\u0639\u0645\u064a\u0644..."),
    "ko-KR": v("ko-KR", "\ud074\ub77c\uc774\uc5b8\ud2b8 \ub77c\uc774\ube0c\ub7ec\ub9ac..."),
    "zh-CN": v("zh-CN", "\u5ba2\u6237\u7aef\u5e93..."),
    "sv-SE": v("sv-SE", "Klientbiblioteket..."),
    "nb-NO": v("nb-NO", "Klientbiblioteket..."),
    "es-ES": v("es-ES", "La biblioteca cliente..."),
    "nl-NL": v("nl-NL", "De clientbibliotheek..."),
    "pl-PL": v("pl-PL", "Biblioteka klienta..."),
  },
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
          "Expandable card for a multilingual TM entry. The hint_src_lang variant is shown as the header; every other variant is rendered beneath.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof TMGroupedEntry>;

export const AutoExpanded: Story = {
  args: {
    entry: basicEntry,
    selected: false,
    onToggleSelect: fn(),
    onEditVariant: fn(),
    onDelete: fn(),
  },
};

export const Selected: Story = {
  args: {
    entry: basicEntry,
    selected: true,
    onToggleSelect: fn(),
    onEditVariant: fn(),
    onDelete: fn(),
  },
};

export const SingleTranslation: Story = {
  args: {
    entry: singleTarget,
    selected: false,
    onToggleSelect: fn(),
    onEditVariant: fn(),
    onDelete: fn(),
  },
};

/** Only shows variants for the specified locales; count shows "visible/total". */
export const FilteredByLocale: Story = {
  args: {
    entry: basicEntry,
    selected: false,
    visibleLocales: ["fr-FR", "de-DE"],
    onToggleSelect: fn(),
    onEditVariant: fn(),
    onDelete: fn(),
  },
};

/** Single locale filter — only French. */
export const SingleLocaleFilter: Story = {
  args: {
    entry: basicEntry,
    selected: false,
    visibleLocales: ["fr-FR"],
    onToggleSelect: fn(),
    onEditVariant: fn(),
    onDelete: fn(),
  },
};

/** Entries with 10+ non-source variants stay collapsed by default. */
export const ManyTargetsCollapsed: Story = {
  args: {
    entry: manyTargets,
    selected: false,
    onToggleSelect: fn(),
    onEditVariant: fn(),
    onDelete: fn(),
  },
};

export const MultipleEntries: Story = {
  render: () => (
    <div className="space-y-2">
      <TMGroupedEntry
        entry={basicEntry}
        selected={false}
        onToggleSelect={fn()}
        onEditVariant={fn()}
        onDelete={fn()}
      />
      <TMGroupedEntry
        entry={singleTarget}
        selected={true}
        onToggleSelect={fn()}
        onEditVariant={fn()}
        onDelete={fn()}
      />
    </div>
  ),
};

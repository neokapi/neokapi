import type { Meta, StoryObj } from "@storybook/react-vite";
import { ConceptCard } from "../../components/resource-browser/ConceptCard";
import { fn } from "storybook/test";

const sampleConcept = {
  id: "c1",
  project_id: "proj-1",
  domain: "e-commerce",
  definition: "A temporary container where customers collect products before purchasing.",
  source: "terminology" as const,
  terms: [
    { text: "shopping cart", locale: "en-US", status: "preferred" },
    { text: "panier", locale: "fr-FR", status: "preferred" },
    { text: "Warenkorb", locale: "de-DE", status: "preferred" },
    { text: "ショッピングカート", locale: "ja-JP", status: "preferred" },
  ],
  properties: {},
  created_at: "2026-03-15T10:00:00Z",
  updated_at: "2026-03-20T14:30:00Z",
};

const multiStatusConcept = {
  id: "c2",
  project_id: "",
  domain: "legal",
  definition: "A legally binding agreement between two or more parties.",
  source: "terminology" as const,
  terms: [
    { text: "contract", locale: "en-US", status: "preferred" },
    { text: "agreement", locale: "en-US", status: "admitted", note: "informal" },
    { text: "contrat", locale: "fr-FR", status: "preferred" },
    { text: "accord", locale: "fr-FR", status: "deprecated", note: "use contrat instead" },
    { text: "Vertrag", locale: "de-DE", status: "approved" },
  ],
  properties: {},
  created_at: "2026-02-10T10:00:00Z",
  updated_at: "2026-02-10T10:00:00Z",
};

const meta: Meta<typeof ConceptCard> = {
  title: "Resource Browser/ConceptCard",
  component: ConceptCard,
  tags: ["autodocs"],
  args: {
    onToggleSelect: fn(),
    onEdit: fn(),
    onDelete: fn(),
    onDeleteConfirm: fn(),
    onDeleteCancel: fn(),
  },
  parameters: {
    docs: {
      description: {
        component:
          "Termbase concept card with reference language term, target translations, domain badge, and hover actions. Built on shadcn Card/Badge/Button.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof ConceptCard>;

export const Default: Story = {
  args: {
    concept: sampleConcept,
    referenceLocale: "en-US",
  },
};

export const Selected: Story = {
  args: {
    concept: sampleConcept,
    referenceLocale: "en-US",
    selected: true,
  },
};

export const MultipleStatuses: Story = {
  name: "Multiple Statuses",
  args: {
    concept: multiStatusConcept,
    referenceLocale: "en-US",
  },
};

export const NoReferenceLocale: Story = {
  name: "No Reference Locale (first term)",
  args: {
    concept: sampleConcept,
  },
};

export const DeleteConfirm: Story = {
  name: "Delete Confirmation",
  args: {
    concept: sampleConcept,
    referenceLocale: "en-US",
    deleteConfirm: true,
  },
};

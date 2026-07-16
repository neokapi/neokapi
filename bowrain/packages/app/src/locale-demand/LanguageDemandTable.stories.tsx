import type { Meta, StoryObj } from "@storybook/react-vite";
import { LanguageDemandTable } from "./LanguageDemandTable";
import { sampleDemandDataSource } from "./locale-demand-fixtures";

// Locale demand — DESIGN PROTOTYPE (mock data only).

const snapshot = sampleDemandDataSource.getSnapshot("30d");

const meta: Meta<typeof LanguageDemandTable> = {
  title: "Locale Demand/LanguageDemandTable",
  component: LanguageDemandTable,
  decorators: [
    (Story) => (
      <div className="max-w-4xl p-6">
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof LanguageDemandTable>;

export const Default: Story = {
  args: { languages: snapshot.languages },
};

export const RowSelected: Story = {
  args: { languages: snapshot.languages, selectedLanguage: "pt-BR" },
};

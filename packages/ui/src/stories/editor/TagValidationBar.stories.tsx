import type { Meta, StoryObj } from "@storybook/react";
import { TagValidationBar } from "../../components/editor/TagValidationBar";
import type { TagValidationResult } from "../../components/editor/tagSemantics";

const meta: Meta<typeof TagValidationBar> = {
  title: "Editor/TagValidationBar",
  component: TagValidationBar,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 500, padding: 16 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof TagValidationBar>;

export const Valid: Story = {
  args: { validation: { valid: true, errors: [], warnings: [] } },
};

export const MissingTag: Story = {
  args: {
    validation: {
      valid: false,
      errors: [{ type: "missing_tag", message: 'Missing 1 closing "b" tag' }],
      warnings: [],
    },
  },
};

export const ExtraTag: Story = {
  args: {
    validation: {
      valid: true,
      errors: [],
      warnings: [{ type: "extra_tag", message: 'Extra 1 opening "i" tag' }],
    },
  },
};

export const UnpairedTag: Story = {
  args: {
    validation: {
      valid: false,
      errors: [{ type: "unpaired", message: 'Closing "b" without matching opening tag' }],
      warnings: [],
    },
  },
};

export const MultipleIssues: Story = {
  args: {
    validation: {
      valid: false,
      errors: [
        { type: "missing_tag", message: 'Missing 1 opening "a" tag' },
        { type: "unpaired", message: '1 opening "b" tags without matching closing tag' },
      ],
      warnings: [
        { type: "extra_tag", message: 'Extra 2 placeholder "br" tags' },
      ],
    } satisfies TagValidationResult,
  },
};

export const NoValidation: Story = {
  args: { validation: null },
};

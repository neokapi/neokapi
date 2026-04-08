import type { Meta, StoryObj } from "@storybook/react-vite";
import { TagValidationBar } from "../../components/editor/TagValidationBar";
import type { TagValidationResult } from "../../components/editor/tagSemantics";

const meta: Meta<typeof TagValidationBar> = {
  title: "Editor/Tags/TagValidationBar",
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

const noIssues: TagValidationResult = { valid: true, errors: [], warnings: [] };
const withErrors: TagValidationResult = {
  valid: false,
  errors: [
    { type: "missing_tag", message: 'Missing 1 opening "b" tag in target' },
    { type: "unpaired", message: "Unpaired closing tag found" },
  ],
  warnings: [],
};
const withWarnings: TagValidationResult = {
  valid: true,
  errors: [],
  warnings: [{ type: "extra_tag", message: 'Extra 1 closing "i" tag in target' }],
};
const mixed: TagValidationResult = {
  valid: false,
  errors: [{ type: "missing_tag", message: 'Missing "b" opening tag' }],
  warnings: [{ type: "extra_tag", message: 'Extra "code" closing tag' }],
};

export const NoIssues: Story = { args: { validation: noIssues } };
export const Errors: Story = { args: { validation: withErrors } };
export const Warnings: Story = { args: { validation: withWarnings } };
export const Mixed: Story = { args: { validation: mixed } };
export const Null: Story = { args: { validation: null } };

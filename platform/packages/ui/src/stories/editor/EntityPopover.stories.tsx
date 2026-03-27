import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { EntityPopover } from "../../components/editor/EntityPopover";
import type { EntityInfo } from "../../types/api";

const sampleEntity: EntityInfo = {
  key: "ent-1",
  text: "John Smith",
  type: "entity:person",
  start: 0,
  end: 10,
  dnt: false,
};

const dntEntity: EntityInfo = {
  key: "ent-2",
  text: "Acme Corp",
  type: "entity:organization",
  start: 22,
  end: 31,
  dnt: true,
  source: "manual",
};

const meta: Meta<typeof EntityPopover> = {
  title: "Editor/Entities/EntityPopover",
  component: EntityPopover,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ position: "relative", padding: 80 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof EntityPopover>;

/** View/edit an existing entity with all actions. */
export const WithAllActions: Story = {
  args: {
    entity: sampleEntity,
    onClose: fn(),
    onUpdate: fn(),
    onDelete: fn(),
    onPromote: fn(),
  },
};

/** Do-not-translate entity. */
export const DNTEntity: Story = {
  args: {
    entity: dntEntity,
    onClose: fn(),
    onUpdate: fn(),
    onDelete: fn(),
  },
};

/** Read-only — no update/delete actions. */
export const ReadOnly: Story = {
  args: {
    entity: sampleEntity,
    onClose: fn(),
  },
};

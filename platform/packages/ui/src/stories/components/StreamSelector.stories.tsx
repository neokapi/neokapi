import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { StreamSelector } from "../../components/StreamSelector";
import { sampleStreams, mainStream, featureStream } from "./fixtures";

const meta: Meta<typeof StreamSelector> = {
  title: "Components/StreamSelector",
  component: StreamSelector,
  tags: ["autodocs"],
  decorators: [
    (Story) => (
      <div style={{ padding: 24 }}>
        <Story />
      </div>
    ),
  ],
};

export default meta;
type Story = StoryObj<typeof StreamSelector>;

export const OnMain: Story = {
  args: {
    streams: sampleStreams,
    activeStream: mainStream,
    onStreamChange: fn(),
    onCreateStream: fn(),
    onEditStream: fn(),
    onMergeStream: fn(),
    onDiffStream: fn(),
    onDeleteStream: fn(),
  },
};

export const OnFeatureBranch: Story = {
  args: {
    streams: sampleStreams,
    activeStream: featureStream,
    onStreamChange: fn(),
    onCreateStream: fn(),
    onEditStream: fn(),
    onMergeStream: fn(),
    onDiffStream: fn(),
    onDeleteStream: fn(),
  },
};

/** Default stream is "main" (typical case). */
export const DefaultStreamMain: Story = {
  args: {
    streams: sampleStreams,
    activeStream: mainStream,
    defaultStream: "main",
    onStreamChange: fn(),
    onCreateStream: fn(),
  },
};

/** Default stream is a non-main stream (e.g., first push was to "feature/translations"). */
export const DefaultStreamNonMain: Story = {
  args: {
    streams: sampleStreams,
    activeStream: featureStream,
    defaultStream: "feature/translations",
    onStreamChange: fn(),
    onCreateStream: fn(),
    onEditStream: fn(),
    onMergeStream: fn(),
    onDiffStream: fn(),
    onDeleteStream: fn(),
  },
};

/** Active stream differs from the default — both badges visible. */
export const ActiveDiffersFromDefault: Story = {
  args: {
    streams: sampleStreams,
    activeStream: featureStream,
    defaultStream: "main",
    onStreamChange: fn(),
    onCreateStream: fn(),
    onEditStream: fn(),
    onMergeStream: fn(),
    onDiffStream: fn(),
    onDeleteStream: fn(),
  },
};

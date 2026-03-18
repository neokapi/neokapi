import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { StreamSelector } from "../../components/StreamSelector";
import { sampleStreams, mainStream, featureStream } from "./fixtures";

const meta: Meta<typeof StreamSelector> = {
  title: "Components/StreamSelector",
  component: StreamSelector,
  tags: ["autodocs"],
  decorators: [(Story) => <div style={{ padding: 24 }}><Story /></div>],
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

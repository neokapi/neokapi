import type { Meta, StoryObj } from "@storybook/react-vite";
import { DeviceAuthorizedPage } from "./DeviceAuthorizedPage";

const meta: Meta<typeof DeviceAuthorizedPage> = {
  title: "Auth/Web/Device Authorized",
  component: DeviceAuthorizedPage,
  parameters: {
    layout: "fullscreen",
  },
};

export default meta;
type Story = StoryObj<typeof DeviceAuthorizedPage>;

export const Default: Story = {};

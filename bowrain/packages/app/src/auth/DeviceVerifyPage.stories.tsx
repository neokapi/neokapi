import type { Meta, StoryObj } from "@storybook/react-vite";
import { DeviceVerifyPage } from "./DeviceVerifyPage";

const meta: Meta<typeof DeviceVerifyPage> = {
  title: "Auth/Web/Device Verify",
  component: DeviceVerifyPage,
  parameters: {
    layout: "fullscreen",
  },
};

export default meta;
type Story = StoryObj<typeof DeviceVerifyPage>;

export const Default: Story = {};

export const WithPrefillCode: Story = {
  decorators: [
    (Story) => {
      // Simulate ?user_code=ABCD-1234 in the URL
      const url = new URL(window.location.href);
      url.searchParams.set("user_code", "ABCD-1234");
      window.history.replaceState({}, "", url.toString());
      return <Story />;
    },
  ],
};

export const WithError: Story = {
  decorators: [
    (Story) => {
      const url = new URL(window.location.href);
      url.searchParams.set("error", "Invalid or expired device code");
      window.history.replaceState({}, "", url.toString());
      return <Story />;
    },
  ],
};

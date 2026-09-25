import React from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { PlaygroundDialog } from "./PlaygroundDialog";

const meta = {
  title: "Labs/CLI window",
  component: PlaygroundDialog,
  parameters: { layout: "padded" },
  args: {
    children: (
      <div style={{ height: "100%", display: "flex", flexDirection: "column", gap: 12 }}>
        <p>
          This shell example preserves your input when the window closes. The website supplies the
          live terminal.
        </p>
        <textarea
          aria-label="Session notes"
          placeholder="Write a note, close, and resume."
          style={{ flex: 1, resize: "none", padding: 12 }}
        />
      </div>
    ),
  },
} satisfies Meta<typeof PlaygroundDialog>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Closed: Story = {};
export const Open: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole("button", { name: "Open terminal" }));
    await expect(
      within(canvasElement).getByRole("dialog", { name: "CLI playground" }),
    ).toBeVisible();
  },
};
export const ResumeSession: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "Open terminal" }));
    await userEvent.type(canvas.getByRole("textbox"), "Keep this session");
    await userEvent.click(canvas.getByRole("button", { name: "Close terminal" }));
    await userEvent.click(canvas.getByRole("button", { name: "Resume terminal" }));
    await expect(canvas.getByRole("textbox")).toHaveValue("Keep this session");
  },
};

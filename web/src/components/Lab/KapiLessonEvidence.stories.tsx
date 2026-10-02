import React from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import KapiLessonLifecycle from "./KapiLessonLifecycle";
import KapiLessonContextEvidence from "./KapiLessonContextEvidence";
import KapiLessonAIEvidence from "./KapiLessonAIEvidence";

const meta = {
  title: "Labs/Kapi evidence",
  component: KapiLessonLifecycle,
  parameters: { layout: "padded" },
} satisfies Meta<typeof KapiLessonLifecycle>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Decisions: Story = {};
export const EstablishedRule: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.selectOptions(canvas.getByLabelText("Recorded step"), "2");
    await expect(canvas.getByRole("heading", { name: "Recorded check: failed" })).toBeVisible();
    await userEvent.selectOptions(canvas.getByLabelText("Recorded step"), "5");
    await expect(canvas.getByRole("heading", { name: "Recorded check: passed" })).toBeVisible();
  },
};
export const Context: Story = { render: () => <KapiLessonContextEvidence /> };
export const Authoring: Story = { render: () => <KapiLessonAIEvidence /> };

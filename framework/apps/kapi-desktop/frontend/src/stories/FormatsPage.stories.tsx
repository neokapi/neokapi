import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { FormatsPage } from "../components/FormatsPage";

const meta: Meta<typeof FormatsPage> = {
  title: "Pages/FormatsPage",
  component: FormatsPage,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof FormatsPage>;

/**
 * Default view — the format list with both built-in and plugin formats.
 * In Storybook the API returns null so formats are empty.
 * Use FormatDetail stories for rich simulated data.
 */
export const Default: Story = {};

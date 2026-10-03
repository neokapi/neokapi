import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import { StaleChangeDialog } from "../../components/editor/StaleChangeDialog";
import { withProviders } from "../decorators";

// What a person sees when a change they sent meets a translation someone else
// changed after they opened it: the server refused it and nothing was written.
// The dialog shows the translation as it stands (inline codes as chips) and,
// for a save, the person's version, and asks whether to apply the change to the
// translation that stands or keep it.

const meta: Meta<typeof StaleChangeDialog> = {
  title: "Editor/StaleChangeDialog",
  component: StaleChangeDialog,
  decorators: [withProviders],
  parameters: { layout: "centered" },
};

export default meta;
type Story = StoryObj<typeof StaleChangeDialog>;

const handlers = () => ({ onReapply: fn(), onKeep: fn() });

/** A save over a translation someone else saved, with a link in both versions. */
export const SaveOverAChangedTranslation: Story = {
  args: {
    state: {
      action: "save",
      locale: "fr-FR",
      mine: 'Lisez le <x id="1"/>guide<x id="/1"/> avant de commencer.',
      current: {
        rev: "r:3f2a9c0d41b7e5a8",
        text: 'Consultez le <x id="1"/>guide<x id="/1"/> avant de commencer.',
      },
      ...handlers(),
    },
  },
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await expect(await body.findByTestId("stale-current")).toHaveTextContent("Consultez le");
    await expect(body.getByTestId("stale-mine")).toHaveTextContent("Lisez le");
    await expect(body.getByTestId("stale-reapply")).toHaveTextContent("Save my version");
  },
};

/** An approval of wording that changed after the reviewer read it. */
export const ApproveAChangedTranslation: Story = {
  args: {
    state: {
      action: "establish",
      locale: "de-DE",
      current: { rev: "r:91c04be2a7d36f10", text: "Willkommen bei Neokapi!" },
      ...handlers(),
    },
  },
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await expect(await body.findByTestId("stale-reapply")).toHaveTextContent(
      "Approve this version",
    );
    await expect(body.queryByTestId("stale-mine")).not.toBeInTheDocument();
  },
};

/** A rejection of wording that changed after the reviewer read it. */
export const RejectAChangedTranslation: Story = {
  args: {
    state: {
      action: "reject",
      locale: "fr-FR",
      current: { rev: "r:5d8e21a0c3f49b76", text: "Bienvenue dans Neokapi" },
      ...handlers(),
    },
  },
};

/** The translation was removed after the person opened the block; saving creates it again. */
export const TranslationRemovedMeanwhile: Story = {
  args: {
    state: {
      action: "save",
      locale: "fr-FR",
      mine: "Bienvenue sur Neokapi",
      current: { rev: "absent", text: "" },
      ...handlers(),
    },
  },
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await expect(await body.findByTestId("stale-reapply")).toHaveTextContent("Save my version");
  },
};

/**
 * An approval of a translation someone removed after the reviewer read it.
 * There is nothing left to approve, so the dialog only closes.
 */
export const ApproveARemovedTranslation: Story = {
  args: {
    state: {
      action: "establish",
      locale: "fr-FR",
      current: { rev: "absent", text: "" },
      ...handlers(),
    },
  },
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await expect(await body.findByTestId("stale-keep")).toHaveTextContent("Close");
    await expect(body.queryByTestId("stale-reapply")).not.toBeInTheDocument();
  },
};

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ErrorNotice } from "../../components/ui/error-notice";

const wailsEnvelope =
  '{"message":"no default flow configured — add a flow or pick one to run","cause":{},"kind":"RuntimeError"}';

const richEnvelope = JSON.stringify({
  message: "push failed: the server rejected 3 of 128 blocks",
  kind: "SyncError",
  cause: {
    message: "block 7f3a: content hash mismatch",
    path: "src/locales/en-US.json",
    attempt: 2,
  },
});

const meta: Meta<typeof ErrorNotice> = {
  title: "Foundations/ErrorNotice",
  component: ErrorNotice,
  tags: ["autodocs"],
  parameters: {
    docs: {
      description: {
        component:
          "Friendly-first error surface. parseAppError normalises Wails envelopes, nested JSON strings, Error instances, and plain strings into a friendly line; the raw structured payload stays available behind a Details disclosure, rendered as theme-aware syntax-highlighted JSON.",
      },
    },
  },
};

export default meta;
type Story = StoryObj<typeof ErrorNotice>;

export const InlineWailsEnvelope: Story = {
  name: "Inline — Wails envelope",
  args: {
    error: wailsEnvelope,
    variant: "inline",
  },
};

export const PanelWithJsonDetail: Story = {
  name: "Panel — structured cause, JSON detail",
  args: {
    error: richEnvelope,
    variant: "panel",
    hint: "Retry the push once the server is reachable, or narrow the collection.",
  },
};

export const ContextualTitle: Story = {
  name: "Panel — contextual title + parsed secondary",
  args: {
    error: wailsEnvelope,
    title: "Failed to bring the project up to date",
    variant: "panel",
  },
};

export const PlainString: Story = {
  name: "Inline — plain string (no disclosure)",
  args: {
    error: "The file could not be opened.",
    variant: "inline",
  },
};

export const ErrorInstance: Story = {
  name: "Panel — Error instance (stack in details)",
  args: {
    error: new Error("fetch failed: ECONNREFUSED 127.0.0.1:8080"),
    variant: "panel",
  },
};

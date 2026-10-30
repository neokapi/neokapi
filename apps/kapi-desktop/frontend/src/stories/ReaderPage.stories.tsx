import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ReaderPage } from "../components/ReaderPage";
import {
  READER_CLEAN,
  READER_REFUSED,
  READER_WITH_FIXES,
  READER_WITHOUT_FIXES,
} from "./fixtures/readerFile";

/** The page reads through react-query; a story hands it the file instead. */
function withClient(Story: React.ComponentType) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false, staleTime: Infinity } },
  });
  return (
    <QueryClientProvider client={client}>
      <div className="h-[56rem]">
        <Story />
      </div>
    </QueryClientProvider>
  );
}

/**
 * The Reader: one file's blocks with each finding on the words it is about,
 * and the rules that hold at the file down the margin. A finding with a fix
 * carries Fix; the header's button applies every fix in the file.
 */
const meta: Meta<typeof ReaderPage> = {
  title: "Pages/ReaderPage",
  component: ReaderPage,
  tags: ["autodocs"],
  args: {
    tabID: "tab-fernwell",
    path: "docs/billing.md",
    reader: READER_WITH_FIXES,
    onBack: fn(),
    backLabel: "Back to the Map",
    onNavigate: fn(),
  },
  decorators: [withClient],
  parameters: { layout: "fullscreen" },
};

export default meta;
type Story = StoryObj<typeof ReaderPage>;

/** Two term rules with a replacement, each with Fix, and a voice rule edited by hand. */
export const FindingsWithFixes: Story = {};

/** A voice rule and a placeholder a translation lost: neither has a fix. */
export const FindingsWithoutFixes: Story = {
  args: { reader: READER_WITHOUT_FIXES },
};

/** No findings: the file and the rules that hold there. */
export const NoFindings: Story = {
  args: { reader: READER_CLEAN },
};

/** A fix the change service refused, with its reason under the finding. */
export const FixRefused: Story = {
  args: { reader: READER_REFUSED },
};

/** Opened from a finding on the Checks page, at its block. */
export const AtABlock: Story = {
  args: { focusBlockID: "p3", backLabel: "Back to checks" },
};

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect } from "react";
import { fn } from "storybook/test";
import { PageHeader, useGuide } from "@neokapi/ui-primitives";
import { DesktopGuide } from "../guide/DesktopGuide";
import { TOPIC_IDS } from "../guide/topics";

/** Opens the Help panel on one topic, or on its list without one. */
function OpenOn({ topic }: { topic?: string }) {
  const guide = useGuide();
  useEffect(() => {
    guide.openHelp(topic);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [topic]);
  return null;
}

const meta = {
  title: "Components/HelpTopics",
  parameters: { layout: "fullscreen" },
  args: { topic: TOPIC_IDS.decide as string | undefined },
  render: ({ topic }) => (
    <DesktopGuide navigate={fn()} hasProject onOpenUrl={fn()}>
      <div className="p-6">
        <PageHeader title="Decide" help={TOPIC_IDS.decide} />
      </div>
      <OpenOn topic={topic} />
    </DesktopGuide>
  ),
} satisfies Meta<{ topic?: string }>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The Decide section of the Kapi Desktop overview, read in the app. */
export const Decide: Story = {};

/** The Overview section, whose links open the Decide, Reader and checks topics. */
export const Overview: Story = { args: { topic: TOPIC_IDS.overview } };

/** The list of guides and topics, with the info button beside the title. */
export const List: Story = { args: { topic: undefined } };

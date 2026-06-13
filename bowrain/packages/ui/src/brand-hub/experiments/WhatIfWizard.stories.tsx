import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { Button } from "@neokapi/ui-primitives";
import { WhatIfWizard } from "./WhatIfWizard";
import { withBrandHub } from "../../stories/brandHubFixtures";

const meta: Meta<typeof WhatIfWizard> = {
  title: "Brand Hub/Experiments/WhatIfWizard",
  component: WhatIfWizard,
  parameters: { layout: "centered" },
  decorators: [withBrandHub],
};

export default meta;
type Story = StoryObj<typeof WhatIfWizard>;

/** Open the wizard: name the experiment, then build ops with a live blast radius. */
export const Default: Story = {
  render: () => {
    const [open, setOpen] = useState(true);
    return (
      <div style={{ padding: 24 }}>
        <Button onClick={() => setOpen(true)}>Compose an experiment</Button>
        <WhatIfWizard open={open} onOpenChange={setOpen} onSubmitted={fn()} />
      </div>
    );
  },
};

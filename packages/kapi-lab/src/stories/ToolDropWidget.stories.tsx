import type { Meta, StoryObj } from "@storybook/react-vite";
import ToolDropWidget from "../ToolDropWidget";
import PseudoTranslateWidget from "../PseudoTranslateWidget";
import StatsWidget from "../StatsWidget";
import SearchReplaceWidget from "../SearchReplaceWidget";

// These stories render the widget shells at idle (assets=null), so they show the
// drop-zone, sample chips, and (for search-replace) the find/replace controls
// without booting the WASM runtime. In the docs, the host injects real asset
// URLs and the widget runs the tool in-browser on the picked file.
const meta: Meta = {
  title: "Lab/Tool Drop Widget",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

export const PseudoTranslate: Story = {
  name: "Pseudo-translate",
  render: () => (
    <div className="max-w-3xl">
      <PseudoTranslateWidget assets={null} />
    </div>
  ),
};

export const Stats: Story = {
  name: "Stats (word-count)",
  render: () => (
    <div className="max-w-3xl">
      <StatsWidget assets={null} />
    </div>
  ),
};

export const SearchReplace: Story = {
  name: "Search / replace",
  render: () => (
    <div className="max-w-3xl">
      <SearchReplaceWidget assets={null} />
    </div>
  ),
};

export const Generic: Story = {
  name: "Generic (custom argv)",
  render: () => (
    <div className="max-w-3xl">
      <ToolDropWidget
        assets={null}
        tool="case-transform"
        buildArgv={(i, o) => ["case-transform", i, "-o", o, "--mode", "upper"]}
        sampleIds={["json"]}
        render="output"
        autoRun={false}
      />
    </div>
  ),
};

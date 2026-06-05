import { useMemo, useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "@neokapi/ui-primitives";
import OutputView from "../OutputView";
import type { ContentNode, ContentTree } from "../types";
import { makeMockRuntime, mockTree, sampleJson } from "./mockData";

const meta: Meta = {
  title: "Lab/Output View",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

function clone<T>(x: T): T {
  return JSON.parse(JSON.stringify(x)) as T;
}

// An "untranslated" variant of the tree: same blocks, no targets/overlays —
// so re-running to the full tree marks the blocks (and lines) as changed.
function stripBlock(n: ContentNode) {
  if (n.kind === "block") {
    delete n.targets;
    delete n.targetMeta;
    delete n.overlays;
    delete n.annotations;
  }
  n.children?.forEach(stripBlock);
}

const beforeTree: ContentTree = (() => {
  const t = clone(mockTree);
  t.root.forEach(stripBlock);
  return t;
})();

const translatedJson = `{
  "greeting": "Bonjour, {name} !",
  "cart": {
    "empty": "Votre panier est vide",
    "checkout": "Passer à la caisse"
  },
  "farewell": "À demain"
}
`;

function OutputDemo() {
  const [stage, setStage] = useState(0);
  const text = stage === 0 ? sampleJson : translatedJson;
  const tree = stage === 0 ? beforeTree : mockTree;
  const runtime = useMemo(() => makeMockRuntime(text, tree), [text, tree]);
  return (
    <div className="flex max-w-3xl flex-col gap-3">
      <Button className="self-start" onClick={() => setStage((s) => (s + 1) % 2)}>
        Simulate re-run (translate)
      </Button>
      <OutputView runtime={runtime} path="/project/messages.json" version={stage} />
    </div>
  );
}

export const ThreeViews: Story = {
  name: "Blocks / Structure / Native",
  render: () => {
    const runtime = makeMockRuntime(sampleJson, mockTree);
    return (
      <div className="max-w-3xl">
        <OutputView runtime={runtime} path="/project/messages.json" />
      </div>
    );
  },
};

export const ChangeAnimation: Story = {
  name: "Write pulse on re-run",
  render: () => <OutputDemo />,
};

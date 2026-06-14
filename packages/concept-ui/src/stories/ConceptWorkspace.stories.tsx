import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { ConceptList } from "../ConceptList";
import { ConceptView } from "../ConceptView";
import type { ConceptDataSource } from "../adapter";
import { makeMemorySource } from "./fixtures";
import { demoSlots } from "./demo-panels";

// The whole experience: the list is the entry; opening a concept shows its view;
// a relation re-centres the view; back returns to the list.
function Workspace({ source }: { source: ConceptDataSource }) {
  const [openId, setOpenId] = useState<string | null>(null);
  return (
    <div className="mx-auto max-w-5xl p-6">
      {openId ? (
        <ConceptView
          conceptId={openId}
          source={source}
          slots={demoSlots}
          onNavigate={setOpenId}
          onBack={() => setOpenId(null)}
          onEdit={() => undefined}
        />
      ) : (
        <ConceptList source={source} onOpen={setOpenId} />
      )}
    </div>
  );
}

const richSource = makeMemorySource();

const meta: Meta<typeof Workspace> = {
  title: "Concept UI/Workspace",
  parameters: { layout: "fullscreen" },
};

export default meta;
type Story = StoryObj<typeof Workspace>;

export const ListToConcept: Story = {
  render: () => <Workspace source={richSource} />,
};

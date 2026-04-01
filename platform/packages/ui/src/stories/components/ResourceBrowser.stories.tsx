import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import {
  ResourceBrowser,
  type ResourceBrowserProps,
  type ResourceInfo,
} from "../../components/ResourceBrowser";
import { Button } from "../../components/ui/button";

const meta: Meta<typeof ResourceBrowser> = {
  title: "Workspace/Resources/ResourceBrowser",
  component: ResourceBrowser,
  tags: ["autodocs"],
};

export default meta;
type Story = StoryObj<typeof ResourceBrowser>;

const sampleTMs: ResourceInfo[] = [
  {
    name: "project-memory",
    kind: "tm",
    path: "~/.config/kapi/tm/project-memory.db",
    entryCount: 12450,
    sourceLocale: "en",
    targetLocales: ["fr", "de"],
  },
  {
    name: "legacy-tm",
    kind: "tm",
    path: "~/.config/kapi/tm/legacy-tm.db",
    entryCount: 85000,
    sourceLocale: "en",
    targetLocales: ["fr"],
  },
  {
    name: "small-test",
    kind: "tm",
    path: "~/.config/kapi/tm/small-test.db",
    entryCount: 120,
    sourceLocale: "en",
    targetLocales: ["de", "ja"],
  },
];

const sampleTermbases: ResourceInfo[] = [
  {
    name: "glossary",
    kind: "termbase",
    path: "~/.config/kapi/termbases/glossary.db",
    entryCount: 340,
    sourceLocale: "en",
    targetLocales: ["fr", "de", "ja"],
  },
  {
    name: "brand-terms",
    kind: "termbase",
    path: "~/.config/kapi/termbases/brand-terms.db",
    entryCount: 52,
    sourceLocale: "en",
    targetLocales: ["fr", "de"],
  },
];

const sampleSRX: ResourceInfo[] = [
  {
    name: "custom-rules",
    kind: "srx",
    path: "~/.config/kapi/srx/custom-rules.srx",
  },
  {
    name: "japanese-rules",
    kind: "srx",
    path: "~/.config/kapi/srx/japanese-rules.srx",
  },
];

function InteractiveBrowser(
  props: Omit<ResourceBrowserProps, "open" | "onClose" | "onSelect">,
) {
  const [open, setOpen] = useState(true);
  const [lastSelected, setLastSelected] = useState<string | null>(null);

  return (
    <div className="flex flex-col gap-4">
      <Button onClick={() => setOpen(true)} variant="outline">
        Open Browser
      </Button>
      {lastSelected && (
        <p className="text-sm text-muted-foreground">
          Selected: <code className="rounded bg-muted px-1.5 py-0.5">{lastSelected}</code>
        </p>
      )}
      <ResourceBrowser
        {...props}
        open={open}
        onClose={() => setOpen(false)}
        onSelect={(ref) => {
          setLastSelected(ref);
          setOpen(false);
        }}
      />
    </div>
  );
}

export const TMBrowser: Story = {
  render: () => (
    <InteractiveBrowser resourceKind="tm" resources={sampleTMs} />
  ),
};

export const TermbaseBrowser: Story = {
  render: () => (
    <InteractiveBrowser resourceKind="termbase" resources={sampleTermbases} />
  ),
};

export const SrxBrowser: Story = {
  render: () => (
    <InteractiveBrowser resourceKind="srx" resources={sampleSRX} />
  ),
};

export const EmptyState: Story = {
  render: () => (
    <InteractiveBrowser resourceKind="tm" resources={[]} />
  ),
};

export const WithSelection: Story = {
  args: {
    open: true,
    onClose: fn(),
    onSelect: fn(),
    resourceKind: "tm",
    resources: sampleTMs,
  },
};

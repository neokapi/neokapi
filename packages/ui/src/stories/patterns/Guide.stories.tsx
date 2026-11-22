import type { Meta, StoryObj } from "@storybook/react-vite";
import * as React from "react";
import { fn } from "storybook/test";
import { FileTextIcon, HelpCircleIcon, PlayIcon } from "lucide-react";

import { GuideProvider, useGuide } from "../../components/guide";
import type { Guide, HelpTopic } from "../../components/guide";
import { Button } from "../../components/ui/button";
import { PortalThemeProvider } from "../../lib/portal-theme";

/**
 * A guided tour lights one element at a time and dims the rest of the window.
 * The Help panel lists the tours and the help topics, with a filter over both.
 */
const meta: Meta = {
  title: "Patterns/Guide",
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj;

const GUIDES: Guide[] = [
  {
    id: "check-a-file",
    title: "Check and fix a file",
    summary: "Run the checks on one file and fix what they find",
    topic: "checks",
    steps: [
      {
        id: "files",
        target: '[data-slot="demo-files"]',
        title: "Pick a file",
        body: "The **Files** list shows every file in the project.",
        placement: "right",
      },
      {
        id: "check",
        target: '[data-slot="demo-check"]',
        title: "Run the checks",
        body: "Press **Check** to run the project's checks on the file.",
        placement: "bottom",
        advanceOn: "target",
      },
      {
        id: "findings",
        target: '[data-slot="demo-findings"]',
        title: "Read the findings",
        body: "Each finding names the rule it broke and the text it found.",
        placement: "top",
      },
    ],
  },
  {
    id: "share",
    title: "Share a project",
    summary: "Invite someone to work on the project with you",
    available: () => ({ ok: false, reason: "Open a project first" }),
    steps: [{ id: "share", title: "Share", body: "Share the project." }],
  },
];

const TOPICS: HelpTopic[] = [
  {
    id: "checks",
    title: "Checks",
    summary: "What a check reports and how to act on it",
    body: [
      "A check reads your files against the project's rules.",
      "",
      "## Findings",
      "",
      "- **Fails** stops a release.",
      "- **Reports** only lists the finding.",
      "",
      "Press **Fix** on a finding to apply the suggested change.",
    ].join("\n"),
    guides: ["check-a-file"],
    moreUrl: "https://neokapi.com/docs/kapi/checks",
  },
  {
    id: "terms",
    title: "Terms",
    summary: "The approved words for your project",
    body: "Terms name the words to use and the words to avoid.",
  },
];

function MockPage() {
  const guide = useGuide();
  return (
    <div className="flex h-[36rem] bg-background text-foreground">
      <aside data-slot="demo-files" className="flex w-56 flex-col gap-1 border-r p-3 text-sm">
        <p className="px-2 pb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
          Files
        </p>
        {["README.md", "docs/intro.md", "docs/setup.md"].map((f) => (
          <div key={f} className="flex items-center gap-2 rounded-md px-2 py-1.5 hover:bg-muted">
            <FileTextIcon className="size-4 text-muted-foreground" />
            {f}
          </div>
        ))}
      </aside>
      <main className="flex flex-1 flex-col gap-6 p-6">
        <div className="flex items-center gap-2">
          <h1 className="flex-1 text-xl font-semibold">docs/intro.md</h1>
          <Button data-slot="demo-check">Check</Button>
          <Button variant="outline" onClick={() => guide.start("check-a-file")}>
            <PlayIcon data-icon="inline-start" />
            Start tour
          </Button>
          <Button variant="ghost" size="icon" aria-label="Help" onClick={() => guide.openHelp()}>
            <HelpCircleIcon />
          </Button>
        </div>
        <section
          data-slot="demo-findings"
          className="mt-auto flex flex-col gap-2 rounded-lg border p-4 text-sm"
        >
          <p className="font-medium">2 findings</p>
          <p className="text-muted-foreground">terms.vocabulary · line 4 · "utilize"</p>
          <p className="text-muted-foreground">voice.pattern · line 9 · "simply"</p>
        </section>
      </main>
    </div>
  );
}

function OnMount({ run }: { run: (api: ReturnType<typeof useGuide>) => void }) {
  const api = useGuide();
  const first = React.useRef({ api, run });
  React.useEffect(() => {
    const { api: initial, run: once } = first.current;
    const id = setTimeout(() => once(initial), 50);
    return () => clearTimeout(id);
  }, []);
  return null;
}

function Frame({
  dark,
  run,
}: {
  dark?: boolean;
  run?: (api: ReturnType<typeof useGuide>) => void;
}) {
  const content = (
    <GuideProvider
      guides={GUIDES}
      topics={TOPICS}
      onOpenUrl={fn()}
      storageKey={`storybook.guide.${dark ? "dark" : "light"}`}
    >
      <MockPage />
      {run && <OnMount run={run} />}
    </GuideProvider>
  );
  return (
    <div className={dark ? "dark" : undefined}>
      {dark ? <PortalThemeProvider className="dark">{content}</PortalThemeProvider> : content}
    </div>
  );
}

/** The first step of a three-step tour, started on load. */
export const Tour: Story = {
  render: () => <Frame run={(api) => api.start("check-a-file")} />,
};

export const TourDark: Story = {
  name: "Tour, dark",
  render: () => <Frame dark run={(api) => api.start("check-a-file")} />,
};

/** The page with a Start tour button and nothing running. */
export const Idle: Story = {
  render: () => <Frame />,
};

/** The Help panel on its list of guides and topics. */
export const HelpList: Story = {
  name: "Help panel",
  render: () => <Frame run={(api) => api.openHelp()} />,
};

export const HelpListDark: Story = {
  name: "Help panel, dark",
  render: () => <Frame dark run={(api) => api.openHelp()} />,
};

/** The Help panel on one topic, with its guide and the website link. */
export const HelpTopicView: Story = {
  name: "Help topic",
  render: () => <Frame run={(api) => api.openHelp("checks")} />,
};

export const HelpTopicDark: Story = {
  name: "Help topic, dark",
  render: () => <Frame dark run={(api) => api.openHelp("checks")} />,
};

import type { ReviewContainer, StoryIndex } from "@neokapi/bowrain-client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react-native";
import type { ReactNode } from "react";

import "../lib/i18n";
import { InContextView } from "../components/InContextView";
import { hasLivePreview, LivePreview, previewScript } from "../components/LivePreview";

const unit = (blockId: string, role: string, src: string, tgt: string) => ({
  blockId,
  item: "OfflineLaunch.kbf.json",
  key: blockId,
  role,
  source: [{ text: src }],
  target: [{ text: tgt }],
  waiting: true,
  sourceRevision: `s:${blockId}`,
  targetRevision: `r:${blockId}`,
  messageKey: `h-${blockId}`,
  runtimeText: tgt,
});

const container: ReviewContainer = {
  kind: "component",
  title: "OfflineLaunch",
  item: "OfflineLaunch.kbf.json",
  locale: "nb",
  focus: "o2",
  moreBefore: false,
  moreAfter: false,
  collectionId: "c1",
  preview: {
    kind: "storybook",
    url: "https://example.org/sb/",
    sourceFile: "src/OfflineLaunch.tsx",
  },
  units: [
    unit("o1", "heading", "Can't reach your server", "Får ikke kontakt"),
    unit("o2", "button", "Try again", "Prøv igjen"),
  ],
};

const index: StoryIndex = {
  entries: {
    a: {
      id: "offline--default",
      title: "App/OfflineLaunch",
      name: "Default",
      importPath: "./src/OfflineLaunch.stories.tsx",
    },
  },
};

function wrap(children: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

describe("LivePreview", () => {
  it("loads the component's story from its collection's Storybook", async () => {
    const loadIndex = jest.fn(async () => index);
    await render(
      wrap(
        <LivePreview
          container={container}
          scope={{ workspace: "w", project: "p" }}
          loadIndex={loadIndex}
          source={false}
        />,
      ),
    );
    const frame = await screen.findByTestId("live-frame", {}, { timeout: 5000 });
    expect(frame.props.accessibilityLabel).toBe(
      JSON.stringify("https://example.org/sb/iframe.html?id=offline--default&viewMode=story"),
    );
    expect(loadIndex).toHaveBeenCalledWith("c1");
    expect(screen.getByText("OfflineLaunch · Default")).toBeTruthy();
  });

  it("says so when no story renders the component", async () => {
    await render(
      wrap(
        <LivePreview
          container={container}
          scope={{ workspace: "w", project: "p" }}
          loadIndex={async () => ({ entries: {} })}
          source={false}
        />,
      ),
    );
    expect(await screen.findByTestId("live-no-story")).toBeTruthy();
  });

  it("answers the story's ready message with the dictionary", () => {
    const script = previewScript("nb", { "h-o1": "Får ikke kontakt" });
    expect(script).toContain('"type":"neokapi-i18n-translations"');
    expect(script).toContain('"h-o1":"Får ikke kontakt"');
    expect(script).toContain('"neokapi-i18n-ready"');
  });

  it("runs only components whose collection declares a Storybook", () => {
    expect(hasLivePreview(container)).toBe(true);
    expect(hasLivePreview({ ...container, kind: "section" })).toBe(false);
    expect(hasLivePreview({ ...container, preview: undefined })).toBe(false);
  });
});

describe("InContextView live", () => {
  it("shows the component running above its strings, and hides it on request", async () => {
    await render(
      wrap(
        <InContextView
          card={{
            key: "o2:nb",
            blockId: "o2",
            item: "OfflineLaunch.kbf.json",
            locale: "nb",
            collectionId: "c1",
            source: [],
            target: [],
            targetRevision: "",
            group: "unsorted",
            notes: [],
            findingsCount: 0,
            failingFindingsCount: 0,
            ownWork: false,
          }}
          scope={{ workspace: "w", project: "p" }}
          load={async () => container}
          loadIndex={async () => index}
          onApprove={() => {}}
          onFix={() => {}}
        />,
      ),
    );
    expect(await screen.findByTestId("live-frame", {}, { timeout: 5000 })).toBeTruthy();
    await fireEvent.press(screen.getByTestId("context-live"));
    expect(screen.queryByTestId("live-frame")).toBeNull();
  });
});

import type { ReviewContainer, StoryIndex } from "@neokapi/bowrain-client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen } from "@testing-library/react-native";
import type { ReactNode } from "react";

import "../lib/i18n";
import { InContextView } from "../components/InContextView";
import {
  hasLivePreview,
  highlightScript,
  LiveView,
  previewScript,
  previewTargets,
  readFound,
} from "../components/LivePreview";

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
    e: {
      id: "offline--error",
      title: "App/OfflineLaunch",
      name: "Load error",
      importPath: "./src/OfflineLaunch.stories.tsx",
    },
    a: {
      id: "offline--default",
      title: "App/OfflineLaunch",
      name: "Default",
      importPath: "./src/OfflineLaunch.stories.tsx",
    },
  },
};

const frameURL = (id: string) =>
  JSON.stringify(`https://example.org/sb/iframe.html?id=${id}&viewMode=story`);

function wrap(children: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

function live(over: Partial<React.ComponentProps<typeof LiveView>> = {}) {
  return wrap(
    <LiveView
      container={container}
      scope={{ workspace: "w", project: "p" }}
      loadIndex={async () => index}
      start="o1"
      isDone={() => false}
      onApprove={() => {}}
      onFix={() => {}}
      onClose={() => {}}
      {...over}
    />,
  );
}

const say = (found: string[]) => ({
  nativeEvent: { data: JSON.stringify({ type: "neokapi-preview-found", found }) },
});

describe("LiveView", () => {
  it("opens on the story most likely to show the strings, error states last", async () => {
    const loadIndex = jest.fn(async () => index);
    await render(live({ loadIndex }));
    const frame = await screen.findByTestId("live-frame", {}, { timeout: 5000 });
    expect(frame.props.accessibilityLabel).toBe(frameURL("offline--default"));
    expect(loadIndex).toHaveBeenCalledWith("c1");
    expect(screen.getByText("OfflineLaunch · Default")).toBeTruthy();
  });

  it("says how many strings the story shows, and steps to the next story", async () => {
    await render(live());
    const frame = await screen.findByTestId("live-frame", {}, { timeout: 5000 });
    await act(async () => frame.props.onMessage(say(["o1"])));
    expect(screen.getByText(/Shows 1 of 2 strings/)).toBeTruthy();
    await fireEvent.press(screen.getByTestId("live-story-next"));
    expect(screen.getByTestId("live-frame").props.accessibilityLabel).toBe(
      frameURL("offline--error"),
    );
  });

  it("steps through the strings and approves in place", async () => {
    const onApprove = jest.fn();
    await render(live({ onApprove }));
    await screen.findByTestId("live-frame", {}, { timeout: 5000 });
    expect(screen.getByText("Får ikke kontakt")).toBeTruthy();
    await fireEvent.press(screen.getByTestId("live-approve"));
    expect(onApprove).toHaveBeenCalledWith(expect.objectContaining({ blockId: "o1" }));
    expect(screen.getByText("Prøv igjen")).toBeTruthy();
    await fireEvent.press(screen.getByTestId("live-string-prev"));
    expect(screen.getByText("Får ikke kontakt")).toBeTruthy();
  });

  it("says so when no story renders the component", async () => {
    await render(live({ loadIndex: async () => ({ entries: {} }) }));
    expect(await screen.findByTestId("live-no-story")).toBeTruthy();
  });

  it("answers the story's ready message with the dictionary", () => {
    const script = previewScript("nb", { "h-o1": "Får ikke kontakt" });
    expect(script).toContain('"type":"neokapi-i18n-translations"');
    expect(script).toContain('"h-o1":"Får ikke kontakt"');
    expect(script).toContain('"neokapi-i18n-ready"');
  });

  it("looks for each string by the text a reader sees, in the reading shown", () => {
    expect(previewTargets(container.units, false)).toEqual([
      { key: "o1", fragments: ["Får ikke kontakt"] },
      { key: "o2", fragments: ["Prøv igjen"] },
    ]);
    expect(previewTargets(container.units, true)[0].fragments).toEqual(["Can't reach your server"]);
    const script = highlightScript(previewTargets(container.units, false), "o2");
    expect(script).toContain('"current":"o2"');
    expect(script).toContain("Prøv igjen");
  });

  it("takes only the strings it asked for from the page", () => {
    const keys = new Set(["o1", "o2"]);
    expect(
      readFound(JSON.stringify({ type: "neokapi-preview-found", found: ["o1", "x", 3] }), keys),
    ).toEqual(["o1"]);
    expect(readFound(JSON.stringify({ type: "other", found: ["o1"] }), keys)).toBeNull();
    expect(readFound("not json", keys)).toBeNull();
  });

  it("runs only components whose collection declares a Storybook", () => {
    expect(hasLivePreview(container)).toBe(true);
    expect(hasLivePreview({ ...container, kind: "section" })).toBe(false);
    expect(hasLivePreview({ ...container, preview: undefined })).toBe(false);
  });
});

describe("InContextView live", () => {
  it("opens the component running, full page, and closes it", async () => {
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
    await fireEvent.press(await screen.findByTestId("context-live", {}, { timeout: 5000 }));
    expect(await screen.findByTestId("live-frame", {}, { timeout: 5000 })).toBeTruthy();
    await fireEvent.press(screen.getByTestId("live-close"));
    expect(screen.queryByTestId("live-frame")).toBeNull();
  });
});

import type { DeckPages } from "@neokapi/bowrain-client";
import { fireEvent, render, screen } from "@testing-library/react-native";

import "../lib/i18n";
import { PagesList } from "../components/PagesList";
import { opensOnPages, pageCard } from "../lib/pages";

const pages: DeckPages = {
  locale: "nb",
  waiting: 5,
  more: false,
  pages: [
    {
      kind: "component",
      title: "OfflineLaunch",
      item: "src/OfflineLaunch.kbf.json",
      collectionId: "c1",
      component: "OfflineLaunch",
      waiting: 3,
      findings: 0,
      firstBlockId: "o1",
    },
    {
      kind: "document",
      title: "guide.md",
      item: "docs/guide.md",
      collectionId: "c2",
      waiting: 2,
      findings: 0,
      firstBlockId: "p1",
    },
  ],
};

describe("PagesList", () => {
  it("lists each component and document with its waiting count, and opens one", async () => {
    const onOpen = jest.fn();
    await render(<PagesList pages={pages} onOpen={onOpen} />);
    expect(screen.getByTestId("page-o1")).toHaveTextContent(/OfflineLaunch/);
    expect(screen.getByTestId("page-o1")).toHaveTextContent(/3 waiting/);
    expect(screen.getByTestId("page-p1")).toHaveTextContent(/guide\.md/);
    expect(screen.getByLabelText("Document guide.md, 2 translations waiting")).toBeTruthy();
    await fireEvent.press(screen.getByTestId("page-p1"));
    expect(onOpen).toHaveBeenCalledWith(pages.pages[1]);
  });

  it("says so when nothing waits", async () => {
    await render(<PagesList pages={{ ...pages, pages: [], waiting: 0 }} onOpen={() => {}} />);
    expect(screen.getByTestId("pages-empty")).toBeTruthy();
  });
});

describe("pages", () => {
  it("opens on pages only when the server has the route and pages to show", () => {
    expect(opensOnPages(pages)).toBe(true);
    expect(opensOnPages({ pages: [] })).toBe(false);
    expect(opensOnPages(null)).toBe(false);
  });

  it("opens a container with a card for the block and the language", () => {
    expect(pageCard("p1", "nb", "c2")).toMatchObject({
      key: "p1:nb",
      blockId: "p1",
      locale: "nb",
      collectionId: "c2",
    });
  });
});

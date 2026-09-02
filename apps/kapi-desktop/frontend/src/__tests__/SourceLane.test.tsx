// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SourceLane } from "../components/SourceLane";
import { ErrorProvider } from "../components/ErrorBanner";
import type { SourceQueueItem } from "../types/api";

const items: SourceQueueItem[] = [
  {
    file: "src/en.json",
    relative: "src/en.json",
    key: "greeting",
    collection: "App",
    sourceLocale: "en",
    source: "Hello world",
    status: "checked",
    held: true,
    approved: false,
  },
  {
    file: "src/en.json",
    relative: "src/en.json",
    key: "farewell",
    sourceLocale: "en",
    source: "Goodbye now",
    status: "checked",
    held: true,
    approved: false,
  },
];

function renderLane(props: Partial<React.ComponentProps<typeof SourceLane>> = {}) {
  return render(
    <ErrorProvider>
      <SourceLane tabID="t1" filter={null} items={items} {...props} />
    </ErrorProvider>,
  );
}

beforeEach(() => {
  vi.restoreAllMocks();
});

describe("SourceLane", () => {
  it("lists the units holding every language", async () => {
    renderLane();
    expect(await screen.findByText("greeting")).toBeTruthy();
    expect(screen.getByText("farewell")).toBeTruthy();
    expect(document.querySelector("[data-slot='source-lane']")).not.toBeNull();
  });

  it("approves the selected unit", async () => {
    const onApprove = vi.fn(async () => {});
    renderLane({ onApprove });
    await userEvent.click(await screen.findByRole("button", { name: /Approve source/ }));
    await waitFor(() => expect(onApprove).toHaveBeenCalledTimes(1));
    expect(onApprove.mock.calls[0][0].key).toBe("greeting");
  });

  // The translations stay where they are and the loop supersedes them. Naming
  // the languages is how the reviewer knows the re-draft is coming.
  it("saves an edit and names the languages awaiting a re-draft", async () => {
    const onSaveSource = vi.fn(async () => ["de", "fr"]);
    renderLane({ onSaveSource });

    const editor = (await screen.findByRole("textbox")) as HTMLTextAreaElement;
    await userEvent.clear(editor);
    await userEvent.type(editor, "Hi there");
    await userEvent.click(screen.getByRole("button", { name: /Save and re-draft/ }));

    await waitFor(() => expect(onSaveSource).toHaveBeenCalledTimes(1));
    expect(onSaveSource.mock.calls[0][1]).toBe("Hi there");
    await waitFor(() =>
      expect(document.querySelector("[data-slot='source-lane-awaiting']")?.textContent).toContain(
        "de, fr",
      ),
    );
  });

  it("will not save an unchanged source", async () => {
    const onSaveSource = vi.fn(async () => []);
    renderLane({ onSaveSource });
    const save = await screen.findByRole("button", { name: /Save and re-draft/ });
    expect(save.hasAttribute("disabled")).toBe(true);
    expect(onSaveSource).not.toHaveBeenCalled();
  });

  it("says the source is settled when nothing is queued", async () => {
    renderLane({ items: [] });
    expect(await screen.findByText(/The source is settled/)).toBeTruthy();
  });

  // Both lanes render one model. Approving source wording without seeing the
  // voice it is approved against is the target defect in reverse.
  it("renders the point and the neighbourhood for the selected source unit", async () => {
    const loadContext = vi.fn(async (item: SourceQueueItem) => ({
      point: {
        collection: "App",
        ref: "retail/web",
        default: false,
        voice: { name: "Kapimart retail" },
        term_rules: [{ term: "cart", replacement: "basket", severity: "major" }],
        terms_total: 1,
      },
      neighbourhood: {
        key: item.key,
        after: [{ key: "farewell", source: [{ text: "Goodbye now" }] }],
        window: 2,
      },
      history: {},
      judgement: {},
      provenance: {},
    }));
    renderLane({ loadContext });
    await waitFor(() =>
      expect(document.querySelector("[data-slot='review-point-voice']")?.textContent).toBe(
        "Kapimart retail",
      ),
    );
    expect(loadContext.mock.calls[0][0].key).toBe("greeting");
    expect(document.querySelector("[data-slot='review-point-term']")?.textContent).toContain(
      "basket",
    );
    expect(document.querySelector("[data-slot='review-neighbour']")?.textContent).toContain(
      "Goodbye now",
    );
  });
});

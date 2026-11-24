import { afterEach, describe, expect, it, vi } from "vite-plus/test";
import userEvent from "@testing-library/user-event";
import { render, screen, waitFor, within } from "./testUtils";
import { api } from "../hooks/useApi";
import { formatSize, useCheckoutStateNotices } from "../components/CheckoutStateDialog";
import type { CheckoutStateNotice } from "../types/api";

afterEach(() => {
  vi.restoreAllMocks();
});

const IMPORT: CheckoutStateNotice = {
  name: "state-in-tree",
  message: "kapi: .kapi/vault in this checkout hold state kapi no longer reads",
  transfer: {
    action: "import",
    from: "/p/.kapi",
    to: "/data/checkouts/k",
    files: [
      { source: "vault/redaction.json", target: "vault/redaction.json", size: 1500 },
      { source: "sync/sync-cache.json", target: "sync/sync-cache.json", size: 1572 },
    ],
    servers: ["https://app.example"],
    sync_server: "https://app.example",
    sync_project: "p1",
    taken: false,
    source_removed: false,
  },
  digest: "d1",
};

const WORK: CheckoutStateNotice = {
  name: "work-in-tree",
  message: "kapi: .kapi/work in this checkout is no longer used",
};

const RELINK: CheckoutStateNotice = {
  name: "earlier-at-path",
  message: "kapi: state recorded for an earlier checkout at /p belongs to another directory",
  transfer: {
    action: "relink",
    from: "/data/checkouts/old",
    to: "/data/checkouts/new",
    files: [{ source: "filters.json", target: "filters.json", size: 40 }],
    withheld: [{ source: "vault/redaction.json", target: "vault/redaction.json", size: 25 }],
    unverified: true,
    project: '"app"',
    taken: false,
    source_removed: false,
  },
  digest: "r1",
};

function Harness({ tabID, enabled = true }: { tabID: string | null; enabled?: boolean }) {
  const { dialog } = useCheckoutStateNotices(tabID, "compass", enabled);
  return <>{dialog}</>;
}

const slot = (name: string, root: HTMLElement) =>
  root.querySelector(`[data-slot="${name}"]`) as HTMLElement | null;

describe("telling a person about state an earlier kapi kept", () => {
  it("formats sizes for a person", () => {
    expect(formatSize(83)).toBe("83 B");
    expect(formatSize(3072)).toBe("3 KB");
    expect(formatSize(1536)).toBe("1.5 KB");
  });

  it("opens nothing when the checkout owes no line", async () => {
    const pending = vi.spyOn(api, "pendingCheckoutState").mockResolvedValue([]);
    render(<Harness tabID="t1" />);
    await waitFor(() => expect(pending).toHaveBeenCalledWith("t1"));
    expect(screen.queryByTestId("checkout-state")).toBeNull();
  });

  it("asks nothing for a tab with no checkout", async () => {
    const pending = vi.spyOn(api, "pendingCheckoutState").mockResolvedValue([IMPORT]);
    render(<Harness tabID="t1" enabled={false} />);
    await new Promise((r) => setTimeout(r, 0));
    expect(pending).not.toHaveBeenCalled();
  });

  it("says what the state is, counts it, takes it as shown, and records the lines on close", async () => {
    vi.spyOn(api, "pendingCheckoutState").mockResolvedValue([WORK, IMPORT]);
    const apply = vi.spyOn(api, "applyCheckoutStateTransfer").mockResolvedValue({
      ...IMPORT.transfer!,
      taken: true,
      source_removed: true,
    });
    const shown = vi.spyOn(api, "markCheckoutStateShown").mockResolvedValue(undefined);
    render(<Harness tabID="t1" />);

    const dialog = await screen.findByTestId("checkout-state");
    expect(slot("checkout-state-project", dialog)?.textContent).toBe("compass");
    expect(slot("checkout-state-work-in-tree", dialog)?.textContent).toContain(
      ".kapi/work is no longer used",
    );
    const row = slot("checkout-state-state-in-tree", dialog)!;
    const contents = slot("checkout-state-contents", row)!.textContent;
    expect(contents).toContain("The original text of withheld values");
    expect(contents).toContain("The saved sign-in for https://app.example, project p1");
    expect(slot("checkout-state-count", row)?.textContent).toBe("2 files");
    expect(slot("checkout-state-total", row)?.textContent).toBe(formatSize(3072));
    expect(slot("checkout-state-files", row)?.textContent).toContain("vault/redaction.json");
    expect(within(row).getByRole("checkbox").closest("label")?.textContent).toContain(
      "They are deleted from .kapi once kapi has its own copy.",
    );
    expect(shown).not.toHaveBeenCalled();

    await userEvent.click(within(row).getByRole("checkbox"));
    await userEvent.click(
      within(row).getByRole("button", { name: "Bring in the state from this folder" }),
    );
    expect(apply).toHaveBeenCalledWith("t1", "state-in-tree", "d1", true);
    await waitFor(() =>
      expect(slot("checkout-state-taken", row)?.textContent).toBe("This state is now in use here."),
    );

    await userEvent.click(within(dialog).getAllByRole("button", { name: "Close" })[0]);
    expect(shown).toHaveBeenCalledWith("t1", ["work-in-tree", "state-in-tree"]);
    await waitFor(() => expect(screen.queryByTestId("checkout-state")).toBeNull());
  });

  it("reports the import as done and lists the copies that could not be removed", async () => {
    vi.spyOn(api, "pendingCheckoutState").mockResolvedValue([IMPORT]);
    vi.spyOn(api, "applyCheckoutStateTransfer").mockResolvedValue({
      ...IMPORT.transfer!,
      taken: true,
      source_removed: false,
      remaining: ["sync/sync-cache.json"],
    });
    render(<Harness tabID="t1" />);
    const dialog = await screen.findByTestId("checkout-state");
    const row = slot("checkout-state-state-in-tree", dialog)!;
    await userEvent.click(within(row).getByRole("checkbox"));
    await userEvent.click(
      within(row).getByRole("button", { name: "Bring in the state from this folder" }),
    );
    await waitFor(() => expect(slot("checkout-state-taken", row)).not.toBeNull());
    expect(slot("checkout-state-taken", row)?.textContent).toContain(
      "This state is now in use here.",
    );
    expect(slot("checkout-state-remaining", row)?.textContent).toContain("sync/sync-cache.json");
    expect(slot("checkout-state-error", row)).toBeNull();
  });

  it("offers an unverified relink without the withheld text and sign-in", async () => {
    vi.spyOn(api, "pendingCheckoutState").mockResolvedValue([RELINK]);
    const apply = vi.spyOn(api, "applyCheckoutStateTransfer").mockResolvedValue({
      ...RELINK.transfer!,
      taken: true,
    });
    render(<Harness tabID="t1" />);
    const dialog = await screen.findByTestId("checkout-state");
    const row = slot("checkout-state-earlier-at-path", dialog)!;
    expect(row.textContent).toContain("Kept for an earlier checkout at this location");
    expect(slot("checkout-state-count", row)?.textContent).toBe("1 file");
    expect(slot("checkout-state-withheld", row)?.textContent).toContain(
      "withheld text and the saved sign-in stay where they are",
    );
    expect(within(row).queryByRole("checkbox")).toBeNull();
    await userEvent.click(within(row).getByRole("button", { name: "Use this state here" }));
    expect(apply).toHaveBeenCalledWith("t1", "earlier-at-path", "r1", false);
  });

  it("leaves the sign-in of another server project and says which project it belongs to", async () => {
    const sameIdOtherServer: CheckoutStateNotice = {
      ...RELINK,
      transfer: {
        ...RELINK.transfer!,
        unverified: false,
        files: [{ source: "vault/redaction.json", target: "vault/redaction.json", size: 25 }],
        withheld: [{ source: "sync/sync-cache.json", target: "sync/sync-cache.json", size: 70 }],
        sync_server: "https://app.example",
        sync_project: "p1",
      },
    };
    vi.spyOn(api, "pendingCheckoutState").mockResolvedValue([sameIdOtherServer]);
    render(<Harness tabID="t1" />);
    const dialog = await screen.findByTestId("checkout-state");
    const row = slot("checkout-state-earlier-at-path", dialog)!;
    expect(slot("checkout-state-contents", row)?.textContent).toContain(
      "The saved sign-in for https://app.example, project p1",
    );
    expect(slot("checkout-state-withheld", row)?.textContent).toBe(
      "The saved sign-in stays where it is, because this project does not connect to the server project it belongs to.",
    );
    expect(slot("checkout-state-files", row)?.textContent).toContain("vault/redaction.json");
    expect(slot("checkout-state-files", row)?.textContent).not.toContain("sync-cache.json");
  });

  it("names another project's state and offers nothing to take", async () => {
    vi.spyOn(api, "pendingCheckoutState").mockResolvedValue([
      { name: "earlier-at-path", message: "…", other_project: '"billing"' },
    ]);
    render(<Harness tabID="t1" />);
    const dialog = await screen.findByTestId("checkout-state");
    const row = slot("checkout-state-earlier-at-path", dialog)!;
    expect(row.textContent).toContain("Kept for another project at this location");
    expect(slot("checkout-state-other-project", row)?.textContent).toBe(
      'It belongs to "billing", so kapi keeps it where it is.',
    );
    expect(within(row).queryByRole("button")).toBeNull();
  });

  it("shows the plan again when it changed after it was shown", async () => {
    const changed: CheckoutStateNotice = {
      ...IMPORT,
      digest: "d2",
      transfer: {
        ...IMPORT.transfer!,
        files: [{ source: "vault/added.json", target: "vault/added.json", size: 2 }],
      },
    };
    vi.spyOn(api, "pendingCheckoutState")
      .mockResolvedValueOnce([IMPORT])
      .mockResolvedValueOnce([changed]);
    const apply = vi
      .spyOn(api, "applyCheckoutStateTransfer")
      .mockRejectedValueOnce(new Error("the state changed after it was shown"));
    render(<Harness tabID="t1" />);

    const dialog = await screen.findByTestId("checkout-state");
    const row = slot("checkout-state-state-in-tree", dialog)!;
    await userEvent.click(
      within(row).getByRole("button", { name: "Bring in the state from this folder" }),
    );
    expect(apply).toHaveBeenCalledWith("t1", "state-in-tree", "d1", false);
    await waitFor(() =>
      expect(slot("checkout-state-state-in-tree", dialog)?.textContent).toContain(
        "vault/added.json",
      ),
    );
  });
});

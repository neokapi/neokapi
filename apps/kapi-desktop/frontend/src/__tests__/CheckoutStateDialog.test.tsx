import { afterEach, describe, expect, it, vi } from "vite-plus/test";
import userEvent from "@testing-library/user-event";
import { render, screen, waitFor, within } from "./testUtils";
import { api } from "../hooks/useApi";
import { useCheckoutStateNotices } from "../components/CheckoutStateDialog";
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
      { source: "vault/redaction.json", target: "vault/redaction.json", size: 25 },
      { source: "sync/sync-cache.json", target: "sync/sync-cache.json", size: 60 },
    ],
    servers: ["https://app.example"],
    taken: false,
    source_removed: false,
  },
  digest: "d1",
};

const WORK: CheckoutStateNotice = {
  name: "work-in-tree",
  message: "kapi: .kapi/work in this checkout is no longer used",
};

function Harness({ tabID, enabled = true }: { tabID: string | null; enabled?: boolean }) {
  const { dialog } = useCheckoutStateNotices(tabID, "compass", enabled);
  return <>{dialog}</>;
}

const slot = (name: string, root: HTMLElement) =>
  root.querySelector(`[data-slot="${name}"]`) as HTMLElement | null;

describe("telling a person about state an earlier kapi left", () => {
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

  it("lists what an import takes, takes it as shown, and records the lines on close", async () => {
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
    expect(row.textContent).toContain("vault/redaction.json");
    expect(row.textContent).toContain("sync/sync-cache.json");
    expect(slot("checkout-state-servers", row)?.textContent).toBe(
      "Sync state issued by: https://app.example",
    );
    expect(shown).not.toHaveBeenCalled();

    await userEvent.click(within(row).getByRole("checkbox"));
    await userEvent.click(within(row).getByRole("button", { name: "Import" }));
    expect(apply).toHaveBeenCalledWith("t1", "state-in-tree", "d1", true);
    await waitFor(() => expect(slot("checkout-state-taken", row)?.textContent).toBe("Imported."));

    await userEvent.click(within(dialog).getAllByRole("button", { name: "Close" })[0]);
    expect(shown).toHaveBeenCalledWith("t1", ["work-in-tree", "state-in-tree"]);
    await waitFor(() => expect(screen.queryByTestId("checkout-state")).toBeNull());
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
    await userEvent.click(within(row).getByRole("button", { name: "Import" }));
    expect(apply).toHaveBeenCalledWith("t1", "state-in-tree", "d1", false);
    await waitFor(() =>
      expect(slot("checkout-state-state-in-tree", dialog)?.textContent).toContain(
        "vault/added.json",
      ),
    );
  });
});

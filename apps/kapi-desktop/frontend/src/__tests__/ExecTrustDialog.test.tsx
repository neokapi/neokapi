import { afterEach, describe, expect, it, vi } from "vite-plus/test";
import userEvent from "@testing-library/user-event";
import { render, screen, waitFor, within } from "./testUtils";
import { api } from "../hooks/useApi";
import { useExecTrustGate } from "../components/ExecTrustDialog";
import type { ExecTrustRequest } from "../types/api";

afterEach(() => {
  vi.restoreAllMocks();
});

const REQUEST: ExecTrustRequest = {
  recipe: "/p/kapi.yaml",
  sites: [
    {
      where: "flows.default.steps[0]",
      kind: "tool",
      name: "external-command",
      detail: "command=./tools/run.sh",
    },
  ],
  checked: ["tools/run.sh"],
  not_checked: ["lib"],
  inline: ["flows.default.steps[1] external-command (node -e)"],
  changed: ["tools/run.sh"],
  limit:
    "kapi checks the content of the files listed, not what those files or the commands run or read.",
  digest: "d1",
};

function Harness({ onResult }: { onResult: (ok: boolean) => void }) {
  const { gate, dialog } = useExecTrustGate("t1");
  return (
    <>
      <button onClick={() => void gate().then(onResult)}>Run</button>
      {dialog}
    </>
  );
}

const slot = (name: string, root: HTMLElement) =>
  root.querySelector(`[data-slot="${name}"]`)?.textContent;

describe("asking about a project's commands before a run", () => {
  it("runs at once when nothing needs answering", async () => {
    vi.spyOn(api, "pendingExecTrust").mockResolvedValue(null);
    const onResult = vi.fn();
    render(<Harness onResult={onResult} />);
    await userEvent.click(screen.getByRole("button", { name: "Run" }));
    await waitFor(() => expect(onResult).toHaveBeenCalledWith(true));
    expect(screen.queryByTestId("exec-trust")).toBeNull();
  });

  it("shows what is checked, what is not, the shell and the limit, and records the shown digest", async () => {
    vi.spyOn(api, "pendingExecTrust").mockResolvedValue(REQUEST);
    const approve = vi.spyOn(api, "approveExecTrust").mockResolvedValue(undefined);
    const onResult = vi.fn();
    render(<Harness onResult={onResult} />);
    await userEvent.click(screen.getByRole("button", { name: "Run" }));

    const dialog = await screen.findByTestId("exec-trust");
    expect(slot("exec-trust-checked", dialog)).toBe("Checked, by content: tools/run.sh");
    expect(slot("exec-trust-not-checked", dialog)).toBe("Named and not checked (directories): lib");
    expect(slot("exec-trust-inline", dialog)).toBe(
      "Runs code given inline, so kapi cannot see which files it uses: flows.default.steps[1] external-command (node -e)",
    );
    expect(slot("exec-trust-changed", dialog)).toBe("Changed since it was answered: tools/run.sh");
    expect(slot("exec-trust-limit", dialog)).toBe(REQUEST.limit);

    await userEvent.click(within(dialog).getByRole("button", { name: "Allow and run" }));
    expect(approve).toHaveBeenCalledWith("t1", "d1");
    await waitFor(() => expect(onResult).toHaveBeenCalledWith(true));
  });

  it("asks again when the project changed between showing and allowing", async () => {
    const changed = { ...REQUEST, digest: "d2", changed: ["tools/other.sh"] };
    vi.spyOn(api, "pendingExecTrust").mockResolvedValueOnce(REQUEST).mockResolvedValueOnce(changed);
    vi.spyOn(api, "approveExecTrust")
      .mockRejectedValueOnce(new Error("changed"))
      .mockResolvedValueOnce(undefined);
    const onResult = vi.fn();
    render(<Harness onResult={onResult} />);
    await userEvent.click(screen.getByRole("button", { name: "Run" }));
    const dialog = await screen.findByTestId("exec-trust");
    await userEvent.click(within(dialog).getByRole("button", { name: "Allow and run" }));

    await waitFor(() =>
      expect(slot("exec-trust-changed", screen.getByTestId("exec-trust"))).toBe(
        "Changed since it was answered: tools/other.sh",
      ),
    );
    expect(onResult).not.toHaveBeenCalled();
  });

  it("starts nothing when the person cancels", async () => {
    vi.spyOn(api, "pendingExecTrust").mockResolvedValue(REQUEST);
    const approve = vi.spyOn(api, "approveExecTrust");
    const onResult = vi.fn();
    render(<Harness onResult={onResult} />);
    await userEvent.click(screen.getByRole("button", { name: "Run" }));
    const dialog = await screen.findByTestId("exec-trust");
    await userEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(onResult).toHaveBeenCalledWith(false));
    expect(approve).not.toHaveBeenCalled();
  });
});

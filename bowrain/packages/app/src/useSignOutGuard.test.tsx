import { afterEach, beforeEach, describe, it, expect, vi } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useSignOutGuard } from "./useSignOutGuard";
import {
  discardAllPendingSaves,
  hasPendingSaves,
  recordPendingEdit,
  setPendingSavesSession,
} from "./pendingSaves";

beforeEach(() => setPendingSavesSession({ userId: "u1", workspace: "acme" }));
afterEach(() => discardAllPendingSaves());

function SignOut({ signOut }: { signOut: () => Promise<void> }) {
  const { request, dialog } = useSignOutGuard(signOut);
  return (
    <>
      <button onClick={request}>Sign out now</button>
      {dialog}
    </>
  );
}

describe("useSignOutGuard", () => {
  it("signs out at once when nothing waits", async () => {
    const signOut = vi.fn().mockResolvedValue(undefined);
    render(<SignOut signOut={signOut} />);
    await userEvent.click(screen.getByText("Sign out now"));
    expect(signOut).toHaveBeenCalledTimes(1);
  });

  it("asks first when an edit waits, and Cancel keeps both", async () => {
    recordPendingEdit({ key: "k", label: "Translate and review", scope: "s", send: async () => {} });
    const signOut = vi.fn().mockResolvedValue(undefined);
    render(<SignOut signOut={signOut} />);
    await userEvent.click(screen.getByText("Sign out now"));
    expect(await screen.findByText("Sign out and discard unsaved changes?")).toBeInTheDocument();
    expect(signOut).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(signOut).not.toHaveBeenCalled();
    expect(hasPendingSaves()).toBe(true);
  });

  it("signing out after the question discards the edits", async () => {
    recordPendingEdit({ key: "k", label: "Translate and review", scope: "s", send: async () => {} });
    const signOut = vi.fn().mockResolvedValue(undefined);
    render(<SignOut signOut={signOut} />);
    await userEvent.click(screen.getByText("Sign out now"));
    await userEvent.click(await screen.findByRole("button", { name: "Sign out" }));
    expect(signOut).toHaveBeenCalledTimes(1);
    expect(hasPendingSaves()).toBe(false);
  });
});

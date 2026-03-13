import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { AccountMenu } from "../components/AccountMenu";
import type { User } from "../types/api";

const alice: User = { id: "1", name: "Alice", email: "alice@example.com", avatar_url: "" };

describe("AccountMenu", () => {
  it("renders the user name in the trigger button", () => {
    render(<AccountMenu user={alice} onSignOut={() => {}} />);
    expect(screen.getByText("Alice")).toBeInTheDocument();
  });

  it("shows the user initial in the avatar circle", () => {
    render(<AccountMenu user={alice} onSignOut={() => {}} />);
    const buttons = screen.getAllByRole("button");
    expect(buttons[0].textContent).toContain("A");
  });

  it("dropdown is closed by default", () => {
    render(<AccountMenu user={alice} onSignOut={() => {}} />);
    expect(screen.queryByText("Sign out")).not.toBeInTheDocument();
  });

  it("opens dropdown on click", async () => {
    const user = userEvent.setup();
    render(<AccountMenu user={alice} onSignOut={() => {}} />);

    await user.click(screen.getByRole("button"));
    await waitFor(() => {
      expect(screen.getByText("Sign out")).toBeInTheDocument();
      expect(screen.getByText("alice@example.com")).toBeInTheDocument();
    });
  });

  it("calls onSignOut when sign out is clicked", async () => {
    const user = userEvent.setup();
    const handleSignOut = vi.fn();
    render(<AccountMenu user={alice} onSignOut={handleSignOut} />);

    await user.click(screen.getByRole("button"));
    const signOut = await screen.findByText("Sign out");
    await user.click(signOut);

    expect(handleSignOut).toHaveBeenCalledOnce();
  });

  it("falls back to email when name is empty", () => {
    const noName: User = { id: "2", name: "", email: "bob@example.com", avatar_url: "" };
    render(<AccountMenu user={noName} onSignOut={() => {}} />);
    expect(screen.getByText("bob@example.com")).toBeInTheDocument();
  });

  it("hides avatar letter when avatar_url is set", () => {
    const withAvatar: User = {
      id: "3",
      name: "Carol",
      email: "carol@example.com",
      avatar_url: "https://example.com/avatar.png",
    };
    render(<AccountMenu user={withAvatar} onSignOut={() => {}} />);
    const trigger = screen.getAllByRole("button")[0];
    expect(trigger.textContent).toContain("Carol");
  });
});

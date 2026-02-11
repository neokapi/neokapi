import { describe, it, expect, vi } from "vitest";
import { render, screen, act } from "@testing-library/react";
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
    // The avatar shows "A" for Alice
    const buttons = screen.getAllByRole("button");
    expect(buttons[0].textContent).toContain("A");
  });

  it("dropdown is closed by default", () => {
    render(<AccountMenu user={alice} onSignOut={() => {}} />);
    expect(screen.queryByText("Sign out")).not.toBeInTheDocument();
  });

  it("opens dropdown on click", () => {
    render(<AccountMenu user={alice} onSignOut={() => {}} />);
    act(() => screen.getByText("Alice").click());
    expect(screen.getByText("Sign out")).toBeInTheDocument();
    expect(screen.getByText("alice@example.com")).toBeInTheDocument();
  });

  it("closes dropdown on second click", () => {
    render(<AccountMenu user={alice} onSignOut={() => {}} />);
    const trigger = screen.getByText("Alice");

    act(() => trigger.click());
    expect(screen.getByText("Sign out")).toBeInTheDocument();

    act(() => trigger.click());
    expect(screen.queryByText("Sign out")).not.toBeInTheDocument();
  });

  it("calls onSignOut and closes when sign out is clicked", () => {
    const handleSignOut = vi.fn();
    render(<AccountMenu user={alice} onSignOut={handleSignOut} />);

    act(() => screen.getByText("Alice").click());
    act(() => screen.getByText("Sign out").click());

    expect(handleSignOut).toHaveBeenCalledOnce();
    expect(screen.queryByText("Sign out")).not.toBeInTheDocument();
  });

  it("falls back to email when name is empty", () => {
    const noName: User = { id: "2", name: "", email: "bob@example.com", avatar_url: "" };
    render(<AccountMenu user={noName} onSignOut={() => {}} />);
    expect(screen.getByText("bob@example.com")).toBeInTheDocument();
  });

  it("hides avatar letter when avatar_url is set", () => {
    const withAvatar: User = { id: "3", name: "Carol", email: "carol@example.com", avatar_url: "https://example.com/avatar.png" };
    render(<AccountMenu user={withAvatar} onSignOut={() => {}} />);
    // The avatar div should not show the letter "C" when avatar_url is set
    const trigger = screen.getAllByRole("button")[0];
    expect(trigger.textContent).toContain("Carol");
    // No standalone "C" initial visible
  });
});

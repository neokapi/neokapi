import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { PresenceAvatars } from "../components/PresenceAvatars";
import type { CollabUser } from "../hooks/useCollaboration";

const alice: CollabUser = { userId: "u1", name: "Alice", color: "#f43f5e" };
const bob: CollabUser = { userId: "u2", name: "Bob", color: "#8b5cf6" };
const carol: CollabUser = { userId: "u3", name: "Carol", color: "#3b82f6" };

describe("PresenceAvatars", () => {
  it("renders nothing when no other users are connected", () => {
    const { container } = render(
      <PresenceAvatars users={[alice]} currentUserId="u1" />,
    );
    expect(container.querySelector("[data-testid='presence-avatars']")).toBeNull();
  });

  it("renders avatars for other users (excludes current user)", () => {
    render(
      <PresenceAvatars users={[alice, bob, carol]} currentUserId="u1" />,
    );
    const avatarContainer = screen.getByTestId("presence-avatars");
    // Should show Bob and Carol but not Alice.
    expect(avatarContainer.textContent).toContain("B");
    expect(avatarContainer.textContent).toContain("C");
    expect(avatarContainer.textContent).not.toContain("A");
  });

  it("shows all users when no currentUserId is provided", () => {
    render(<PresenceAvatars users={[alice, bob]} />);
    const avatarContainer = screen.getByTestId("presence-avatars");
    expect(avatarContainer.textContent).toContain("A");
    expect(avatarContainer.textContent).toContain("B");
  });

  it("shows overflow count when more than maxVisible users", () => {
    const users: CollabUser[] = Array.from({ length: 8 }, (_, i) => ({
      userId: `u${i}`,
      name: `User${i}`,
      color: "#000",
    }));
    render(<PresenceAvatars users={users} maxVisible={3} />);
    expect(screen.getByText("+5")).toBeDefined();
  });

  it("uses avatar image when avatarUrl is provided", () => {
    const userWithAvatar: CollabUser = {
      userId: "u2",
      name: "Bob",
      color: "#8b5cf6",
      avatarUrl: "https://example.com/bob.jpg",
    };
    render(
      <PresenceAvatars users={[userWithAvatar]} currentUserId="u1" />,
    );
    const img = screen.getByAltText("Bob");
    expect(img).toBeDefined();
    expect(img.getAttribute("src")).toBe("https://example.com/bob.jpg");
  });
});

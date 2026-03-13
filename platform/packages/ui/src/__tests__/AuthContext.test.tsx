import { describe, it, expect } from "vite-plus/test";
import { render, screen, act } from "@testing-library/react";
import { AuthProvider, useAuth } from "../context/AuthContext";

function AuthDisplay() {
  const { user, isAuthenticated, setUser } = useAuth();
  return (
    <div>
      <span data-testid="auth">{isAuthenticated ? "yes" : "no"}</span>
      <span data-testid="name">{user?.name ?? "none"}</span>
      <button
        data-testid="login"
        onClick={() => setUser({ id: "1", email: "a@b.com", name: "Alice", avatar_url: "" })}
      />
      <button data-testid="logout" onClick={() => setUser(null)} />
    </div>
  );
}

describe("AuthContext", () => {
  it("starts unauthenticated with no user", () => {
    render(
      <AuthProvider>
        <AuthDisplay />
      </AuthProvider>,
    );
    expect(screen.getByTestId("auth").textContent).toBe("no");
    expect(screen.getByTestId("name").textContent).toBe("none");
  });

  it("setting a user marks as authenticated", () => {
    render(
      <AuthProvider>
        <AuthDisplay />
      </AuthProvider>,
    );

    act(() => screen.getByTestId("login").click());

    expect(screen.getByTestId("auth").textContent).toBe("yes");
    expect(screen.getByTestId("name").textContent).toBe("Alice");
  });

  it("clearing the user marks as unauthenticated", () => {
    render(
      <AuthProvider>
        <AuthDisplay />
      </AuthProvider>,
    );

    act(() => screen.getByTestId("login").click());
    expect(screen.getByTestId("auth").textContent).toBe("yes");

    act(() => screen.getByTestId("logout").click());
    expect(screen.getByTestId("auth").textContent).toBe("no");
    expect(screen.getByTestId("name").textContent).toBe("none");
  });

  it("throws when useAuth is called outside AuthProvider", () => {
    expect(() => render(<AuthDisplay />)).toThrow("useAuth must be used within AuthProvider");
  });
});

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { WelcomePage } from "../components/WelcomePage";

describe("WelcomePage", () => {
  it("renders the title and tagline", () => {
    render(<WelcomePage onOpen={vi.fn()} onNew={vi.fn()} />);
    expect(screen.getByText("Kapi Desktop")).toBeInTheDocument();
    expect(
      screen.getByText(/plumbing and glue/),
    ).toBeInTheDocument();
  });

  it("displays the neokapi logo", () => {
    render(<WelcomePage onOpen={vi.fn()} onNew={vi.fn()} />);
    const logo = screen.getByAltText("neokapi");
    expect(logo).toBeInTheDocument();
    expect(logo).toHaveAttribute("src", "/neokapi-logo.png");
  });

  it("renders primary action buttons", () => {
    render(<WelcomePage onOpen={vi.fn()} onNew={vi.fn()} />);
    expect(screen.getByText("New Project")).toBeInTheDocument();
    expect(screen.getByText("Open a Kapi project")).toBeInTheDocument();
  });

  it("calls onNew when clicking New Project", async () => {
    const onNew = vi.fn();
    render(<WelcomePage onOpen={vi.fn()} onNew={onNew} />);

    await userEvent.click(screen.getByText("New Project"));
    expect(onNew).toHaveBeenCalledOnce();
    expect(onNew).toHaveBeenCalledWith(
      expect.objectContaining({ version: "v1", name: "New Project" }),
    );
  });

  it("shows Get Started section with all items", () => {
    render(<WelcomePage onOpen={vi.fn()} onNew={vi.fn()} />);
    expect(screen.getByText("Get Started")).toBeInTheDocument();
    expect(screen.getByText("Create a project")).toBeInTheDocument();
    expect(screen.getByText("AI Translate a file")).toBeInTheDocument();
    expect(screen.getByText("Build a flow")).toBeInTheDocument();
    expect(screen.getByText("Pseudo-translate for testing")).toBeInTheDocument();
    expect(screen.getByText("Add plugins")).toBeInTheDocument();
    expect(screen.getByText("Run a quality check")).toBeInTheDocument();
  });

  it("shows Recent Projects empty state", () => {
    render(<WelcomePage onOpen={vi.fn()} onNew={vi.fn()} />);
    expect(screen.getByText("Recent Projects")).toBeInTheDocument();
    expect(screen.getByText(/No recent projects yet/)).toBeInTheDocument();
  });

  it("shows footer with neokapi branding", () => {
    render(<WelcomePage onOpen={vi.fn()} onNew={vi.fn()} />);
    expect(screen.getByText("neokapi")).toBeInTheDocument();
  });
});

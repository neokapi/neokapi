import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { WelcomePage } from "../components/WelcomePage";

describe("WelcomePage", () => {
  it("renders the title and tagline", () => {
    render(<WelcomePage onOpen={vi.fn()} onNew={vi.fn()} />);
    expect(screen.getByText("Kapi")).toBeInTheDocument();
    expect(screen.getByText(/plumbing and glue/)).toBeInTheDocument();
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

  it("shows name form when clicking New Project", async () => {
    render(<WelcomePage onOpen={vi.fn()} onNew={vi.fn()} />);
    await userEvent.click(screen.getByText("New Project"));
    expect(screen.getByPlaceholderText("My App")).toBeInTheDocument();
    expect(screen.getByText("Create Project")).toBeInTheDocument();
  });

  it("calls onNew with the entered name", async () => {
    const onNew = vi.fn();
    render(<WelcomePage onOpen={vi.fn()} onNew={onNew} />);
    await userEvent.click(screen.getByText("New Project"));
    await userEvent.type(screen.getByPlaceholderText("My App"), "Test App");
    await userEvent.click(screen.getByText("Create Project"));
    expect(onNew).toHaveBeenCalledWith(
      expect.objectContaining({ name: "" }),
      "~/KapiProjects/Test App/project.kapi",
    );
  });

  it("shows Get Started section with narrative flow", () => {
    render(<WelcomePage onOpen={vi.fn()} onNew={vi.fn()} />);
    expect(screen.getByText("Get Started")).toBeInTheDocument();
    expect(screen.getByText("Create a project")).toBeInTheDocument();
    expect(screen.getByText("Build a flow")).toBeInTheDocument();
    expect(screen.getByText("Run tools")).toBeInTheDocument();
    expect(screen.getByText("Add plugins")).toBeInTheDocument();
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

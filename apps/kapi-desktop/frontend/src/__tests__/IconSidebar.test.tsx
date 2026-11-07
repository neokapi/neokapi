import { render, screen } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vite-plus/test";
import { IconSidebar } from "../components/IconSidebar";

describe("IconSidebar", () => {
  it("lists one entry per page, in a fixed order", () => {
    render(<IconSidebar active="home" onChange={vi.fn()} />);
    const labels = screen.getAllByRole("button").map((b) => b.getAttribute("aria-label"));
    expect(labels).toEqual([
      "Home",
      "Overview",
      "Context",
      "Review",
      "Project Settings",
      "App Settings",
    ]);
  });

  it("files the stores under Context rather than beside it", () => {
    render(<IconSidebar active="home" onChange={vi.fn()} />);
    expect(screen.queryByRole("button", { name: "Terms" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Content Memory" })).not.toBeInTheDocument();
  });

  it("keeps Context lit while a reader is inside one of its stores", () => {
    render(<IconSidebar active="termbases" onChange={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Context" }).className).toContain("bg-primary");
  });

  it("disables every project item except Home until a project is open", () => {
    render(<IconSidebar active="home" onChange={vi.fn()} projectDisabled={true} />);
    expect(screen.getByRole("button", { name: "Home" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Overview" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Context" })).toBeDisabled();
  });

  it("navigates on click, the Overview by its persisted id", async () => {
    const onChange = vi.fn();
    render(<IconSidebar active="home" onChange={onChange} />);
    await userEvent.click(screen.getByRole("button", { name: "Context" }));
    expect(onChange).toHaveBeenLastCalledWith("context");
    await userEvent.click(screen.getByRole("button", { name: "Overview" }));
    expect(onChange).toHaveBeenLastCalledWith("map");
  });

  it.each(["checks", "reader:docs/a.md", "setup", "project-home", "languages"])(
    "keeps the Overview lit on %s",
    (view) => {
      render(<IconSidebar active={view} onChange={vi.fn()} />);
      expect(screen.getByRole("button", { name: "Overview" }).className).toContain("bg-primary");
    },
  );

  it.each(["project-settings:flows", "flows", "tools", "formats"])(
    "keeps Project Settings lit on %s, where Flows open",
    (view) => {
      render(<IconSidebar active={view} onChange={vi.fn()} />);
      expect(screen.getByRole("button", { name: "Project Settings" }).className).toContain(
        "bg-primary",
      );
    },
  );

  it("keeps Project Settings lit on its Assistants tab, and Context dark", () => {
    render(<IconSidebar active="project-settings:assistants" onChange={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Project Settings" }).className).toContain(
      "bg-primary",
    );
    expect(screen.getByRole("button", { name: "Context" }).className).not.toContain("bg-primary");
  });
});

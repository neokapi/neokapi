import { render, screen } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vite-plus/test";
import { IconSidebar } from "../components/IconSidebar";

describe("IconSidebar", () => {
  it("keeps locale-gated items out of a project with no target languages", () => {
    render(
      <IconSidebar mode="projects" active="home" onChange={vi.fn()} hasTargetLanguages={false} />,
    );
    expect(screen.getByRole("button", { name: "Map" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Review" })).not.toBeInTheDocument();
  });

  it("shows locale-gated items once the project declares targets", () => {
    render(
      <IconSidebar mode="projects" active="home" onChange={vi.fn()} hasTargetLanguages={true} />,
    );
    expect(screen.getByRole("button", { name: "Review" })).toBeInTheDocument();
  });

  it("files the stores under Context rather than beside it", () => {
    render(
      <IconSidebar mode="projects" active="home" onChange={vi.fn()} hasTargetLanguages={true} />,
    );
    expect(screen.getByRole("button", { name: "Context" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Terms" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Content Memory" })).not.toBeInTheDocument();
  });

  it("keeps Context lit while a reader is inside one of its stores", () => {
    render(<IconSidebar mode="projects" active="termbases" onChange={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Context" }).className).toContain("bg-primary");
  });

  it("keeps the stores on the ad-hoc rail, where there is no project to file them under", () => {
    render(<IconSidebar mode="adhoc" active="home" onChange={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Terms" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Content Memory" })).toBeInTheDocument();
  });

  it("disables every project item except Home until a project is open", () => {
    render(<IconSidebar mode="projects" active="home" onChange={vi.fn()} projectDisabled={true} />);
    expect(screen.getByRole("button", { name: "Home" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Project" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Context" })).toBeDisabled();
  });

  it("navigates on click", async () => {
    const onChange = vi.fn();
    render(<IconSidebar mode="projects" active="home" onChange={onChange} />);
    await userEvent.click(screen.getByRole("button", { name: "Context" }));
    expect(onChange).toHaveBeenCalledWith("context");
  });

  it("offers the quick-tool surfaces in ad-hoc mode", () => {
    render(<IconSidebar mode="adhoc" active="home" onChange={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Formats" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Checks" })).not.toBeInTheDocument();
  });

  it("puts the Map first among the project's items, and no Checks page beside it", () => {
    render(
      <IconSidebar mode="projects" active="home" onChange={vi.fn()} hasTargetLanguages={true} />,
    );
    const labels = screen.getAllByRole("button").map((b) => b.getAttribute("aria-label"));
    expect(labels).toEqual([
      "Home",
      "Map",
      "Context",
      "Project",
      "Review",
      "Toolbox",
      "Project Settings",
      "App Settings",
    ]);
  });

  it("keeps the Map lit while its findings are open", () => {
    render(<IconSidebar mode="projects" active="checks" onChange={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Map" }).className).toContain("bg-primary");
  });

  it("keeps Context lit while the hub is open on one of its sections", () => {
    render(<IconSidebar mode="projects" active="context:agent" onChange={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Context" }).className).toContain("bg-primary");
    expect(screen.getByRole("button", { name: "Project" }).className).not.toContain("bg-primary");
  });
});

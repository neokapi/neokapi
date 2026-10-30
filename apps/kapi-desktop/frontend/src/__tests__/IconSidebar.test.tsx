import { render, screen } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vite-plus/test";
import { IconSidebar } from "../components/IconSidebar";

describe("IconSidebar", () => {
  it("keeps locale-gated items out of a project with no target languages", () => {
    render(<IconSidebar active="home" onChange={vi.fn()} hasTargetLanguages={false} />);
    expect(screen.getByRole("button", { name: "Map" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Review" })).not.toBeInTheDocument();
  });

  it("shows locale-gated items once the project declares targets", () => {
    render(<IconSidebar active="home" onChange={vi.fn()} hasTargetLanguages={true} />);
    expect(screen.getByRole("button", { name: "Review" })).toBeInTheDocument();
  });

  it("files the stores under Context rather than beside it", () => {
    render(<IconSidebar active="home" onChange={vi.fn()} hasTargetLanguages={true} />);
    expect(screen.getByRole("button", { name: "Context" })).toBeInTheDocument();
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
    expect(screen.getByRole("button", { name: "Project" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Context" })).toBeDisabled();
  });

  it("navigates on click", async () => {
    const onChange = vi.fn();
    render(<IconSidebar active="home" onChange={onChange} />);
    await userEvent.click(screen.getByRole("button", { name: "Context" }));
    expect(onChange).toHaveBeenCalledWith("context");
  });

  it("puts the Map first among the project's items, and no Checks page beside it", () => {
    render(<IconSidebar active="home" onChange={vi.fn()} hasTargetLanguages={true} />);
    const labels = screen.getAllByRole("button").map((b) => b.getAttribute("aria-label"));
    expect(labels).toEqual([
      "Home",
      "Map",
      "Context",
      "Project",
      "Languages",
      "Review",
      "Toolbox",
      "Project Settings",
      "App Settings",
    ]);
  });

  it("offers Languages + as the one door to language work while the recipe declares no target", () => {
    render(<IconSidebar active="home" onChange={vi.fn()} hasTargetLanguages={false} />);
    const labels = screen.getAllByRole("button").map((b) => b.getAttribute("aria-label"));
    expect(labels).toEqual([
      "Home",
      "Map",
      "Context",
      "Project",
      "Languages +",
      "Toolbox",
      "Project Settings",
      "App Settings",
    ]);
  });

  it("opens the languages view from either form of the entry", async () => {
    const onChange = vi.fn();
    const { rerender } = render(
      <IconSidebar active="home" onChange={onChange} hasTargetLanguages={false} />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Languages +" }));
    expect(onChange).toHaveBeenLastCalledWith("languages");

    rerender(<IconSidebar active="home" onChange={onChange} hasTargetLanguages={true} />);
    expect(screen.queryByRole("button", { name: "Languages +" })).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Languages" }));
    expect(onChange).toHaveBeenLastCalledWith("languages");
  });

  it("keeps Languages + lit on a Review view while Review has nothing to hold", () => {
    render(<IconSidebar active="review" onChange={vi.fn()} hasTargetLanguages={false} />);
    expect(screen.getByRole("button", { name: "Languages +" }).className).toContain("bg-primary");
  });

  it("keeps the Map lit while its findings are open", () => {
    render(<IconSidebar active="checks" onChange={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Map" }).className).toContain("bg-primary");
  });

  it("keeps Context lit while the hub is open on one of its sections", () => {
    render(<IconSidebar active="context:agent" onChange={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Context" }).className).toContain("bg-primary");
    expect(screen.getByRole("button", { name: "Project" }).className).not.toContain("bg-primary");
  });
});

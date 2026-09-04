// @vitest-environment jsdom
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { PageHeader } from "../components/PageHeader";
import { SectionHeading } from "../components/SectionHeading";

afterEach(cleanup);

describe("PageHeader", () => {
  it("renders the title as an h1", () => {
    render(<PageHeader title="Content memory" />);
    const h1 = screen.getByRole("heading", { level: 1 });
    expect(h1.textContent).toBe("Content memory");
  });

  it("renders the subtitle and actions when given", () => {
    render(
      <PageHeader
        title="Flows"
        subtitle="Ordered pipelines of tool steps"
        actions={<button type="button">New flow</button>}
      />,
    );
    expect(screen.getByText("Ordered pipelines of tool steps")).toBeTruthy();
    expect(screen.getByRole("button", { name: "New flow" })).toBeTruthy();
  });

  it("omits the subtitle when not given", () => {
    render(<PageHeader title="Terms" />);
    expect(screen.queryByText(/pipelines/)).toBeNull();
  });

  it("takes an element as the subtitle, so a count can be pluralized", () => {
    render(<PageHeader title="Projects" subtitle={<span>3 projects in this workspace</span>} />);
    expect(screen.getByText("3 projects in this workspace")).toBeTruthy();
  });

  it("renders an eyebrow above the title", () => {
    render(<PageHeader title="Projects" eyebrow="Acme" />);
    expect(screen.getByText("Acme")).toBeTruthy();
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Projects");
  });

  it("keeps the hero title an h1, centred, with its lead and actions", () => {
    render(
      <PageHeader
        variant="hero"
        eyebrow="Acme"
        title="Set up your workspace"
        subtitle="Start with your assistant, your files, or your team."
        actions={<button type="button">Create a project</button>}
      />,
    );
    const h1 = screen.getByRole("heading", { level: 1 });
    expect(h1.textContent).toBe("Set up your workspace");
    expect(h1.className).toContain("text-3xl");
    expect(screen.getByText("Acme")).toBeTruthy();
    expect(screen.getByText("Start with your assistant, your files, or your team.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Create a project" })).toBeTruthy();
  });

  it("lets a caller's spacing win over the default", () => {
    const { container } = render(<PageHeader title="Projects" className="mb-8" />);
    const root = container.firstElementChild as HTMLElement;
    expect(root.className).toContain("mb-8");
    expect(root.className).not.toContain("mb-6");
  });
});

describe("SectionHeading", () => {
  it("renders its children as an h2", () => {
    render(<SectionHeading>Where content sits</SectionHeading>);
    const h2 = screen.getByRole("heading", { level: 2 });
    expect(h2.textContent).toContain("Where content sits");
  });

  it("renders a trailing count", () => {
    render(<SectionHeading count={7}>Channels</SectionHeading>);
    const h2 = screen.getByRole("heading", { level: 2 });
    expect(within(h2).getByText("7")).toBeTruthy();
  });
});

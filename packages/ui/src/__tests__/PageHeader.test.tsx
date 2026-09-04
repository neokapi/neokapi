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

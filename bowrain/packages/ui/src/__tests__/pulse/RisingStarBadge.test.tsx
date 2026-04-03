import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { RisingStarBadge } from "../../components/pulse";

describe("RisingStarBadge", () => {
  it("shows positive growth with + prefix", () => {
    render(<RisingStarBadge growth={5.3} />);
    expect(screen.getByText("+5.3%")).toBeTruthy();
  });

  it("shows negative growth without + prefix", () => {
    render(<RisingStarBadge growth={-2.7} />);
    expect(screen.getByText("-2.7%")).toBeTruthy();
  });

  it("shows zero growth without + prefix", () => {
    render(<RisingStarBadge growth={0} />);
    expect(screen.getByText("0.0%")).toBeTruthy();
  });
});

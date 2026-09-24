// @vitest-environment jsdom
import React from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import FlowBuilderRunner from "./FlowBuilderRunner";

vi.mock("@neokapi/flow-editor", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@neokapi/flow-editor")>()),
  FlowEditor: () => <div>Flow editor</div>,
}));

afterEach(cleanup);

describe("FlowBuilderRunner initial sample", () => {
  it("honors an explicit sample override for a scenario with its own sample", () => {
    render(
      <FlowBuilderRunner
        assets={null}
        defaultScenarioId="pseudo"
        defaultSampleId="checkout-messageformat"
      />,
    );
    expect(screen.getByRole("button", { name: "checkout.mf" })).toBeTruthy();
  });

  it("uses the scenario's sample when no override is provided", () => {
    render(<FlowBuilderRunner assets={null} defaultScenarioId="pseudo" />);
    expect(
      screen.getByRole("button", { name: "support-reply.json" }),
    ).toBeTruthy();
  });
});

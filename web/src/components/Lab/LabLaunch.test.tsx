// @vitest-environment jsdom
import React, { useEffect, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { renderToString } from "react-dom/server";
import { LabLaunch } from "./LabLaunch";

afterEach(cleanup);

describe("LabLaunch", () => {
  it("does not render experiment content on the server or before launch", () => {
    const mount = vi.fn();
    function Experiment(): React.ReactElement {
      mount();
      return <p>Experiment content</p>;
    }
    const element = (
      <LabLaunch>
        <Experiment />
      </LabLaunch>
    );
    expect(renderToString(element)).toContain("Open experiment");
    render(element);
    expect(mount).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Open experiment" }));
    expect(screen.getByText("Experiment content")).toBeTruthy();
  });

  it("cleans up on close and starts fresh local controls on reopen", () => {
    const dispose = vi.fn();
    function Experiment(): React.ReactElement {
      const [value, setValue] = useState("sample");
      useEffect(() => dispose, []);
      return (
        <input
          aria-label="Input"
          value={value}
          onChange={(event) => setValue(event.target.value)}
        />
      );
    }
    render(
      <LabLaunch>
        <Experiment />
      </LabLaunch>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Open experiment" }));
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "edited" } });
    fireEvent.click(screen.getByRole("button", { name: "Close experiment" }));
    expect(dispose).toHaveBeenCalledOnce();
    fireEvent.click(screen.getByRole("button", { name: "Open experiment" }));
    expect((screen.getByRole("textbox") as HTMLInputElement).value).toBe("sample");
  });

  it("contains rendering failures and allows another attempt", () => {
    const error = vi.spyOn(console, "error").mockImplementation(() => {});
    let fail = true;
    function Experiment(): React.ReactElement {
      if (fail) throw new Error("unavailable");
      return <p>Recovered</p>;
    }
    try {
      render(
        <LabLaunch>
          <Experiment />
        </LabLaunch>,
      );
      fireEvent.click(screen.getByRole("button", { name: "Open experiment" }));
      expect(screen.getByRole("alert")).toBeTruthy();
      fail = false;
      fireEvent.click(screen.getByRole("button", { name: "Try again" }));
      expect(screen.getByText("Recovered")).toBeTruthy();
    } finally {
      error.mockRestore();
    }
  });
});

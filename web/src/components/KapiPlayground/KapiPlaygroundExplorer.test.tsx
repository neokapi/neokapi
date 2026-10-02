// @vitest-environment jsdom
import React, { useEffect, useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { PlaygroundDialog } from "./PlaygroundDialog";

const showModal = vi.fn(function (this: HTMLDialogElement) {
  this.setAttribute("open", "");
});
const close = vi.fn(function (this: HTMLDialogElement) {
  this.removeAttribute("open");
});
const originalShow = Object.getOwnPropertyDescriptor(HTMLDialogElement.prototype, "showModal");
const originalClose = Object.getOwnPropertyDescriptor(HTMLDialogElement.prototype, "close");
beforeEach(() => {
  Object.defineProperty(HTMLDialogElement.prototype, "showModal", {
    configurable: true,
    value: showModal,
  });
  Object.defineProperty(HTMLDialogElement.prototype, "close", { configurable: true, value: close });
});
afterEach(() => {
  cleanup();
  for (const [name, descriptor] of [
    ["showModal", originalShow],
    ["close", originalClose],
  ] as const) {
    if (descriptor) Object.defineProperty(HTMLDialogElement.prototype, name, descriptor);
    else Reflect.deleteProperty(HTMLDialogElement.prototype, name);
  }
  document.body.style.overflow = "";
  vi.clearAllMocks();
});

describe("full-window CLI playground", () => {
  it("launches once, retains the session on close and restores focus", () => {
    const mounted = vi.fn();
    function Session(): React.ReactElement {
      const [command, setCommand] = useState("kapi info");
      useEffect(() => {
        mounted();
      }, []);
      return (
        <input
          aria-label="Command"
          value={command}
          onChange={(event) => setCommand(event.target.value)}
        />
      );
    }
    render(
      <PlaygroundDialog>
        <Session />
      </PlaygroundDialog>,
    );
    expect(mounted).not.toHaveBeenCalled();
    document.body.style.overflow = "auto";
    fireEvent.click(screen.getByRole("button", { name: "Open terminal" }));
    expect(showModal).toHaveBeenCalledOnce();
    expect(document.body.style.overflow).toBe("hidden");
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "kapi stat my-file.json" } });
    fireEvent.click(screen.getByRole("button", { name: "Close terminal" }));
    expect(document.body.style.overflow).toBe("auto");
    const resume = screen.getByRole("button", { name: "Resume terminal" });
    expect(document.activeElement).toBe(resume);
    fireEvent.click(resume);
    expect(mounted).toHaveBeenCalledOnce();
    expect((screen.getByRole("textbox") as HTMLInputElement).value).toBe("kapi stat my-file.json");
  });

  it("closes on native Escape cancellation and restores scrolling on unmount", () => {
    const { unmount } = render(
      <PlaygroundDialog>
        <p>Session</p>
      </PlaygroundDialog>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Open terminal" }));
    fireEvent(screen.getByRole("dialog"), new Event("cancel", { cancelable: true }));
    expect(close).toHaveBeenCalledOnce();
    expect(document.body.style.overflow).toBe("");
    fireEvent.click(screen.getByRole("button", { name: "Resume terminal" }));
    unmount();
    expect(document.body.style.overflow).toBe("");
  });
  it("leaves Escape cancellation to an open file preview", () => {
    render(
      <PlaygroundDialog>
        <div className="kapi-pg-overlay">File preview</div>
      </PlaygroundDialog>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Open terminal" }));
    fireEvent(screen.getByRole("dialog"), new Event("cancel", { cancelable: true }));
    expect(close).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeTruthy();
  });
});

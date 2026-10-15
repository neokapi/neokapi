import { describe, it, expect, vi } from "vitest";
import userEvent from "@testing-library/user-event";
import { render, screen } from "./testUtils";

import { DESKTOP_OVERVIEW_URL, StartHere } from "../components/StartHere";

function renderStartHere(hasOpenProject = true) {
  const handlers = {
    onDismiss: vi.fn(),
    onCreateSampleProject: vi.fn(),
    onConnectAssistant: vi.fn(),
  };
  render(<StartHere {...handlers} hasOpenProject={hasOpenProject} />);
  return handlers;
}

describe("Start here", () => {
  it("states the three steps word for word", () => {
    renderStartHere();
    expect(
      screen.getByText(
        "Open a project, or try the sample. The sample arrives with an assistant's notes to review.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        "Connect an assistant. kapi writes the project's rules where it reads them, and the assistant records what it notices.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Come back. What it noticed appears under What's new; keep what is right."),
    ).toBeInTheDocument();
  });

  it("creates the KapiMart sample", async () => {
    const { onCreateSampleProject } = renderStartHere();
    await userEvent.click(screen.getByTestId("sample-kapimart"));
    expect(onCreateSampleProject).toHaveBeenCalledWith("kapimart");
  });

  it("opens the open project's Assistant section", async () => {
    const { onConnectAssistant } = renderStartHere();
    await userEvent.click(screen.getByRole("button", { name: "Connect an assistant" }));
    expect(onConnectAssistant).toHaveBeenCalled();
  });

  it("waits for a project before it connects an assistant", () => {
    renderStartHere(false);
    expect(screen.getByRole("button", { name: "Connect an assistant" })).toBeDisabled();
  });

  it("links How it works to the desktop overview", () => {
    renderStartHere();
    expect(screen.getByRole("link", { name: "How it works" })).toHaveAttribute(
      "href",
      DESKTOP_OVERVIEW_URL,
    );
  });

  it("dismisses", async () => {
    const { onDismiss } = renderStartHere();
    await userEvent.click(screen.getByRole("button", { name: "Dismiss Start here" }));
    expect(onDismiss).toHaveBeenCalled();
  });
});

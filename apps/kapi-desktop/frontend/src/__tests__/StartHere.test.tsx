import { afterEach, beforeEach, describe, it, expect, vi } from "vite-plus/test";
import userEvent from "@testing-library/user-event";
import { render, screen, waitFor } from "./testUtils";

import { StartHere } from "../components/StartHere";
import { DesktopGuide, GUIDE_STORAGE_KEY } from "../guide/DesktopGuide";
import { GUIDE_IDS } from "../guide/guides";

function renderStartHere(hasOpenProject = true) {
  const handlers = {
    onDismiss: vi.fn(),
    onCreateSampleProject: vi.fn(async () => {}),
  };
  const navigate = vi.fn();
  render(
    <DesktopGuide navigate={navigate} hasProject={hasOpenProject} onOpenUrl={vi.fn()}>
      <StartHere {...handlers} hasOpenProject={hasOpenProject} />
    </DesktopGuide>,
  );
  return { ...handlers, navigate };
}

const bubble = () => document.querySelector<HTMLElement>('[data-slot="guide-bubble"]');

beforeEach(() => localStorage.removeItem(GUIDE_STORAGE_KEY));
afterEach(() => localStorage.removeItem(GUIDE_STORAGE_KEY));

describe("Get started", () => {
  it("offers three tours, none done yet", () => {
    renderStartHere();
    expect(screen.getByText("Get started")).toBeInTheDocument();
    expect(
      screen.getByText("Try the sample project and take a tour of its Overview."),
    ).toBeInTheDocument();
    expect(document.querySelector('[data-slot="start-here-done"]')?.textContent).toBe("0 of 3");
    expect(bubble()).toBeNull();
  });

  it("marks the tours finished on this machine", () => {
    localStorage.setItem(GUIDE_STORAGE_KEY, JSON.stringify([GUIDE_IDS.decide]));
    renderStartHere();
    expect(document.querySelector('[data-slot="start-here-done"]')?.textContent).toBe("1 of 3");
    const done = document.querySelectorAll('[data-slot="start-here-step"][data-done]');
    expect(done).toHaveLength(1);
  });

  it("creates the KapiMart sample, then starts the project tour", async () => {
    const { onCreateSampleProject, navigate } = renderStartHere();
    await userEvent.click(screen.getByTestId("sample-kapimart"));
    expect(onCreateSampleProject).toHaveBeenCalledWith("kapimart");
    await waitFor(() => expect(bubble()).not.toBeNull());
    expect(navigate).toHaveBeenCalledWith("map");
  });

  it("starts the Decide tour", async () => {
    const { navigate } = renderStartHere();
    await userEvent.click(document.querySelector<HTMLElement>('[data-slot="start-here-decide"]')!);
    await waitFor(() => expect(navigate).toHaveBeenCalledWith("context:rules"));
  });

  it("starts the assistant tour", async () => {
    const { navigate } = renderStartHere();
    await userEvent.click(document.querySelector<HTMLElement>('[data-slot="start-here-connect"]')!);
    await waitFor(() => expect(navigate).toHaveBeenCalledWith("project-settings:assistants"));
  });

  it("waits for a project before the project tours", () => {
    renderStartHere(false);
    expect(document.querySelector('[data-slot="start-here-decide"]')).toBeDisabled();
    expect(document.querySelector('[data-slot="start-here-connect"]')).toBeDisabled();
    expect(screen.getByTestId("sample-kapimart")).toBeEnabled();
  });

  it("does not count a skipped tour as done", async () => {
    renderStartHere();
    await userEvent.click(document.querySelector<HTMLElement>('[data-slot="start-here-decide"]')!);
    await waitFor(() => expect(bubble()).not.toBeNull());
    await userEvent.click(screen.getByRole("button", { name: "Skip tour" }));
    await waitFor(() => expect(bubble()).toBeNull());
    expect(document.querySelector('[data-slot="start-here-done"]')?.textContent).toBe("0 of 3");
    expect(localStorage.getItem(GUIDE_STORAGE_KEY)).toBeNull();
  });

  it("hides", async () => {
    const { onDismiss } = renderStartHere();
    await userEvent.click(screen.getByRole("button", { name: "Hide Get started" }));
    expect(onDismiss).toHaveBeenCalled();
  });
});

import { render, screen } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vite-plus/test";
import { ProjectSettingsTabs, settingsSectionOf } from "../components/ProjectSettingsTabs";

const pages = {
  settings: <p>project settings</p>,
  content: <p>collections</p>,
  assistants: <p>assistants page</p>,
  sessions: <p>sessions page</p>,
  share: <p>share page</p>,
};

describe("Project Settings tabs", () => {
  it("opens on the project's settings", () => {
    render(<ProjectSettingsTabs section={null} onNavigate={vi.fn()} pages={pages} />);
    expect(screen.getByText("project settings")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Settings" })).toHaveAttribute("aria-selected", "true");
    expect(screen.queryByText("collections")).not.toBeInTheDocument();
  });

  it("offers one tab per page, each a route", async () => {
    const onNavigate = vi.fn();
    render(<ProjectSettingsTabs section={null} onNavigate={onNavigate} pages={pages} />);
    expect(screen.getAllByRole("tab").map((tab) => tab.textContent)).toEqual([
      "Settings",
      "Content",
      "Assistants",
      "Share",
    ]);
    await userEvent.click(screen.getByRole("tab", { name: "Assistants" }));
    expect(onNavigate).toHaveBeenLastCalledWith("project-settings:assistants");
    await userEvent.click(screen.getByRole("tab", { name: "Settings" }));
    expect(onNavigate).toHaveBeenLastCalledWith("project-settings");
  });

  it("shows Sessions under the Assistants tab", () => {
    render(<ProjectSettingsTabs section="sessions" onNavigate={vi.fn()} pages={pages} />);
    expect(screen.getByText("sessions page")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Assistants" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("reads the section from the view", () => {
    expect(settingsSectionOf("project-settings")).toBeNull();
    expect(settingsSectionOf("project-settings:content")).toBe("content");
    expect(settingsSectionOf("project-settings:tools")).toBeNull();
  });
});

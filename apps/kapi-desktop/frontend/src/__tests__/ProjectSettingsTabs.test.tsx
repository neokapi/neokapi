import { render, screen } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vite-plus/test";
import {
  ProjectSettingsTabs,
  settingsSectionOf,
  type SettingsToolSection,
} from "../components/ProjectSettingsTabs";

const tools: Record<SettingsToolSection, React.ReactNode> = {
  tools: <p>tool runner</p>,
  flows: <p>flow library</p>,
  terms: <p>terms page</p>,
  memories: <p>memories page</p>,
  formats: <p>formats page</p>,
};

describe("Project Settings → Tools", () => {
  it("opens on the project's settings", () => {
    render(
      <ProjectSettingsTabs
        section={null}
        onNavigate={vi.fn()}
        settings={<p>project settings</p>}
        tools={tools}
      />,
    );
    expect(screen.getByText("project settings")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Settings" })).toHaveAttribute("aria-selected", "true");
    expect(screen.queryByText("tool runner")).not.toBeInTheDocument();
  });

  it("reaches Tools from its tab", async () => {
    const onNavigate = vi.fn();
    render(
      <ProjectSettingsTabs
        section={null}
        onNavigate={onNavigate}
        settings={<p>project settings</p>}
        tools={tools}
      />,
    );
    await userEvent.click(screen.getByRole("tab", { name: "Tools" }));
    expect(onNavigate).toHaveBeenCalledWith("project-settings:tools");
  });

  it("holds the app-wide pages under Tools, one tab each", async () => {
    const onNavigate = vi.fn();
    render(
      <ProjectSettingsTabs
        section="terms"
        onNavigate={onNavigate}
        settings={<p>project settings</p>}
        tools={tools}
      />,
    );
    expect(screen.getByText("terms page")).toBeInTheDocument();
    expect(screen.queryByText("project settings")).not.toBeInTheDocument();
    const sections = screen
      .getAllByRole("tab")
      .slice(2)
      .map((tab) => tab.textContent);
    expect(sections).toEqual(["Tools", "Flows", "Terms", "Content memory", "Formats"]);
    await userEvent.click(screen.getByRole("tab", { name: "Formats" }));
    expect(onNavigate).toHaveBeenCalledWith("project-settings:formats");
  });

  it("reads the section from the view", () => {
    expect(settingsSectionOf("project-settings")).toBeNull();
    expect(settingsSectionOf("project-settings:memories")).toBe("memories");
    expect(settingsSectionOf("project-settings:nonsense")).toBeNull();
  });
});

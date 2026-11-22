import { render, screen } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect } from "vite-plus/test";
import { SettingsPage } from "../components/SettingsPage";
import { ErrorProvider } from "../components/ErrorBanner";

describe("App Settings", () => {
  it("holds the Formats as a tab beside Plugins", async () => {
    render(
      <ErrorProvider>
        <SettingsPage theme="light" uiLanguage="en" />
      </ErrorProvider>,
    );
    const tabs = screen.getAllByRole("tab").map((tab) => tab.textContent);
    expect(tabs).toEqual(["General", "AI Models", "Plugins", "Formats", "Languages"]);
    await userEvent.click(screen.getByRole("tab", { name: "Formats" }));
    expect(screen.getByRole("tab", { name: "Formats" })).toHaveAttribute("aria-selected", "true");
  });
});

import { describe, it, expect, vi } from "vite-plus/test";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NotificationSettings, type DigestSettings } from "../components/NotificationSettings";

const defaults: DigestSettings = {
  frequency: "daily",
  quiet_start: "22:00",
  quiet_end: "08:00",
  timezone: "America/New_York",
};

describe("NotificationSettings", () => {
  it("renders all three cards", () => {
    render(<NotificationSettings settings={defaults} onChange={() => {}} />);
    expect(screen.getByText("Email digest")).toBeInTheDocument();
    expect(screen.getByText("Quiet hours")).toBeInTheDocument();
    expect(screen.getAllByText("Timezone")[0]).toBeInTheDocument();
  });

  it("shows quiet hours time inputs when enabled", () => {
    render(<NotificationSettings settings={defaults} onChange={() => {}} />);
    expect(screen.getByLabelText("From")).toBeInTheDocument();
    expect(screen.getByLabelText("Until")).toBeInTheDocument();
  });

  it("hides time inputs when quiet hours are off", () => {
    const noQuiet: DigestSettings = { ...defaults, quiet_start: "", quiet_end: "" };
    render(<NotificationSettings settings={noQuiet} onChange={() => {}} />);
    expect(screen.queryByLabelText("From")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Until")).not.toBeInTheDocument();
  });

  it("calls onChange with defaults when quiet hours toggle is enabled", async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    const noQuiet: DigestSettings = { ...defaults, quiet_start: "", quiet_end: "" };
    render(<NotificationSettings settings={noQuiet} onChange={handleChange} />);

    // The switch is the quiet hours toggle
    const toggle = screen.getByRole("switch");
    await user.click(toggle);

    expect(handleChange).toHaveBeenCalledWith(
      expect.objectContaining({ quiet_start: "22:00", quiet_end: "08:00" }),
    );
  });

  it("calls onChange with empty times when quiet hours toggle is disabled", async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    render(<NotificationSettings settings={defaults} onChange={handleChange} />);

    const toggle = screen.getByRole("switch");
    await user.click(toggle);

    expect(handleChange).toHaveBeenCalledWith(
      expect.objectContaining({ quiet_start: "", quiet_end: "" }),
    );
  });

  it("shows saving indicator", () => {
    render(<NotificationSettings settings={defaults} onChange={() => {}} saving />);
    expect(screen.getByText("Saving...")).toBeInTheDocument();
  });

  it("does not show saving indicator by default", () => {
    render(<NotificationSettings settings={defaults} onChange={() => {}} />);
    expect(screen.queryByText("Saving...")).not.toBeInTheDocument();
  });
});

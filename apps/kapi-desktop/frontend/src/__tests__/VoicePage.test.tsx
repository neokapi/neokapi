import { render, screen, within } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect } from "vitest";
import { VoicePage } from "../components/VoicePage";
import { valueSetsFixture, voiceFixture } from "./voiceFixture";

describe("VoicePage", () => {
  it("lists every declared point, the project's own first", () => {
    render(<VoicePage tabID="t1" result={voiceFixture} />);
    const rows = screen.getAllByTestId("voice-point-row");
    expect(rows.map((r) => r.textContent?.split("\n")[0])).toHaveLength(3);
    expect(rows[0]).toHaveTextContent("project default");
    expect(rows[1]).toHaveTextContent("campaign");
    expect(rows[2]).toHaveTextContent("support");
  });

  it("names the binding that selected the profile", () => {
    render(<VoicePage tabID="t1" result={voiceFixture} />);
    // The recipe plumbing sits on its own quiet line, off the resolution chain.
    const plumbing = screen.getByTestId("voice-plumbing");
    expect(plumbing).toHaveTextContent("defaults.voice");
    expect(plumbing).toHaveTextContent("profile_file");
    expect(plumbing).toHaveTextContent(".kapi/voice.yaml");
  });

  it("draws the skipped binding and what governs in its place", async () => {
    render(<VoicePage tabID="t1" result={voiceFixture} />);
    await userEvent.click(screen.getByRole("button", { name: /campaign/ }));

    const chain = screen.getByTestId("voice-chain");
    expect(chain).toHaveTextContent("campaign");
    expect(chain).toHaveTextContent("window closed");
    expect(chain).toHaveTextContent("2026-08-29");
    // The profile that governs instead is named, so the reader is not left to
    // infer that nothing does.
    expect(chain).toHaveTextContent("project default");
    expect(screen.getByTestId("voice-validity")).toHaveTextContent("expired");
  });

  it("says a term rule's severity and whether it fails or reports", () => {
    render(<VoicePage tabID="t1" result={voiceFixture} />);
    const rules = screen.getAllByTestId("voice-term-rule");
    const logIn = rules.find((r) => r.textContent?.includes("log in"));
    expect(logIn).toBeDefined();
    expect(logIn).toHaveTextContent("sign in");
    expect(logIn).toHaveTextContent("major");

    // A rule with no replacement is skipped by the tools, and says so.
    const bare = rules.find((r) => r.textContent?.includes("bulletproof"));
    expect(bare).toHaveTextContent("no replacement, so tools skip it");
  });

  it("renders tone, style patterns and examples", () => {
    render(<VoicePage tabID="t1" result={voiceFixture} />);
    expect(within(screen.getByTestId("voice-personality")).getByText("clear")).toBeInTheDocument();

    const patterns = screen.getAllByTestId("voice-pattern");
    // The human-readable description is the label; the regex is behind a tooltip.
    expect(patterns[0]).toHaveTextContent("Corporate filler.");
    expect(patterns[0]).toHaveTextContent("up to 2 per 1000 words");

    expect(screen.getAllByTestId("voice-example")[0]).toHaveTextContent("Use the portal.");
  });

  it("renders the locale, channel and persona overrides as themselves", () => {
    render(<VoicePage tabID="t1" result={voiceFixture} />);
    expect(screen.getByText("By language")).toBeInTheDocument();
    expect(screen.getByText("nb-NO")).toBeInTheDocument();
    expect(screen.getByText("By channel")).toBeInTheDocument();
    expect(screen.getByText("docs")).toBeInTheDocument();
    expect(screen.getByText("By persona")).toBeInTheDocument();
    expect(screen.getByText("support-agent")).toBeInTheDocument();
  });

  it("carries the guide a tool reads", () => {
    render(<VoicePage tabID="t1" result={voiceFixture} />);
    expect(screen.getByText(/Write as Northsea/)).toBeInTheDocument();
  });

  it("says so when nothing binds at a point", async () => {
    render(<VoicePage tabID="t1" result={voiceFixture} />);
    await userEvent.click(screen.getByRole("button", { name: /support/ }));
    expect(screen.getByText("No voice profile binds at this point")).toBeInTheDocument();
  });

  it("offers to edit the file the point resolves to", async () => {
    render(<VoicePage tabID="t1" result={voiceFixture} valueSets={valueSetsFixture} />);
    await userEvent.click(screen.getByTestId("voice-edit"));
    expect(screen.getByTestId("voice-editor")).toHaveTextContent(".kapi/voice.yaml");
  });

  it("offers to give an inheriting point its own voice", async () => {
    render(<VoicePage tabID="t1" result={voiceFixture} valueSets={valueSetsFixture} />);
    await userEvent.click(screen.getAllByTestId("voice-point-row")[1]);
    expect(screen.getByTestId("voice-edit")).toHaveTextContent("Give this point its own voice");
  });

  it("refuses to edit a binding no file edit reaches", () => {
    const pack = structuredClone(voiceFixture);
    pack.points[0].edit = {
      writable: false,
      exists: false,
      inherited: false,
      reason: 'defaults.voice binds the "technical-docs" starter pack.',
    };
    render(<VoicePage tabID="t1" result={pack} valueSets={valueSetsFixture} />);
    expect(screen.getByTestId("voice-edit")).toBeDisabled();
    expect(screen.getByTestId("voice-edit-reason")).toHaveTextContent("starter pack");
  });

  it("renders read-only when editing is off", () => {
    render(<VoicePage tabID="t1" result={voiceFixture} editable={false} />);
    expect(screen.queryByTestId("voice-edit")).not.toBeInTheDocument();
  });

  it("reads as complete for a project with a single point", () => {
    render(
      <VoicePage tabID="t1" result={{ at: voiceFixture.at, points: [voiceFixture.points[0]] }} />,
    );
    expect(screen.getAllByTestId("voice-point-row")).toHaveLength(1);
    expect(screen.getByTestId("voice-detail")).toBeInTheDocument();
    expect(screen.queryByText(/did not load/)).not.toBeInTheDocument();
  });
});

import { render, screen, within } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { VoiceProfileEditor } from "../components/voice/VoiceProfileEditor";
import { valueSetsFixture, voiceFixture } from "./voiceFixture";
import type { VoiceProfile, VoiceSaveResult } from "../types/voice";

const target = { profile: "northsea", writable: true, exists: true, inherited: false };

function renderEditor(overrides: Partial<React.ComponentProps<typeof VoiceProfileEditor>> = {}) {
  const save = vi.fn<(p: VoiceProfile) => Promise<VoiceSaveResult | null>>().mockResolvedValue({
    saved: true,
    changed: true,
    profile: "northsea",
    problems: [],
  });
  const onSaved = vi.fn();
  render(
    <VoiceProfileEditor
      tabID="t1"
      profileName=""
      target={target}
      profile={voiceFixture.points[0].profile as VoiceProfile}
      valueSets={valueSetsFixture}
      save={save}
      onSaved={onSaved}
      onCancel={vi.fn()}
      {...overrides}
    />,
  );
  return { save, onSaved };
}

describe("VoiceProfileEditor", () => {
  it("says which profile a save lands in", () => {
    renderEditor();
    expect(screen.getByTestId("voice-editor")).toHaveTextContent("northsea");
    expect(screen.getByTestId("voice-editor")).toHaveTextContent("Editing");
  });

  it("says a save creates the profile when the store holds none", () => {
    renderEditor({ target: { ...target, exists: false } });
    expect(screen.getByTestId("voice-editor")).toHaveTextContent("Creating");
  });

  it("warns that saving gives an inheriting point its own voice", () => {
    renderEditor({ target: { ...target, exists: false, inherited: true } });
    expect(screen.getByTestId("voice-editor")).toHaveTextContent(
      "This point reads a voice bound coarser",
    );
  });

  it("edits a field and sends the whole profile", async () => {
    const { save, onSaved } = renderEditor();
    const name = screen.getByLabelText("Name");
    await userEvent.clear(name);
    await userEvent.type(name, "Northsea Two");
    await userEvent.click(screen.getByRole("button", { name: /Save/ }));

    expect(save).toHaveBeenCalledTimes(1);
    const sent = save.mock.calls[0][0];
    expect(sent.name).toBe("Northsea Two");
    // Everything the profile carried travels with the edit rather than being
    // dropped by a form that models only part of it.
    expect(sent.tone?.personality).toEqual(["clear", "calm"]);
    expect(sent.locales?.["nb-NO"].formality).toBe("informal");
    expect(sent.personas?.["support-agent"]).toBeDefined();
    expect(onSaved).toHaveBeenCalled();
  });

  it("offers a closed set as a picker and an open one as free text", () => {
    renderEditor();
    const style = within(screen.getByTestId("style-editor-profile"));
    // person_pov is read by the offline check, so its values are fixed.
    expect(style.getByLabelText("Point of view").tagName).toBe("BUTTON");

    // Tone is described, not enumerated: a register the list does not name is
    // often what distinguishes one voice from another.
    const tone = within(screen.getByTestId("tone-editor-profile"));
    expect(tone.getByLabelText("Formality").tagName).toBe("INPUT");
  });

  it("points word rules at the project's terms", () => {
    renderEditor();
    expect(screen.getByTestId("voice-terms-note")).toHaveTextContent("Word rules are terms");
    expect(screen.queryByTestId("preferred-terms")).toBeNull();
  });

  it("shows the problems a refused save reports, and does not close", async () => {
    const save = vi.fn().mockResolvedValue({
      saved: false,
      changed: false,
      problems: [{ field: "style.person_pov", message: 'unknown value "fourth"' }],
    });
    const onSaved = vi.fn();
    render(
      <VoiceProfileEditor
        tabID="t1"
        profileName=""
        target={target}
        profile={{ name: "Northsea" }}
        valueSets={valueSetsFixture}
        save={save}
        onSaved={onSaved}
        onCancel={vi.fn()}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: /Save/ }));

    const problems = await screen.findByTestId("voice-problems");
    expect(problems).toHaveTextContent("Fix these before saving");
    expect(problems).toHaveTextContent("style.person_pov");
    expect(onSaved).not.toHaveBeenCalled();
  });

  it("separates a note from a refusal", async () => {
    const save = vi.fn().mockResolvedValue({
      saved: true,
      changed: true,
      problems: [
        { field: "tone.formality", message: "not one of the usual values", warning: true },
      ],
    });
    render(
      <VoiceProfileEditor
        tabID="t1"
        profileName=""
        target={target}
        profile={{ name: "Northsea" }}
        valueSets={valueSetsFixture}
        save={save}
        onSaved={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: /Save/ }));

    const problems = await screen.findByTestId("voice-problems");
    expect(problems).toHaveTextContent("Saved, with notes");
  });

  it("edits an override with the same controls as the profile", async () => {
    const { save } = renderEditor();
    const locales = screen.getByTestId("locale-overrides");
    const notes = within(locales).getByLabelText("What a reader here expects");
    await userEvent.clear(notes);
    await userEvent.type(notes, "Direct address, no hedging.");
    await userEvent.click(screen.getByRole("button", { name: /Save/ }));

    expect(save.mock.calls[0][0].locales?.["nb-NO"].cultural_notes).toBe(
      "Direct address, no hedging.",
    );
  });
});

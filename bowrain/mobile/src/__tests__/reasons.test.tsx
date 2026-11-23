import { DECISION_REASONS, PERSON_REASONS } from "@neokapi/contract-types";
import { render, screen } from "@testing-library/react-native";

import en from "../../i18n/en.json";
import i18n from "../lib/i18n";
import { DecisionRow } from "../components/DecisionRow";
import { reasonLabel } from "../lib/reasons";

describe("reasons", () => {
  it("has a phrase in the catalog for every reason code a decision may carry", () => {
    const phrases = en.reasons as Record<string, string>;
    for (const code of DECISION_REASONS) {
      expect(phrases[code]).toBeTruthy();
      expect(reasonLabel(i18n.t, code)).toBe(phrases[code]);
    }
  });

  it("offers every reason a person may give", () => {
    for (const code of PERSON_REASONS) expect(DECISION_REASONS).toContain(code);
  });

  it("shows a past decision's reasons as phrases, beside its note", async () => {
    await render(
      <DecisionRow
        testID="row"
        kind="reject"
        status="Sent"
        target={[{ text: "Mål" }]}
        source={[{ text: "Source" }]}
        reasons={["meaning", "tone"]}
        note="Says the opposite."
        canUndo={false}
        onUndo={() => {}}
      />,
    );
    expect(screen.getByText("Meaning")).toBeTruthy();
    expect(screen.getByText("Tone")).toBeTruthy();
    expect(screen.queryByText("meaning")).toBeNull();
    expect(screen.getByTestId("row-note").props.children).toBe("Says the opposite.");
  });
});

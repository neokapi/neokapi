import { codeDisplays, type Run } from "@neokapi/kapi-format";
import { fireEvent, render, screen } from "@testing-library/react-native";
import { Alert } from "react-native";

import "../lib/i18n";
import { RunText } from "../components/RunText";

describe("code chips in a string", () => {
  it("say what a variable is when tapped", async () => {
    const alert = jest.spyOn(Alert, "alert").mockImplementation(() => {});
    const ph = { id: "1", type: "variable", data: "{count}", equiv: "{count}" };
    const runs: Run[] = [{ ph }, { text: " angitt" }];
    expect(codeDisplays(runs).ph(ph).name).toBe("count");
    await render(<RunText runs={runs} />);
    await fireEvent.press(screen.getByTestId("run-chip"));
    expect(alert).toHaveBeenCalledWith("count", expect.stringContaining("fills in when it runs"));
  });
});

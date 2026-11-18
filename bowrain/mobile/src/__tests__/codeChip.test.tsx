import { codeDisplays, type Run } from "@neokapi/kapi-format";
import { render, screen } from "@testing-library/react-native";

import "../lib/i18n";
import { RunText } from "../components/RunText";

describe("codes in a string", () => {
  it("read as a muted monospace name and do not respond to a tap", async () => {
    const ph = { id: "1", type: "variable", data: "{serverURL}", equiv: "{serverURL}" };
    const runs: Run[] = [{ text: "Bowrain må nå " }, { ph }, { text: " for å laste" }];
    expect(codeDisplays(runs).ph(ph).name).toBe("serverURL");
    await render(<RunText runs={runs} />);
    const code = screen.getByTestId("run-chip");
    expect(code.props.children).toBe("serverURL");
    expect(code.props.onPress).toBeUndefined();
    expect(code.props.className).toContain("font-mono");
    expect(code.props.className).toContain("text-muted-foreground");
  });
});

import type { Run } from "@neokapi/kapi-format";
import { render, screen } from "@testing-library/react-native";

import { RunText } from "../components/RunText";

const name: Run = { ph: { id: "1", type: "x-var", data: "{{name}}", equiv: "{{name}}" } };

describe("RunText", () => {
  it("draws a {{name}} placeholder as a chip, never as plain text", async () => {
    await render(<RunText runs={[{ text: "Hello, " }, name, { text: "!" }]} testID="runs" />);
    const chips = screen.getAllByTestId("run-chip");
    expect(chips).toHaveLength(1);
    expect(chips[0]).toHaveTextContent("{{name}}");
    expect(screen.getByTestId("runs")).toHaveTextContent("Hello, {{name}}!");
  });

  it("draws paired codes and every plural form", async () => {
    const runs: Run[] = [
      { pcOpen: { id: "b", type: "bold", data: "<b>", equiv: "b" } },
      { text: "Saved" },
      { pcClose: { id: "b", type: "bold", data: "</b>" } },
      { text: " " },
      {
        plural: {
          pivot: "count",
          forms: { one: [{ text: "one file" }], other: [{ text: "many files" }] },
        },
      },
    ];
    await render(<RunText runs={runs} testID="runs" />);
    expect(screen.getAllByTestId("run-chip")).toHaveLength(3);
    expect(screen.getByTestId("runs")).toHaveTextContent(/one: one file/);
    expect(screen.getByTestId("runs")).toHaveTextContent(/other: many files/);
  });
});

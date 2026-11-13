import type { Run } from "@neokapi/kapi-format";
import { render, screen } from "@testing-library/react-native";

import { toSegments } from "../components/RunEditor";
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

  it("labels a paired code's two halves with one name, and placeholders in the same notation", async () => {
    // A link as the markdown reader writes it: a bracketed display label on
    // the opening half, nothing but the id on the closing half.
    const runs: Run[] = [
      { text: "Read " },
      { pcOpen: { id: "1", type: "link:hyperlink", data: "[", equiv: "", disp: "[l]" } },
      { text: "the guide" },
      { pcClose: { id: "1", type: "link:hyperlink", data: "](u)" } },
      { ph: { id: "2", type: "struct:break", data: "<br/>", equiv: "\n", disp: "[br]" } },
      { pcOpen: { id: "3", type: "fmt:bold", data: "<b>", equiv: "b", disp: "[B]" } },
      { text: "now" },
      { pcClose: { id: "3", type: "fmt:bold", data: "</b>", equiv: "b" } },
    ];
    await render(<RunText runs={runs} testID="runs" />);
    const labels = screen.getAllByTestId("run-chip").map((c) => c.props.accessibilityLabel);
    expect(labels).toEqual(["<l>", "</l>", "<br/>", "<B>", "</B>"]);

    const locked = toSegments(runs).flatMap((s) => (s.kind === "locked" ? [s.label] : []));
    expect(locked).toEqual(labels);
  });
});

import type { Run } from "@neokapi/kapi-format";
import { render, screen } from "@testing-library/react-native";

import "../lib/i18n";
import { toSegments } from "../components/RunEditor";
import { RunText } from "../components/RunText";

const name: Run = { ph: { id: "1", type: "x-var", data: "{{name}}", equiv: "{{name}}" } };

/** Characters that only markup or a code's native data would put on screen. */
const CODED = /[<>{}=]/;

describe("RunText", () => {
  it("draws a {{name}} placeholder as a chip with the variable's name", async () => {
    await render(<RunText runs={[{ text: "Hello, " }, name, { text: "!" }]} testID="runs" />);
    const chips = screen.getAllByTestId("run-chip");
    expect(chips).toHaveLength(1);
    expect(chips[0]).toHaveTextContent("name");
    expect(screen.getByTestId("runs")).toHaveTextContent("Hello, name!");
  });

  it("draws a paired code as styled text with no tags, and every plural form", async () => {
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
    expect(screen.getAllByTestId("run-chip")).toHaveLength(1);
    expect(screen.getByTestId("runs")).toHaveTextContent(/^Saved count/);
    expect(screen.getByTestId("runs")).toHaveTextContent(/one: one file/);
    expect(screen.getByTestId("runs")).toHaveTextContent(/other: many files/);
  });

  it("never shows coded text: the React extractor's <span>{serverURL}</span>", async () => {
    const runs: Run[] = [
      { text: "Bowrain needs to reach " },
      {
        pcOpen: {
          id: "0",
          type: "jsx:element",
          subType: "span",
          data: '<span className="x">',
          equiv: "=m0",
        },
      },
      { ph: { id: "1", type: "jsx:var", data: "{serverURL}", equiv: "serverURL" } },
      { pcClose: { id: "0", type: "jsx:element", subType: "span", data: "</span>", equiv: "=m0" } },
      { text: " to load your workspace." },
      { ph: { id: "2", type: "jsx:node", data: "{open && <Link/>}", equiv: "=m2" } },
    ];
    await render(<RunText runs={runs} testID="runs" />);
    expect(screen.getByTestId("runs")).toHaveTextContent(
      "Bowrain needs to reach serverURL to load your workspace.content",
    );
    expect(screen.getByTestId("runs")).not.toHaveTextContent(CODED);
  });

  it("locks the same codes in the editor, drawn the same way", async () => {
    const runs: Run[] = [
      { text: "Read " },
      { pcOpen: { id: "1", type: "link:hyperlink", data: "[", equiv: "", disp: "[l]" } },
      { text: "the guide" },
      { pcClose: { id: "1", type: "link:hyperlink", data: "](u)" } },
      { ph: { id: "2", type: "struct:break", data: "<br/>", equiv: "\n", disp: "[br]" } },
    ];
    const locked = toSegments(runs).flatMap((s) => (s.kind === "locked" ? [s.display] : []));
    expect(locked.map((d) => [d.role, d.kind, d.name])).toEqual([
      ["open", "link", "l"],
      ["close", "link", "l"],
      ["placeholder", "break", undefined],
    ]);
  });
});

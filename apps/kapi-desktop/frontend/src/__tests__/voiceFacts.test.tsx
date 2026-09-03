import { render, screen } from "./testUtils";
import { describe, it, expect } from "vitest";
import { FactGrid } from "../components/voice/facts";

describe("voice fact grid", () => {
  it("renders each fact that has a value", () => {
    render(
      <FactGrid
        facts={[
          { label: "Formality", value: "neutral" },
          { label: "Emotion", value: "measured" },
        ]}
      />,
    );
    expect(screen.getByText("Formality")).toBeInTheDocument();
    expect(screen.getByText("neutral")).toBeInTheDocument();
    expect(screen.getByText("Emotion")).toBeInTheDocument();
    expect(screen.getAllByTestId("voice-fact")).toHaveLength(2);
  });

  it("skips a fact with no value", () => {
    render(
      <FactGrid
        facts={[
          { label: "Formality", value: "informal" },
          { label: "Emotion", value: undefined },
        ]}
      />,
    );
    expect(screen.getByText("Formality")).toBeInTheDocument();
    expect(screen.queryByText("Emotion")).not.toBeInTheDocument();
    expect(screen.getAllByTestId("voice-fact")).toHaveLength(1);
  });

  it("renders nothing when no fact has a value", () => {
    render(<FactGrid facts={[{ label: "Formality", value: undefined }]} />);
    expect(screen.queryByTestId("voice-fact-grid")).not.toBeInTheDocument();
  });
});

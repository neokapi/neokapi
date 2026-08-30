import { render, screen } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { CollectionPointBadge, CollectionPointFields } from "../components/CollectionPointFields";
import type { Collection } from "../types/api";

const channels = ["campaign/promo", "support/docs"];

describe("CollectionPointFields", () => {
  it("offers the declared channel references", async () => {
    const onChange = vi.fn();
    render(
      <CollectionPointFields
        coll={{ name: "Docs" }}
        channels={channels}
        declarableAxes={["brand", "mode"]}
        onChange={onChange}
      />,
    );
    await userEvent.click(screen.getByLabelText("Channel"));
    await userEvent.click(screen.getByRole("option", { name: "support/docs" }));
    expect(onChange).toHaveBeenCalledWith({ channel: "support/docs" });
  });

  it("clears the channel back to the project's own point", async () => {
    const onChange = vi.fn();
    render(
      <CollectionPointFields
        coll={{ name: "Docs", channel: "support/docs" }}
        channels={channels}
        declarableAxes={[]}
        onChange={onChange}
      />,
    );
    await userEvent.click(screen.getByLabelText("Channel"));
    await userEvent.click(screen.getByRole("option", { name: "The project's own point" }));
    expect(onChange).toHaveBeenCalledWith({ channel: undefined });
  });

  it("says so when the project declares no profiles", () => {
    render(
      <CollectionPointFields
        coll={{ name: "Docs" }}
        channels={[]}
        declarableAxes={[]}
        onChange={vi.fn()}
      />,
    );
    expect(
      screen.getByText("This project declares no profiles, so its content sits at one point."),
    ).toBeInTheDocument();
  });

  it("adds and removes a declared axis", async () => {
    const onChange = vi.fn();
    const { rerender } = render(
      <CollectionPointFields
        coll={{ name: "Docs" }}
        channels={channels}
        declarableAxes={["brand", "mode"]}
        onChange={onChange}
      />,
    );
    await userEvent.click(screen.getByLabelText("New axis"));
    await userEvent.click(screen.getByRole("option", { name: "mode" }));
    await userEvent.click(screen.getByRole("button", { name: /Add axis/ }));
    expect(onChange).toHaveBeenCalledWith({ coordinates: { mode: "" } });

    rerender(
      <CollectionPointFields
        coll={{ name: "Docs", coordinates: { mode: "reference" } }}
        channels={channels}
        declarableAxes={["brand", "mode"]}
        onChange={onChange}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "Remove mode" }));
    // Dropping the last axis drops the key, so the recipe carries no empty map.
    expect(onChange).toHaveBeenLastCalledWith({ coordinates: undefined });
  });
});

describe("CollectionPointBadge", () => {
  it("shows the channel a collection names", () => {
    render(<CollectionPointBadge coll={{ name: "Docs", channel: "support/docs" }} />);
    expect(screen.getByTestId("collection-point-badge")).toHaveTextContent("support/docs");
  });

  it("shows the inherited axes when no channel is named", () => {
    render(<CollectionPointBadge coll={{ name: "App" }} defaults={{ brand: "northsea" }} />);
    expect(screen.getByTestId("collection-point-badge")).toHaveTextContent("brand:northsea");
  });

  it("stays quiet when the collection sits nowhere in particular", () => {
    const coll: Collection = { name: "App" };
    render(<CollectionPointBadge coll={coll} />);
    expect(screen.queryByTestId("collection-point-badge")).not.toBeInTheDocument();
  });
});

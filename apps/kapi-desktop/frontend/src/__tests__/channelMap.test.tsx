import { render, screen, within } from "./testUtils";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import type { ChannelMapRow } from "../types/api";
import { ChannelMap, ChannelRow } from "../components/channels/ChannelMap";

const declared: ChannelMapRow = {
  ref: "campaign/promo",
  profile: "campaign",
  channel: "promo",
  declared: true,
  voice: "Northsea",
  collections: ["Promo"],
  item_count: 142,
};

function rowFixture(over: Partial<ChannelMapRow>): ChannelMapRow {
  return { ...declared, ...over };
}

describe("ChannelRow", () => {
  it("shows the channel, its item count and the governing voice", () => {
    render(
      <ul>
        <ChannelRow channel={declared} onRename={() => {}} />
      </ul>,
    );
    const row = screen.getByTestId("channel-row");
    expect(row).toHaveTextContent("promo");
    expect(row).toHaveTextContent("campaign");
    expect(within(row).getByTestId("channel-items")).toHaveTextContent("142");
    expect(within(row).getByTestId("channel-voice")).toHaveTextContent("Northsea");
  });

  it("says so when no voice profile binds", () => {
    render(
      <ul>
        <ChannelRow channel={rowFixture({ voice: undefined })} onRename={() => {}} />
      </ul>,
    );
    expect(screen.getByText("No voice profile")).toBeInTheDocument();
    expect(screen.queryByTestId("channel-voice")).not.toBeInTheDocument();
  });

  it("renames a declared channel", async () => {
    const onRename = vi.fn();
    render(
      <ul>
        <ChannelRow channel={declared} onRename={onRename} />
      </ul>,
    );
    await userEvent.click(screen.getByRole("button", { name: /Rename promo/ }));
    const input = screen.getByLabelText("New channel name");
    await userEvent.clear(input);
    await userEvent.type(input, "email");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onRename).toHaveBeenCalledWith("email");
  });

  it("marks a derived channel read-only, with no rename", () => {
    render(
      <ul>
        <ChannelRow channel={rowFixture({ ref: "blog/news", channel: "news", declared: false })} />
      </ul>,
    );
    expect(screen.getByTestId("channel-derived")).toHaveTextContent("declared by collections");
    expect(screen.queryByRole("button", { name: /Rename/ })).not.toBeInTheDocument();
  });
});

describe("ChannelMap", () => {
  it("lists the channels it is given", () => {
    render(
      <ChannelMap
        tabID="t1"
        channels={[
          declared,
          rowFixture({ ref: "support/docs", profile: "support", channel: "docs" }),
        ]}
      />,
    );
    expect(screen.getAllByTestId("channel-row")).toHaveLength(2);
  });

  it("points to the project's own point when there are no channels", () => {
    render(<ChannelMap tabID="t1" channels={[]} />);
    expect(screen.getByText(/No channels yet/)).toBeInTheDocument();
    expect(screen.queryByTestId("channel-row")).not.toBeInTheDocument();
  });
});

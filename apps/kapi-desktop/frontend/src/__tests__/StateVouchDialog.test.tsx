import { describe, it, expect, vi } from "vite-plus/test";
import { render, fireEvent } from "./testUtils";

import { StateVouchDialog } from "../components/StateVouchDialog";

const STATE = {
  path: "/work/app/kapi.yaml",
  entries: [".kapi/vault", ".kapi/sync"],
  origin: "host laptop, /srv/checkouts/app, created 2026-11-01T10:00:00Z",
};

describe("StateVouchDialog", () => {
  it("shows the state and where it appears to come from", () => {
    const { getByText } = render(
      <StateVouchDialog state={STATE} onVouch={vi.fn()} onCancel={vi.fn()} />,
    );
    expect(getByText(".kapi/vault")).toBeTruthy();
    expect(getByText(".kapi/sync")).toBeTruthy();
    expect(getByText(/It appears to come from "host laptop, \/srv\/checkouts\/app/)).toBeTruthy();
  });

  it("vouches only when the person chooses to", () => {
    const onVouch = vi.fn();
    const onCancel = vi.fn();
    const { getByText } = render(
      <StateVouchDialog state={STATE} onVouch={onVouch} onCancel={onCancel} />,
    );
    fireEvent.click(getByText("Cancel"));
    expect(onCancel).toHaveBeenCalledOnce();
    expect(onVouch).not.toHaveBeenCalled();
    fireEvent.click(getByText("Use as this machine's own"));
    expect(onVouch).toHaveBeenCalledOnce();
  });

  it("says so when nothing tells where the state comes from", () => {
    const { getByText } = render(
      <StateVouchDialog state={{ ...STATE, origin: "" }} onVouch={vi.fn()} onCancel={vi.fn()} />,
    );
    expect(getByText("Nothing in it says where it comes from.")).toBeTruthy();
  });
});

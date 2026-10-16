import { afterEach, describe, expect, it, vi } from "vite-plus/test";
import userEvent from "@testing-library/user-event";
import { render, screen } from "./testUtils";

import { ShareSetupDialog, missingField, shareSpec } from "../components/ShareSetupDialog";
import { api } from "../hooks/useApi";
import { GIT_IN_SYNC } from "../stories/fixtures/sharing";

const slot = (name: string) => document.querySelector(`[data-slot='${name}']`);

afterEach(() => {
  vi.restoreAllMocks();
});

describe("the context block the dialog writes", () => {
  it("leaves out every field the person left empty", () => {
    const empty = {
      ref: "",
      remote: " ",
      path: "",
      bucket: "",
      prefix: "",
      region: "",
      endpoint: "",
    };
    expect(shareSpec("git", empty)).toEqual({ backend: "git" });
    expect(shareSpec("s3", { ...empty, bucket: "acme-kapi", prefix: "fernwell/" })).toEqual({
      backend: "s3",
      bucket: "acme-kapi",
      prefix: "fernwell/",
    });
  });

  it("asks a folder for a path and S3 for a bucket", () => {
    const empty = {
      ref: "",
      remote: "",
      path: "",
      bucket: "",
      prefix: "",
      region: "",
      endpoint: "",
    };
    expect(missingField("git", empty)).toBeNull();
    expect(missingField("file", empty)).toBe("path");
    expect(missingField("s3", empty)).toBe("bucket");
    expect(missingField("file", { ...empty, path: "/Volumes/team" })).toBeNull();
  });
});

describe("the Set up sharing dialog", () => {
  it("offers git, a folder and S3, with git's defaults as placeholders", () => {
    render(<ShareSetupDialog tabID="t1" open onClose={vi.fn()} />);
    expect(screen.getByLabelText("Git")).toBeChecked();
    expect(screen.getByLabelText("A folder")).not.toBeChecked();
    expect(screen.getByLabelText("S3")).not.toBeChecked();
    expect(screen.getByText(/a ref outside every branch; whoever can push a branch/)).toBeVisible();
    expect(screen.getByLabelText("Ref")).toHaveAttribute("placeholder", "refs/kapi/context");
    expect(screen.getByLabelText("Remote")).toHaveAttribute("placeholder", "origin");
  });

  it("shows the fields of the kind the person chooses", async () => {
    render(<ShareSetupDialog tabID="t1" open onClose={vi.fn()} />);
    await userEvent.click(screen.getByLabelText("S3"));
    expect(screen.getByLabelText("Bucket")).toBeInTheDocument();
    expect(screen.getByLabelText("Prefix")).toBeInTheDocument();
    expect(screen.getByLabelText("Region")).toBeInTheDocument();
    expect(screen.getByLabelText("Endpoint")).toBeInTheDocument();
    expect(screen.queryByLabelText("Ref")).toBeNull();

    await userEvent.click(screen.getByLabelText("A folder"));
    expect(screen.getByLabelText("Path")).toBeInTheDocument();
    expect(screen.queryByLabelText("Bucket")).toBeNull();
  });

  it("refuses a folder with no path and S3 with no bucket", async () => {
    const save = vi.spyOn(api, "setProjectSharing");
    render(<ShareSetupDialog tabID="t1" open onClose={vi.fn()} />);

    await userEvent.click(screen.getByLabelText("A folder"));
    await userEvent.click(screen.getByRole("button", { name: "Save to kapi.yaml" }));
    expect(slot("share-setup-missing")?.textContent).toBe("A folder needs a path.");

    await userEvent.click(screen.getByLabelText("S3"));
    await userEvent.click(screen.getByRole("button", { name: "Save to kapi.yaml" }));
    expect(slot("share-setup-missing")?.textContent).toBe("S3 needs a bucket.");
    expect(save).not.toHaveBeenCalled();
  });

  it("saves the block to kapi.yaml, then offers Sync now and closes", async () => {
    const save = vi.spyOn(api, "setProjectSharing").mockResolvedValue(GIT_IN_SYNC);
    const onSaved = vi.fn();
    const onSync = vi.fn();
    const onClose = vi.fn();
    render(
      <ShareSetupDialog tabID="t1" open onClose={onClose} onSaved={onSaved} onSync={onSync} />,
    );

    await userEvent.type(screen.getByLabelText("Remote"), "upstream");
    await userEvent.click(screen.getByRole("button", { name: "Save to kapi.yaml" }));

    expect(save).toHaveBeenCalledWith("t1", { backend: "git", remote: "upstream" });
    expect(onSaved).toHaveBeenCalledWith(GIT_IN_SYNC);
    expect(await screen.findByText(/Saved to/)).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Sync now" }));
    expect(onSync).toHaveBeenCalledTimes(1);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("keeps the form and shows why when kapi.yaml cannot be saved", async () => {
    vi.spyOn(api, "setProjectSharing").mockRejectedValue(
      new Error("context.path: applies to the file backend, and this recipe names git"),
    );
    render(<ShareSetupDialog tabID="t1" open onClose={vi.fn()} />);
    await userEvent.click(screen.getByRole("button", { name: "Save to kapi.yaml" }));
    expect(await screen.findByText("kapi.yaml could not be saved")).toBeInTheDocument();
    expect(
      screen.getByText("context.path: applies to the file backend, and this recipe names git"),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save to kapi.yaml" })).toBeEnabled();
  });

  it("closes on Escape", async () => {
    const onClose = vi.fn();
    render(<ShareSetupDialog tabID="t1" open onClose={onClose} />);
    await userEvent.keyboard("{Escape}");
    expect(onClose).toHaveBeenCalled();
  });
});

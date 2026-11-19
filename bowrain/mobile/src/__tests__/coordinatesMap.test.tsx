import { coordinatesFromServer, type ReviewCoordinates } from "@neokapi/bowrain-client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react-native";
import type { ReactNode } from "react";
import { Alert } from "react-native";

import "../lib/i18n";
import CoordinatesScreen from "../app/[ws]/[project]/coordinates";

jest.mock("expo-router", () => ({
  router: { push: jest.fn() },
  Stack: { Screen: () => null },
  useLocalSearchParams: () => ({ ws: "ws", project: "p" }),
}));

const mockMap = jest.fn<Promise<ReviewCoordinates>, unknown[]>();
const mockConfirm = jest.fn<Promise<void>, unknown[]>();
const mockPermissions = jest.fn();
jest.mock("../lib/auth", () => ({
  useAuth: () => ({
    client: {
      reviewCoordinates: (...a: unknown[]) => mockMap(...a),
      confirmCoordinates: (...a: unknown[]) => mockConfirm(...a),
      permissions: (...a: unknown[]) => mockPermissions(...a),
    },
  }),
}));

const map = (changed: boolean) =>
  coordinatesFromServer({
    stream: "main",
    coordinates: {
      source_language: "en",
      target_languages: ["nb"],
      confirmed: true,
      confirmed_by: "u-ada",
      confirmed_by_name: "Ada",
      changes: changed
        ? [
            {
              kind: "point_added",
              collection: "neokapi-mobile",
              coordinates: { product: "neokapi", channel: "mobile" },
            },
            {
              kind: "point_removed",
              collection: "neokapi-demos",
              coordinates: { product: "neokapi", channel: "demos" },
            },
          ]
        : [],
      changed: changed ? 2 : 0,
    },
    cells: [
      {
        product: "neokapi",
        channel: "docs",
        collections: [
          {
            name: "neokapi-docs",
            coordinates: { product: "neokapi", channel: "docs" },
            voice: "neokapi documentation",
            blocks: 18420,
            coverage: { nb: 0.4 },
          },
        ],
        blocks: 18420,
        voices: ["neokapi documentation"],
        coverage: { nb: 0.4 },
      },
      {
        product: "neokapi",
        channel: "mobile",
        collections: [{ name: "neokapi-mobile", blocks: 412 }],
        blocks: 412,
        voices: [],
        coverage: { nb: 0 },
        changes: changed ? ["point_added"] : [],
      },
      {
        product: "bowrain",
        channel: "app",
        collections: [{ name: "bowrain-app", blocks: 3576 }],
        blocks: 3576,
        voices: ["bowrain platform"],
        coverage: { nb: 1 },
      },
    ],
  });

function wrap(children: ReactNode) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

describe("coordinates map", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockPermissions.mockResolvedValue({ permissions: ["review", "manage_project"] });
  });

  it("lists what changed as chips and draws the cells as tiles", async () => {
    mockMap.mockResolvedValue(map(true));
    await render(wrap(<CoordinatesScreen />));
    expect(await screen.findByText("Since Ada confirmed")).toBeTruthy();
    expect(screen.getByText("New: neokapi / mobile")).toBeTruthy();
    expect(screen.getByText("Removed: neokapi / demos")).toBeTruthy();
    expect(screen.getByTestId("coordinates-removed")).toBeTruthy();
    expect(screen.getByText("neokapi / demos · neokapi-demos")).toBeTruthy();
    expect(screen.getByLabelText("neokapi docs, 18420 blocks, 40% established")).toBeTruthy();
    expect(screen.getByLabelText("neokapi mobile, 412 blocks, 0% established, New")).toBeTruthy();
    expect(screen.getByLabelText("bowrain app, 3576 blocks, 100% established")).toBeTruthy();
    expect(screen.getByText("Shade: share established in Norwegian Bokmål")).toBeTruthy();
  });

  it("zooms into a product, opens a tile's collections, and zooms out", async () => {
    mockMap.mockResolvedValue(map(false));
    await render(wrap(<CoordinatesScreen />));
    // Zoomed out, a tile zooms into its product rather than opening.
    await fireEvent.press(await screen.findByTestId("coordinates-tile-neokapi-docs"));
    expect(screen.queryByTestId("coordinates-cell-detail")).toBeNull();
    expect(screen.getByTestId("coordinates-zoom-out")).toBeTruthy();
    expect(screen.queryByTestId("coordinates-tile-bowrain-app")).toBeNull();
    expect(screen.getByLabelText("docs, 18420 blocks, 40% established")).toBeTruthy();

    await fireEvent.press(screen.getByTestId("coordinates-tile-neokapi-docs"));
    expect(screen.getByTestId("coordinates-cell-detail")).toBeTruthy();
    expect(screen.getByText("neokapi-docs")).toBeTruthy();
    expect(screen.getByText("40%")).toBeTruthy();
    await fireEvent.press(screen.getByTestId("coordinates-cell-close"));
    expect(screen.queryByTestId("coordinates-cell-detail")).toBeNull();

    await fireEvent.press(screen.getByTestId("coordinates-zoom-out"));
    expect(screen.getByTestId("coordinates-tile-bowrain-app")).toBeTruthy();
    await fireEvent.press(screen.getByTestId("coordinates-product-bowrain"));
    expect(screen.queryByTestId("coordinates-tile-neokapi-docs")).toBeNull();
  });

  it("confirms the changes after asking, and offers nothing when settled", async () => {
    mockMap.mockResolvedValue(map(true));
    mockConfirm.mockResolvedValue();
    const alert = jest.spyOn(Alert, "alert").mockImplementation((_t, _m, buttons) => {
      buttons?.find((b) => b.style !== "cancel")?.onPress?.();
    });
    await render(wrap(<CoordinatesScreen />));
    await fireEvent.press(await screen.findByTestId("coordinates-confirm"));
    expect(alert).toHaveBeenCalled();
    expect(mockConfirm).toHaveBeenCalledWith({ workspace: "ws", project: "p" });

    mockMap.mockResolvedValue(map(false));
    await render(wrap(<CoordinatesScreen />));
    expect(await screen.findByText("Nothing changed since the last confirmation.")).toBeTruthy();
    expect(screen.queryByTestId("coordinates-confirm")).toBeNull();
  });
});

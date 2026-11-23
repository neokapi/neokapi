// At the largest text sizes the server sheet's rows are taller than the sheet,
// so its content scrolls: every option, the address field and its confirm
// button sit inside the sheet's scroll view and stay reachable.
import { fireEvent, render, screen } from "@testing-library/react-native";
import { ScrollView } from "react-native";

import "../lib/i18n";
import Server from "../app/server";

const mockScale = { current: 1 };
jest.mock("react-native/Libraries/Utilities/useWindowDimensions", () => ({
  __esModule: true,
  default: () => ({ width: 402, height: 874, scale: 3, fontScale: mockScale.current }),
}));

jest.mock("expo-router", () => ({ router: { back: jest.fn() } }));
jest.mock("../lib/auth", () => ({ useAuth: () => ({ session: null }) }));

/** Whether `testID`'s element sits inside the sheet's scroll view. */
function inSheetScroll(testID: string): boolean {
  const sheet = screen.getByTestId("server-sheet");
  for (let node = screen.getByTestId(testID).parent; node; node = node.parent) {
    if (node === sheet || (node.type === ScrollView && node.props.testID === "server-sheet")) {
      return true;
    }
  }
  return false;
}

describe("the server sheet at the largest text size", () => {
  beforeEach(() => {
    mockScale.current = 3.1;
  });

  it("puts every option inside the sheet's scroll view", async () => {
    await render(<Server />);
    for (const id of ["server-option-cloud", "server-option-local", "server-option-custom"]) {
      expect(inSheetScroll(id)).toBe(true);
    }
  });

  it("keeps the address field and its confirm button inside it", async () => {
    await render(<Server />);
    await fireEvent.press(screen.getByTestId("server-option-custom"));
    await screen.findByTestId("server-url");
    expect(inSheetScroll("server-url")).toBe(true);
    expect(inSheetScroll("server-save")).toBe(true);
  });

  it("keeps the keyboard open when a row inside the sheet is tapped", async () => {
    await render(<Server />);
    expect(screen.getByTestId("server-sheet").props.keyboardShouldPersistTaps).toBe("handled");
  });
});

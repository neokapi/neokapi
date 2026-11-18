// Test doubles for the native modules the units under test reach. The server
// is never mocked here: the tests that need one pass a client with a fetch of
// their own.
const mockStore = new Map<string, string>();

jest.mock("expo-secure-store", () => ({
  getItemAsync: async (k: string) => mockStore.get(`secure:${k}`) ?? null,
  setItemAsync: async (k: string, v: string) => void mockStore.set(`secure:${k}`, v),
  deleteItemAsync: async (k: string) => void mockStore.delete(`secure:${k}`),
}));

jest.mock("expo-sqlite/kv-store", () => ({
  __esModule: true,
  default: {
    getItem: async (k: string) => mockStore.get(k) ?? null,
    setItem: async (k: string, v: string) => void mockStore.set(k, v),
    removeItem: async (k: string) => void mockStore.delete(k),
  },
}));

jest.mock("expo-haptics", () => ({
  notificationAsync: async () => undefined,
  impactAsync: async () => undefined,
  selectionAsync: async () => undefined,
  NotificationFeedbackType: { Success: "success" },
  ImpactFeedbackStyle: { Medium: "medium" },
}));

jest.mock("expo-network", () => ({
  addNetworkStateListener: () => ({ remove: () => undefined }),
  getNetworkStateAsync: async () => ({ isConnected: true, isInternetReachable: true }),
}));

jest.mock("expo-localization", () => ({
  getLocales: () => [{ languageCode: "en" }],
}));

// The WebView is a native view; tests read the props it was given.
jest.mock("react-native-webview", () => {
  const React = require("react");
  const { View } = require("react-native");
  const WebView = React.forwardRef((props: Record<string, unknown>, ref: unknown) => {
    React.useImperativeHandle(ref, () => ({ injectJavaScript: jest.fn() }));
    return React.createElement(View, {
      testID: props.testID,
      accessibilityLabel: JSON.stringify((props.source as { uri?: string })?.uri ?? ""),
    });
  });
  return { WebView };
});

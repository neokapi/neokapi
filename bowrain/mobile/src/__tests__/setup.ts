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

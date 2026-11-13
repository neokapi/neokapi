import * as SecureStore from "expo-secure-store";

import { defaultServerUrl } from "../lib/config";
import {
  currentServer,
  loadServerChoice,
  normalizeServerUrl,
  serverHost,
  setServerChoice,
} from "../lib/serverChoice";

describe("normalizeServerUrl", () => {
  it.each([
    ["https://app.bowrain.cloud", "https://app.bowrain.cloud"],
    ["  https://app.bowrain.cloud/  ", "https://app.bowrain.cloud"],
    ["bowrain.example.com", "https://bowrain.example.com"],
    ["http://localhost:8080", "http://localhost:8080"],
    ["HTTP://10.0.2.2:8080/", "http://10.0.2.2:8080"],
    ["https://example.com/bowrain/", "https://example.com/bowrain"],
  ])("reads %j as %j", (input, want) => {
    expect(normalizeServerUrl(input)).toBe(want);
  });

  it.each(["", "   ", "ftp://example.com", "bowrain://auth", "https://", "https://user@host"])(
    "refuses %j",
    (input) => {
      expect(normalizeServerUrl(input)).toBeNull();
    },
  );
});

describe("server choice", () => {
  it("shows a server by its host", () => {
    expect(serverHost("https://app.bowrain.cloud")).toBe("app.bowrain.cloud");
    expect(serverHost("http://localhost:8080/x")).toBe("localhost:8080");
  });

  it("keeps a chosen server across a restart and forgets it on reset", async () => {
    await setServerChoice("bowrain.example.com");
    expect(currentServer()).toBe("https://bowrain.example.com");
    expect(await SecureStore.getItemAsync("bowrain.server")).toBe("https://bowrain.example.com");

    await loadServerChoice();
    expect(currentServer()).toBe("https://bowrain.example.com");

    await setServerChoice(null);
    expect(currentServer()).toBe(defaultServerUrl);
    expect(await SecureStore.getItemAsync("bowrain.server")).toBeNull();
  });

  it("stores nothing when the choice is the default", async () => {
    await setServerChoice(defaultServerUrl);
    expect(currentServer()).toBe(defaultServerUrl);
    expect(await SecureStore.getItemAsync("bowrain.server")).toBeNull();
  });
});

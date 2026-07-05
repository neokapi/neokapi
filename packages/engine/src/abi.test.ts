// Tests for ABI feature detection and the capability probe against a mocked
// global surface.

import { afterEach, describe, expect, it, vi } from "vitest";
import { engineABI, hasEngineFunction, SUPPORTED_ENGINE_ABI } from "./abi.ts";
import { detectCapabilities } from "./capabilities.ts";

const NAMES = [
  "kapiEngineABI",
  "kapiRun",
  "labSegment",
  "__kapiPdfium",
  "kapiLocalGenerate",
  "kapiLocalNER",
  "kapiBrowserTranslate",
  "Translator",
] as const;

afterEach(() => {
  for (const name of NAMES) delete (globalThis as Record<string, unknown>)[name];
});

describe("engineABI", () => {
  it("returns null when no engine is booted (or on a pre-ABI build)", () => {
    expect(engineABI()).toBeNull();
  });

  it("parses the descriptor from the kapiEngineABI global", () => {
    globalThis.kapiEngineABI = () => ({
      abi: SUPPORTED_ENGINE_ABI,
      version: "1.2.0",
      functions: ["kapiRun", "kapiPreview"],
    });
    expect(engineABI()).toEqual({
      abi: 1,
      version: "1.2.0",
      functions: ["kapiRun", "kapiPreview"],
    });
  });
});

describe("hasEngineFunction", () => {
  it("uses the ABI descriptor when available", () => {
    globalThis.kapiEngineABI = () => ({ abi: 1, version: "dev", functions: ["kapiRun"] });
    // The descriptor is authoritative even when the global itself exists.
    globalThis.labSegment = vi.fn();
    expect(hasEngineFunction("kapiRun")).toBe(true);
    expect(hasEngineFunction("labSegment")).toBe(false);
  });

  it("falls back to probing the global on pre-ABI builds", () => {
    globalThis.kapiRun = vi.fn();
    expect(hasEngineFunction("kapiRun")).toBe(true);
    expect(hasEngineFunction("kapiPreview")).toBe(false);
  });
});

describe("detectCapabilities", () => {
  it("reports nothing on a bare page", () => {
    expect(detectCapabilities()).toEqual({
      pdf: false,
      localLLM: false,
      localNER: false,
      browserMT: false,
    });
  });

  it("mirrors the engine's per-capability checks", () => {
    globalThis.__kapiPdfium = {
      ready: Promise.resolve(),
      extract: () => Promise.resolve({ pages: [] }),
    };
    globalThis.kapiLocalGenerate = () => Promise.resolve("ok");
    globalThis.kapiLocalNER = () => Promise.resolve(`{"entities":[]}`);
    // Bridge alone is not enough for browser MT — the platform Translator API
    // must exist too (mirrors browser_mt.go's browserTranslatorAvailable).
    globalThis.kapiBrowserTranslate = () => Promise.resolve({ ok: true });
    expect(detectCapabilities().browserMT).toBe(false);
    (globalThis as Record<string, unknown>).Translator = {};

    expect(detectCapabilities()).toEqual({
      pdf: true,
      localLLM: true,
      localNER: true,
      browserMT: true,
    });
  });
});

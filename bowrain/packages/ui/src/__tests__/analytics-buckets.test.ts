/**
 * Boundary tests for the client-side analytics bucketing helpers — the TS
 * mirror of `bowrain/analytics/events_test.go`. The label tables here and in
 * the Go test must stay identical: the same property is emitted from both the
 * server and the web surface, and a divergent edge would split one metric into
 * two incomparable ones (and quietly corrupt the grant-sizing read at 200k).
 */
import { describe, it, expect } from "vite-plus/test";
import { countBucket, creditBucket, percentBucket, sharePercentBucket } from "../analytics-buckets";

describe("countBucket", () => {
  it("buckets unit counts on the Go boundaries", () => {
    expect(countBucket(-3)).toBe("0");
    expect(countBucket(0)).toBe("0");
    expect(countBucket(1)).toBe("1_10");
    expect(countBucket(10)).toBe("1_10");
    expect(countBucket(11)).toBe("11_100");
    expect(countBucket(100)).toBe("11_100");
    expect(countBucket(101)).toBe("101_1000");
    expect(countBucket(1000)).toBe("101_1000");
    expect(countBucket(1001)).toBe("gt_1000");
  });
});

describe("creditBucket", () => {
  it("buckets credit amounts on the Go boundaries", () => {
    expect(creditBucket(-1)).toBe("0");
    expect(creditBucket(0)).toBe("0");
    expect(creditBucket(1)).toBe("1_1k");
    expect(creditBucket(1_000)).toBe("1_1k");
    expect(creditBucket(1_001)).toBe("1k_10k");
    expect(creditBucket(10_000)).toBe("1k_10k");
    expect(creditBucket(10_001)).toBe("10k_50k");
    expect(creditBucket(50_000)).toBe("10k_50k");
    expect(creditBucket(50_001)).toBe("50k_100k");
    expect(creditBucket(100_000)).toBe("50k_100k");
    expect(creditBucket(100_001)).toBe("100k_200k");
    expect(creditBucket(200_001)).toBe("200k_500k");
    expect(creditBucket(500_000)).toBe("200k_500k");
    expect(creditBucket(500_001)).toBe("500k_1m");
    expect(creditBucket(1_000_000)).toBe("500k_1m");
    expect(creditBucket(1_000_001)).toBe("gt_1m");
  });

  it("puts the new-workspace grant exactly on a boundary", () => {
    // The whole point of the edge: everything at or under the 200K grant falls
    // in bands up to and including 100k_200k; anything above overflows it.
    expect(creditBucket(200_000)).toBe("100k_200k");
    expect(creditBucket(200_001)).toBe("200k_500k");
  });
});

describe("percentBucket", () => {
  it("bands percentages and clamps out-of-range input", () => {
    expect(percentBucket(-5)).toBe("0");
    expect(percentBucket(0)).toBe("0");
    expect(percentBucket(1)).toBe("1_25");
    expect(percentBucket(25)).toBe("1_25");
    expect(percentBucket(26)).toBe("26_50");
    expect(percentBucket(50)).toBe("26_50");
    expect(percentBucket(51)).toBe("51_75");
    expect(percentBucket(75)).toBe("51_75");
    expect(percentBucket(76)).toBe("76_99");
    expect(percentBucket(99)).toBe("76_99");
    expect(percentBucket(100)).toBe("100");
    expect(percentBucket(150)).toBe("100");
  });
});

describe("sharePercentBucket", () => {
  it("keeps the saturated bands meaning exactly none / exactly all", () => {
    expect(sharePercentBucket(0, 0)).toBe("0");
    expect(sharePercentBucket(5, 0)).toBe("0");
    expect(sharePercentBucket(0, 10)).toBe("0");
    expect(sharePercentBucket(10, 10)).toBe("100");
    expect(sharePercentBucket(11, 10)).toBe("100");
    expect(sharePercentBucket(1, 1000)).toBe("1_25");
    expect(sharePercentBucket(999, 1000)).toBe("76_99");
  });

  it("bands the TM share of the work", () => {
    expect(sharePercentBucket(25, 100)).toBe("1_25");
    expect(sharePercentBucket(1, 2)).toBe("26_50");
    expect(sharePercentBucket(3, 4)).toBe("51_75");
    expect(sharePercentBucket(9, 10)).toBe("76_99");
  });
});

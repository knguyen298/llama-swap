import { describe, expect, it } from "vitest";
import {
  axisLabels,
  countLabel,
  formatExact,
  rangeLabel,
  drawablePercentiles,
  formatRate,
  labeledEdges,
  percentileEntries,
  reportingLine,
  shares,
  tableRows,
  tailCaption,
  yTicks,
  niceMax,
  type SpeedDistribution,
  type SpeedSampleCounts,
} from "./speedChart";

function dist(overrides: Partial<SpeedDistribution> = {}): SpeedDistribution {
  return {
    stepsPerDecade: 12,
    edges: [10, 12.1153, 14.678, 17.7828, 21.5443],
    counts: [2, 10, 20, 30, 5, 3],
    below: 2,
    above: 3,
    window: { lo: 10, hi: 21.5443 },
    count: 70,
    min: 8,
    max: 40,
    minTokens: 2,
    maxRatePerSecond: 100000,
    p50: 17,
    p90: 20,
    p95: 21,
    p99: null,
    ...overrides,
  };
}

const samples: SpeedSampleCounts = { rows: 100, reported: 90, usable: 70, excluded: 20, missing: 10 };

describe("niceMax", () => {
  it("rounds an axis maximum up to a readable value", () => {
    expect(niceMax(14.2)).toBe(15);
    expect(niceMax(24.3)).toBe(25);
    expect(niceMax(8.4)).toBe(10);
    expect(niceMax(0.42)).toBe(0.5);
  });

  it("falls back to 1 for an empty or invalid axis", () => {
    expect(niceMax(0)).toBe(1);
    expect(niceMax(-3)).toBe(1);
    expect(niceMax(NaN)).toBe(1);
  });
});

describe("shares", () => {
  it("turns counts into percentages of the group", () => {
    expect(shares([0, 50, 50, 0], 100)).toEqual([0, 50, 50, 0]);
  });

  it("returns zeros when the group is empty", () => {
    expect(shares([1, 2], 0)).toEqual([0, 0]);
  });
});

describe("yTicks", () => {
  it("labels five ticks from zero to the axis maximum", () => {
    const ticks = yTicks(15);
    expect(ticks.map((t) => t.value)).toEqual(["0.0%", "3.8%", "7.5%", "11.3%", "15.0%"]);
    expect(ticks.map((t) => t.fraction)).toEqual([0, 0.25, 0.5, 0.75, 1]);
  });

  it("adds a decimal place only for a small maximum", () => {
    expect(yTicks(0.5)[1].value).toBe("0.13%");
  });
});

describe("labeledEdges", () => {
  it("keeps only decade edges", () => {
    const edges = [1, 1.212, 10, 12.12, 100, 1000];
    expect(labeledEdges(edges)).toEqual([1, 10, 100, 1000]);
  });
});

describe("formatRate", () => {
  it("shortens large values and keeps small ones exact", () => {
    expect(formatRate(0.5)).toBe("0.5");
    expect(formatRate(2.54)).toBe("2.5");
    expect(formatRate(40)).toBe("40");
    expect(formatRate(1500)).toBe("1.5k");
    expect(formatRate(15041)).toBe("15k");
    expect(formatRate(1_200_000)).toBe("1.2M");
  });
});

describe("axisLabels", () => {
  it("prefers decade edges", () => {
    expect(axisLabels([1, 2, 10, 20, 100])).toEqual([1, 10, 100]);
  });

  it("labels the ends and the middle when the window holds no decade", () => {
    expect(axisLabels([54, 57, 60, 63, 66])).toEqual([54, 60, 66]);
  });

  it("does not repeat an edge", () => {
    expect(axisLabels([54, 66])).toEqual([54, 66]);
  });
});

describe("formatExact", () => {
  it("keeps three significant digits and groups thousands", () => {
    expect(formatExact(2912.63)).toBe("2,910");
    expect(formatExact(3527.9)).toBe("3,530");
    expect(formatExact(93.3665)).toBe("93.4");
    expect(formatExact(1.1548)).toBe("1.15");
  });
});

describe("rangeLabel", () => {
  it("states the unit once, after the last value", () => {
    expect(rangeLabel(2912.63, 3527.9, "t/s")).toBe("2,910 to 3,530 t/s");
  });
});

describe("countLabel", () => {
  it("counts requests and agrees in number with the noun", () => {
    expect(countLabel(8428)).toBe("8,428 requests");
    expect(countLabel(1)).toBe("1 request");
  });
});

describe("tailCaption", () => {
  it("states count, share and direction for a clipped tail", () => {
    expect(tailCaption(91, 23819, 56.2, "below")).toBe("91 below 56 (0.38%) not drawn");
    expect(tailCaption(82, 23819, 14678, "above")).toBe("82 above 15k (0.34%) not drawn");
  });

  it("does not divide by zero when nothing was sampled", () => {
    expect(tailCaption(0, 0, 10, "below")).toBe("0 below 10 (0.00%) not drawn");
  });
});

describe("reportingLine", () => {
  it("names every bucket of requests, so missing data is not silent", () => {
    expect(reportingLine(samples, "generation rate", "fewer than 2 tokens")).toBe(
      "70 of 100 requests produced a usable generation rate. 10 reported no rate. " +
        "20 were excluded because they measured fewer than 2 tokens.",
    );
  });
});

describe("percentileEntries", () => {
  it("marks a null percentile as unsupported", () => {
    const entries = percentileEntries(dist());
    expect(entries.find((e) => e.key === "p99")).toEqual({ key: "p99", value: null, supported: false });
    expect(entries.filter((e) => e.supported).map((e) => e.key)).toEqual(["p50", "p90", "p95"]);
  });
});

describe("drawablePercentiles", () => {
  it("drops percentiles outside the plotted window instead of clamping them", () => {
    // The fixture window stops at 21.54, so p50, p90 and p95 sit inside it and
    // the null p99 is unsupported.
    expect(drawablePercentiles(dist()).map((e) => e.key)).toEqual(["p50", "p90", "p95"]);

    // A window that stops below p90 keeps only the median line.
    expect(drawablePercentiles(dist({ window: { lo: 10, hi: 19 } })).map((e) => e.key)).toEqual(["p50"]);

    // A wide window draws all four.
    const wide = drawablePercentiles(dist({ window: { lo: 8, hi: 60 }, p99: 30 }));
    expect(wide.map((e) => e.key)).toEqual(["p50", "p90", "p95", "p99"]);
  });

  it("gives every line a distinct dash pattern", () => {
    const dashes = drawablePercentiles(dist({ window: { lo: 8, hi: 60 } })).map((e) => e.dash);
    expect(new Set(dashes).size).toBe(dashes.length);
  });
});

describe("tableRows", () => {
  it("lists tails and non-empty buckets with exact counts and shares", () => {
    const rows = tableRows(dist());
    expect(rows[0]).toEqual({ lower: "under 10", upper: "10", count: 2, share: "2.86%" });
    expect(rows[rows.length - 1]).toEqual({ lower: "21.5", upper: "over 21.5", count: 3, share: "4.29%" });
    expect(rows).toHaveLength(6);
  });

  it("drops empty buckets so the table stays readable", () => {
    const rows = tableRows(dist({ counts: [0, 0, 70, 0, 0, 0], below: 0, above: 0 }));
    expect(rows).toHaveLength(1);
    expect(rows[0].count).toBe(70);
  });
});

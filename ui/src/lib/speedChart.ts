// Pure helpers for the Activity speed charts. Kept out of Svelte so the
// arithmetic is unit testable, like the other modules in this directory.
//
// The data contract lives in internal/store/activity.go: a SpeedDistribution
// holds geometric bucket edges (equal log width), a counts array where index 0
// is below the first edge and the last index is at or above the final edge, a
// plotted window, and percentiles that are null when the sample is too small.

export interface SpeedWindowData {
  lo: number;
  hi: number;
}

export interface SpeedDistribution {
  stepsPerDecade: number;
  edges: number[];
  counts: number[];
  below: number;
  above: number;
  window: SpeedWindowData;
  count: number;
  min: number;
  max: number;
  /** Usability rules the backend applied, restated for the exclusion note. */
  minTokens: number;
  maxRatePerSecond: number;
  p50: number | null;
  p90: number | null;
  p95: number | null;
  p99: number | null;
}

export interface SpeedSampleCounts {
  rows: number;
  reported: number;
  usable: number;
  excluded: number;
  missing: number;
}

const numberFormat = new Intl.NumberFormat();
const nf = (value: number) => numberFormat.format(value);

export type PercentileKey = "p50" | "p90" | "p95" | "p99";

export const PERCENTILE_KEYS: PercentileKey[] = ["p50", "p90", "p95", "p99"];

/**
 * Smallest value from a short list that is greater than or equal to x. Axis
 * maxima come from here, so tick labels stay readable (15% rather than 14.2%).
 */
export function niceMax(x: number): number {
  if (!(x > 0)) return 1;
  const exponent = Math.pow(10, Math.floor(Math.log10(x)));
  // A short list of mantissas. The caller adds headroom, so the tallest bar
  // always stops below the top gridline and leaves a hoverable band.
  for (const mantissa of [1, 1.2, 1.5, 1.8, 2, 2.5, 3, 4, 5, 6, 8, 10]) {
    if (mantissa * exponent >= x) return mantissa * exponent;
  }
  return 10 * exponent;
}

/** Share of the group that each bucket holds, as a percentage. */
export function shares(counts: number[], total: number): number[] {
  if (total <= 0) return counts.map(() => 0);
  return counts.map((n) => (n / total) * 100);
}

/** Y-axis ticks for a chart whose maximum share is yMax. */
export function yTicks(yMax: number): { fraction: number; value: string }[] {
  const digits = yMax < 5 ? 2 : 1;
  return [0, 0.25, 0.5, 0.75, 1].map((fraction) => ({
    fraction,
    value: (yMax * fraction).toFixed(digits) + "%",
  }));
}

/**
 * Bucket edges that get an x-axis label. Every bucket is the same width in log
 * space, so labelling only the decades keeps the axis readable while the grid
 * stays fine enough to show shape.
 */
export function labeledEdges(edges: number[]): number[] {
  return edges.filter((e) => Math.abs(Math.log10(e) - Math.round(Math.log10(e))) < 1e-9);
}

/**
 * Edges to print on the x axis. Decade edges are the useful labels, but a
 * single-model window such as 54 to 66 t/s holds no power of ten, so fall back
 * to the low end, the middle and the high end.
 */
export function axisLabels(edges: number[]): number[] {
  const decades = labeledEdges(edges);
  if (decades.length >= 2) return decades;
  const last = edges.length - 1;
  return [...new Set([edges[0], edges[Math.round(last / 2)], edges[last]])];
}

/**
 * Exact-enough label for a rate: three significant digits with digit groups, so
 * 2912.63 reads as "2,910". Chart labels and the table use this. Axis ticks use
 * formatRate, because they are space limited.
 */
export function formatExact(value: number): string {
  if (!Number.isFinite(value)) return "0";
  const rounded = Number(value.toPrecision(3));
  const digits = rounded < 10 ? 2 : rounded < 100 ? 1 : 0;
  return rounded.toLocaleString(undefined, { maximumFractionDigits: digits });
}

/**
 * Bucket range with the unit stated once, after the last value. That is the
 * SI and ISO 80000 convention for a range of quantities: "2,910 to 3,530 t/s",
 * not "2,910 t/s to 3,530 t/s".
 */
export function rangeLabel(lower: number, upper: number, unit: string): string {
  return `${formatExact(lower)} to ${formatExact(upper)} ${unit}`;
}

/** Second label line for a bucket. */
export function countLabel(count: number): string {
  return `${nf(count)} request${count === 1 ? "" : "s"}`;
}

/** Short label for a rate: 1, 2.5, 15k, 1.2M. Full values go in the table. */
export function formatRate(value: number): string {
  if (!Number.isFinite(value)) return "0";
  if (value >= 1e6) return (value / 1e6).toFixed(value >= 1e7 ? 0 : 1) + "M";
  if (value >= 1e4) return (value / 1e3).toFixed(0) + "k";
  if (value >= 1e3) return (value / 1e3).toFixed(1) + "k";
  if (value >= 10) return value.toFixed(0);
  if (value >= 1) return String(+value.toFixed(1));
  return String(+value.toFixed(2));
}

/**
 * Caption for samples that sit outside the plotted window. The count is always
 * stated with its share, because a clipped tail is the one thing a reader must
 * not have to guess about.
 */
export function tailCaption(
  count: number,
  total: number,
  edge: number,
  side: "below" | "above",
): string {
  const share = total > 0 ? ((count / total) * 100).toFixed(2) : "0.00";
  // No arrow glyph here. Unicode triangles render as boxes in some font stacks,
  // so the caller draws a path instead.
  return `${nf(count)} ${side} ${formatRate(edge)} (${share}%) not drawn`;
}

/** One sentence stating where the samples came from. */
export function reportingLine(
  samples: SpeedSampleCounts,
  label: string,
  rule: string,
): string {
  return (
    `${nf(samples.usable)} of ${nf(samples.rows)} requests produced a usable ${label}. ` +
    `${nf(samples.missing)} reported no rate. ${nf(samples.excluded)} were excluded because they measured ${rule}.`
  );
}

/**
 * Percentiles to draw or print, with the same support rule as the backend
 * (store.SpeedPercentileSupported): five samples must sit beyond the requested
 * percentile, so p50 needs 10 samples, p90 needs 50, p95 needs 100, p99 needs 500.
 */
export function percentileEntries(dist: SpeedDistribution): {
  key: PercentileKey;
  value: number | null;
  supported: boolean;
}[] {
  const values: Record<PercentileKey, number | null> = {
    p50: dist.p50,
    p90: dist.p90,
    p95: dist.p95,
    p99: dist.p99,
  };
  return PERCENTILE_KEYS.map((key) => ({
    key,
    value: values[key],
    supported: values[key] !== null && values[key] !== undefined,
  }));
}

/** Percentile lines to draw: only supported values inside the plotted window. */
export function drawablePercentiles(dist: SpeedDistribution) {
  const dash: Record<PercentileKey, string> = {
    p50: "6 3",
    p90: "4 2",
    p95: "2 2",
    p99: "12 3 2 3",
  };
  return percentileEntries(dist)
    .filter((entry) => entry.supported && entry.value! >= dist.window.lo && entry.value! <= dist.window.hi)
    .map((entry, index) => ({ ...entry, dash: dash[entry.key], index }));
}

/** Rows for the accessible bucket table: tails first and last, empties dropped. */
export function tableRows(dist: SpeedDistribution): {
  lower: string;
  upper: string;
  count: number;
  share: string;
}[] {
  const rows: { lower: string; upper: string; count: number; share: string }[] = [];
  const push = (lower: string, upper: string, count: number) => {
    if (!count) return;
    const share = dist.count > 0 ? ((count / dist.count) * 100).toFixed(2) + "%" : "0%";
    rows.push({ lower, upper, count, share });
  };
  push("under " + formatExact(dist.edges[0]), formatExact(dist.edges[0]), dist.below);
  for (let i = 0; i < dist.edges.length - 1; i++) {
    push(formatExact(dist.edges[i]), formatExact(dist.edges[i + 1]), dist.counts[i + 1] ?? 0);
  }
  const last = dist.edges[dist.edges.length - 1];
  push(formatExact(last), "over " + formatExact(last), dist.above);
  return rows;
}


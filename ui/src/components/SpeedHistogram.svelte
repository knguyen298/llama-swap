<script lang="ts">
  // Activity speed chart, variant 2c: geometric buckets of equal log width, a
  // share-of-group Y axis from zero, and a plotted window with stated tails.
  //
  // Only the text the original chart had is on screen: axis labels and tick
  // values. Hovering a bar shows that bar's count in one reserved row above the
  // plot. Hovering a percentile line shows that line's value in the same row,
  // and nothing else. The reporting sentence, the percentile list and the bucket
  // table are screen-reader text, so assistive technology still gets exact
  // numbers instead of bar tooltips.
  import {
    countLabel,
    formatExact,
    rangeLabel,
    drawablePercentiles,
    formatRate,
    axisLabels,
    percentileEntries,
    reportingLine,
    shares,
    tableRows,
    tailCaption,
    yTicks,
    niceMax,
    type SpeedDistribution,
    type SpeedSampleCounts,
  } from "../lib/speedChart";

  interface Props {
    dist: SpeedDistribution;
    samples: SpeedSampleCounts;
    title: string;
    /** Rate the backend applied, phrased to finish "measured ...". */
    rule: string;
    unit: string;
    /** Label for the rate column, used in the reporting sentence. */
    sampleLabel: string;
    colorClass: string;
  }

  let { dist, samples, title, rule, unit, sampleLabel, colorClass }: Props = $props();

  const W = 560;
  const H = 224;
  // The top padding is a reserved two-line label row, above the top gridline.
  const PAD = { top: 40, right: 12, bottom: 34, left: 48 };
  const LABEL_Y = 15;
  const LABEL_Y2 = 29;
  const KINK_W = 14;
  const KINK_AMP = 6;
  const KINK_PAD = 4;
  const KINK_CLEAR = 6;
  const MIN_BAR_PX = 1.5;
  // Smallest hover target for a bar, so a one-request bucket is still catchable.
  const MIN_HIT_PX = 12;
  // Headroom above the tallest bucket, so a percentile line is always hoverable
  // over the bars and the tallest bar does not touch the top gridline.
  const Y_HEADROOM = 1.2;
  // Labels keep to the reserved rows, so nothing overlaps a bar or a gridline.
  const labelX = (centre: number) => Math.min(Math.max(centre, 96), W - 96);
  const shortUnit = "t/s";

  const innerH = H - PAD.top - PAD.bottom;
  const base = PAD.top + innerH;

  let bucketShares = $derived(shares(dist.counts, dist.count));
  let mid = $derived(dist.counts.slice(1, -1));
  let maxShare = $derived(Math.max(...mid.map((_, i) => bucketShares[i + 1]), 0));
  let yMax = $derived(niceMax(maxShare * Y_HEADROOM));
  // Each kink leaves the axis and returns to it, so the line, the kink and the
  // line read as one stroke. In the middle the kink swings above and below the
  // axis. A bar never starts closer than KINK_CLEAR to a kink.
  let kinks = $derived(
    [
      dist.below > 0 ? { x: PAD.left + KINK_PAD, side: "below" as const } : null,
      dist.above > 0 ? { x: W - PAD.right - KINK_PAD - KINK_W, side: "above" as const } : null,
    ].filter((k): k is { x: number; side: "below" | "above" } => k !== null),
  );
  let axisSegments = $derived.by(() => {
    const segs: [number, number][] = [];
    let cursor = PAD.left;
    for (const kink of kinks) {
      segs.push([cursor, kink.x]);
      cursor = kink.x + KINK_W;
    }
    segs.push([cursor, W - PAD.right]);
    return segs.filter(([from, to]) => to > from);
  });
  let plotLeft = $derived(PAD.left + KINK_PAD + KINK_W + (dist.below > 0 ? KINK_CLEAR : 0));
  let plotRight = $derived(W - PAD.right - KINK_PAD - KINK_W - (dist.above > 0 ? KINK_CLEAR : 0));

  // One peak above the axis and one trough below it, then back up onto the axis.
  // The stroke joins the two axis segments at their ends, so the break reads as a
  // kink in the line instead of a mark placed beside it.
  function kinkPath(x0: number) {
    const up = base - KINK_AMP;
    const down = base + KINK_AMP;
    const peak = x0 + Math.round(KINK_W * 0.28);
    const trough = x0 + Math.round(KINK_W * 0.67);
    return `M ${x0} ${base} L ${peak} ${up} L ${trough} ${down} L ${x0 + KINK_W} ${base}`;
  }
  let decades = $derived(dist.window.lo > 0 ? Math.log10(dist.window.lo) : 0);
  let decadeSpan = $derived(Math.log10(dist.window.hi) - decades || 1);

  let x = $derived.by(() => {
    const scale = (value: number) =>
      plotLeft + ((Math.log10(value) - decades) / decadeSpan) * (plotRight - plotLeft);
    return { scale, left: plotLeft, right: plotRight };
  });

  let bars = $derived(
    dist.edges.slice(0, -1).map((edge, i) => {
      const next = dist.edges[i + 1];
      const left = x.scale(edge);
      const right = x.scale(next);
      const share = bucketShares[i + 1] ?? 0;
      const count = mid[i] ?? 0;
      const height = count > 0 ? Math.max((share / yMax) * innerH, MIN_BAR_PX) : 0;
      return {
        index: i,
        lower: edge,
        upper: next,
        count,
        share,
        left,
        width: Math.max(right - left - 1, 1),
        y: base - height,
        // Own height plus a small tolerance. Everything above that belongs to the
        // percentile lines, so a short bar never steals a line that crosses it.
        hitY: base - Math.max(height, MIN_HIT_PX),
        height,
        centre: (left + right) / 2,
        range: rangeLabel(edge, next, shortUnit),
        countText: countLabel(count),
      };
    }),
  );

  let axisTicks = $derived(axisLabels(dist.edges).map((edge) => ({ x: x.scale(edge), label: formatRate(edge) })));
  let yAxisTicks = $derived(yTicks(yMax).map((tick) => ({
    y: base - tick.fraction * innerH,
    value: tick.value,
  })));
  let lines = $derived(drawablePercentiles(dist));
  let strip = $derived(percentileEntries(dist));
  let rows = $derived(tableRows(dist));
  let caption = $derived(reportingLine(samples, sampleLabel, rule));
  // One bar spans this factor of the speed axis, so a reader comparing two time
  // ranges can tell why the bars differ.
  let bucketFactor = $derived(Math.pow(10, 1 / dist.stepsPerDecade).toFixed(3));
  let bucketWidthText = $derived(`each bar covers a factor of ${bucketFactor} in speed`);
  let description = $derived(
    `${title}, per request, log scale on the speed axis. ${caption} ` +
      `Median ${dist.p50 === null ? "unavailable" : formatRate(dist.p50)} ${unit}. ` +
      `The plot runs from ${formatRate(dist.window.lo)} to ${formatRate(dist.window.hi)} ${unit} in ` +
      `${Math.max(dist.edges.length - 1, 1)} buckets of equal log width, ${bucketWidthText}. ` +
      `${dist.below} samples below and ${dist.above} above the window are not drawn. ` +
      `A data table lists every non-empty bucket.`,
  );

  let hovered = $state<number | null>(null);
  let hoveredLine: string | null = $state(null);
</script>

<figure class="m-0">
  <svg
    viewBox="0 0 {W} {H}"
    class="{colorClass} h-auto w-full"
    role="img"
    aria-label={description}
  >
    <!-- axis line, with an inline kink at each end that clips a tail -->
    {#each axisSegments as seg}
      <line x1={seg[0]} y1={base} x2={seg[1]} y2={base} class="axis" />
    {/each}
    {#each kinks as kink}
      {@const d = kinkPath(kink.x)}
      <path {d} class="axis kink">
        <title
          >{kink.side === "below"
            ? tailCaption(dist.below, dist.count, dist.edges[0], "below")
            : tailCaption(dist.above, dist.count, dist.edges[dist.edges.length - 1], "above")}</title
        >
      </path>
    {/each}

    <!-- grid -->
    {#each yAxisTicks as tick}
      <line x1={PAD.left} y1={tick.y} x2={W - PAD.right} y2={tick.y} class="grid" />
      <text x={PAD.left - 6} y={tick.y + 3} text-anchor="end">{tick.value}</text>
    {/each}
    {#each axisTicks as tick}
      <line x1={tick.x} y1={PAD.top} x2={tick.x} y2={base} class="grid" />
      <text x={tick.x} y={base + 12} text-anchor="middle">{tick.label}</text>
    {/each}

    <!-- window bounds, only where a tail is clipped -->
    {#if dist.below > 0}
      <line x1={x.left} y1={PAD.top} x2={x.left} y2={base} class="axis" stroke-dasharray="2 3" />
    {/if}
    {#if dist.above > 0}
      <line x1={x.right} y1={PAD.top} x2={x.right} y2={base} class="axis" stroke-dasharray="2 3" />
    {/if}

    <!-- Percentile lines: always drawn, labelled only while the line itself is
         hovered. A wide invisible line makes each one easy to hit. This block is
         painted before the bars, so where a bar and a line overlap the bar wins
         the pointer. Everywhere above a bar belongs to the lines, and the
         headroom above the tallest bar guarantees each line has such a band. -->
    {#each lines as line}
      {@const px = x.scale(line.value as number)}
      <line
        x1={px}
        y1={PAD.top}
        x2={px}
        y2={base}
        class="pl"
        class:pl-on={hoveredLine === line.key}
        stroke-dasharray={line.dash}
      />
      <line
        x1={px}
        y1={PAD.top}
        x2={px}
        y2={base}
        class="pl-hit"
        role="presentation"
        aria-hidden="true"
        onpointerenter={() => (hoveredLine = line.key)}
        onpointerleave={() => (hoveredLine = null)}
      />
      {#if hoveredLine === line.key}
        <text x={labelX(px)} y={LABEL_Y2} text-anchor="middle" class="callout">
          {line.key}: {formatExact(line.value as number)} {shortUnit}
        </text>
      {/if}
    {/each}

    <!-- buckets. The group is role="presentation": the svg already carries the
         description, the hover label repeats it on screen, and the table below
         holds the same numbers as text. -->
    {#each bars as bar}
      {#if bar.count > 0}
        <g
          role="presentation"
          class="bucket"
          onpointerenter={() => (hovered = bar.index)}
          onpointerleave={() => (hovered = null)}
        >
          <rect x={bar.left} y={bar.hitY} width={bar.width} height={base - bar.hitY} fill="transparent" />
          <rect
            x={bar.left}
            y={bar.y}
            width={bar.width}
            height={bar.height}
            fill="currentColor"
            opacity={hovered === bar.index ? 0.95 : 0.6}
          />
          {#if hovered === bar.index}
            <text x={labelX(bar.centre)} y={LABEL_Y} text-anchor="middle" class="callout">{bar.range}</text>
            <text x={labelX(bar.centre)} y={LABEL_Y2} text-anchor="middle" class="callout">{bar.countText}</text>
          {/if}
        </g>
      {/if}
    {/each}

  </svg>

  <div class="sr-only">
    <p>{caption}</p>
    <p>
      Percentiles of {unit}:
      {[...strip]
        .map((entry) => {
          if (!entry.supported) return `${entry.key} not enough samples`;
          const drawn = lines.some((line) => line.key === entry.key);
          return `${entry.key} ${formatExact(entry.value ?? 0)}${drawn ? "" : " (outside the plotted range)"}`;
        })
        .join("; ")}
    </p>
    <table class="sr-only">
      <caption>{title} per request, in buckets of equal log width on a log speed axis, {bucketWidthText}</caption>
      <thead>
        <tr>
          <th>Lower {unit}</th>
          <th>Upper {unit}</th>
          <th>Requests</th>
          <th>Share</th>
        </tr>
      </thead>
      <tbody>
        {#each rows as row}
          <tr>
            <td>{row.lower}</td>
            <td>{row.upper}</td>
            <td>{row.count.toLocaleString()}</td>
            <td>{row.share}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
</figure>

<style>
  svg text {
    font-size: 10px;
    fill: var(--muted-foreground);
  }
  .pl {
    stroke: currentColor;
    stroke-width: 1.6;
    opacity: 0.45;
  }
  .pl-on {
    opacity: 0.95;
  }
  .pl-hit {
    stroke: transparent;
    stroke-width: 12;
    pointer-events: stroke;
  }
  .callout {
    pointer-events: none;
    fill: var(--card-foreground);
    paint-order: stroke;
    stroke: var(--card);
    stroke-width: 3px;
    stroke-linejoin: round;
  }
  .axis {
    stroke: var(--foreground);
    stroke-opacity: 0.3;
    stroke-width: 1;
  }
  .grid {
    stroke: var(--foreground);
    stroke-opacity: 0.08;
    stroke-width: 1;
  }
  .kink {
    fill: none;
    stroke-linejoin: round;
    stroke-linecap: round;
    pointer-events: stroke;
  }
  .kink:hover {
    stroke-opacity: 0.6;
  }
</style>

<script lang="ts">
  import type { ActivityStatsData } from "../lib/types";
  import { persistentStore } from "../stores/persistent";
  import SpeedHistogram from "./SpeedHistogram.svelte";
  import { ChevronDown, X } from "@lucide/svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import { Button } from "$lib/components/ui/button/index.js";

  interface Props {
    stats: ActivityStatsData | null;
  }

  let { stats }: Props = $props();

  const nf = new Intl.NumberFormat();

  /** States the backend's usability rule, so exclusions are explainable. */
  function speedRule(dist: { minTokens: number; maxRatePerSecond: number }, unit: string): string {
    return `fewer than ${nf.format(dist.minTokens)} tokens, or above ${nf.format(dist.maxRatePerSecond)} ${unit}`;
  }
  const histogramCollapsed = persistentStore<boolean>("activity-histogram-collapsed", false);
</script>

<Card.Root class="relative p-3">
  <Button
    variant="ghost"
    size="icon-xs"
    class="text-muted-foreground absolute right-2 top-2 rounded-full"
    onclick={() => ($histogramCollapsed = !$histogramCollapsed)}
    title={$histogramCollapsed ? "Show histograms" : "Hide histograms"}
  >
    {#if $histogramCollapsed}
      <ChevronDown />
    {:else}
      <X />
    {/if}
  </Button>
  {#if !$histogramCollapsed}
    <div class="mb-3 flex flex-col gap-6 sm:flex-row">
      <div class="w-full min-w-0 sm:w-1/2">
        <div class="text-muted-foreground mb-1 text-sm font-medium">Prompt Processing</div>
        {#if stats?.prompt_speed && stats?.prompt_samples}
          <SpeedHistogram
            dist={stats.prompt_speed}
            samples={stats.prompt_samples}
            title="Prompt processing speed"
            unit="prompt tokens/sec"
            sampleLabel="prompt rate"
            rule={speedRule(stats.prompt_speed, "prompt tokens/sec")}
            colorClass="text-amber-500 dark:text-amber-400"
          />
        {:else}
          <div class="text-muted-foreground py-6 text-center text-sm">No prompt speed data yet</div>
        {/if}
      </div>
      <div class="w-full min-w-0 sm:w-1/2">
        <div class="text-muted-foreground mb-1 text-sm font-medium">Token Generation</div>
        {#if stats?.gen_speed && stats?.gen_samples}
          <SpeedHistogram
            dist={stats.gen_speed}
            samples={stats.gen_samples}
            title="Token generation speed"
            unit="tokens/sec"
            sampleLabel="generation rate"
            rule={speedRule(stats.gen_speed, "tokens/sec")}
            colorClass="text-blue-500 dark:text-blue-400"
          />
        {:else}
          <div class="text-muted-foreground py-6 text-center text-sm">No generation speed data yet</div>
        {/if}
      </div>
    </div>
  {/if}
  <div class="grid grid-cols-4 gap-x-6 gap-y-1 text-sm">
    <div class="text-muted-foreground text-xs uppercase tracking-wider">Requests</div>
    <div class="text-muted-foreground text-xs uppercase tracking-wider">Cached</div>
    <div class="text-muted-foreground text-xs uppercase tracking-wider">Processed</div>
    <div class="text-muted-foreground text-xs uppercase tracking-wider">Generated</div>
    <div class="text-sm">
      <span class="font-semibold">{nf.format(stats?.total_requests ?? 0)}</span> completed
    </div>
    <div class="text-sm">
      <span class="font-semibold">{nf.format(stats?.total_cache_tokens ?? 0)}</span> tokens
    </div>
    <div class="text-sm">
      <span class="font-semibold">{nf.format(stats?.total_input_tokens ?? 0)}</span> tokens
    </div>
    <div class="text-sm">
      <span class="font-semibold">{nf.format(stats?.total_output_tokens ?? 0)}</span> tokens
    </div>
  </div>
</Card.Root>

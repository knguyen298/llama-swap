package sqlite

import (
	"context"
	"math"
	"testing"

	"github.com/mostlygeek/llama-swap/internal/store"
)

// Every sort key the store package accepts must map to a SQL column here, and
// nothing outside that whitelist may be sortable.
func TestActivity_SortColumnsCoverStoreKeys(t *testing.T) {
	keys := store.ActivitySortKeys()
	for _, key := range keys {
		if _, ok := activitySortColumns[key]; !ok {
			t.Errorf("store sort key %q has no SQL column", key)
		}
	}
	if len(activitySortColumns) != len(keys) {
		t.Errorf("activitySortColumns has %d entries, store has %d keys", len(activitySortColumns), len(keys))
	}
	for key := range activitySortColumns {
		if !store.ValidActivitySortKey(key) {
			t.Errorf("SQL sort column key %q is not a store sort key", key)
		}
	}
}

func TestActivity_StatsSpeedSamples(t *testing.T) {
	ctx := context.Background()
	st, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer st.Close()

	// Each row exercises one rule: usable, too few prompt tokens, too few
	// generated tokens, over the physical bound, reported as zero, and unreported.
	rows := []store.ActivityLogEntry{
		{Model: "m1", Tokens: store.TokenMetrics{InputTokens: 500, OutputTokens: 100, PromptPerSecond: 3000, TokensPerSecond: 40}},
		{Model: "m1", Tokens: store.TokenMetrics{InputTokens: 1, OutputTokens: 1, PromptPerSecond: 5, TokensPerSecond: 20}},
		{Model: "m1", Tokens: store.TokenMetrics{InputTokens: 800, OutputTokens: 1, PromptPerSecond: -1, TokensPerSecond: 1_000_000}},
		{Model: "m1", Tokens: store.TokenMetrics{InputTokens: 10, OutputTokens: 10, PromptPerSecond: -1, TokensPerSecond: -1}},
		{Model: "m1", Tokens: store.TokenMetrics{InputTokens: 10, OutputTokens: 0, PromptPerSecond: 0, TokensPerSecond: 0}},
		{Model: "m1", Tokens: store.TokenMetrics{InputTokens: 100, OutputTokens: 50, PromptPerSecond: 200_000, TokensPerSecond: 30}},
	}
	for i := range rows {
		if _, err := st.Activity().Insert(ctx, rows[i]); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}

	stats, err := st.Activity().Stats(ctx, store.ActivityStatsQuery{})
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	if got, want := stats.PromptSamples, (store.SpeedSampleCounts{Rows: 6, Reported: 3, Usable: 1, Excluded: 2, Missing: 3}); got != want {
		t.Errorf("prompt samples = %+v, want %+v", got, want)
	}
	if got, want := stats.GenerationSamples, (store.SpeedSampleCounts{Rows: 6, Reported: 4, Usable: 2, Excluded: 2, Missing: 2}); got != want {
		t.Errorf("generation samples = %+v, want %+v", got, want)
	}

	// The shipped HistogramData fields must still see every reported rate, so old
	// clients keep getting the numbers they got before.
	if got := sumBins(stats.PromptHistogram); got != 3 {
		t.Errorf("legacy prompt histogram holds %d values, want 3 reported rates", got)
	}
	if got := sumBins(stats.GenerationHistogram); got != 4 {
		t.Errorf("legacy generation histogram holds %d values, want 4 reported rates", got)
	}

	if stats.PromptSpeed == nil || stats.PromptSpeed.Count != 1 {
		t.Fatalf("prompt speed = %+v, want one usable sample", stats.PromptSpeed)
	}
	if sum := sumCounts(stats.PromptSpeed.Counts); sum != stats.PromptSpeed.Count {
		t.Errorf("prompt speed counts %v sum to %d, want %d", stats.PromptSpeed.Counts, sum, stats.PromptSpeed.Count)
	}
	if len(stats.PromptSpeed.Edges) < 2 {
		t.Fatalf("a single value still needs two edges: %v", stats.PromptSpeed.Edges)
	}
	if stats.PromptSpeed.P50 != nil {
		t.Errorf("one sample must not publish p50, got %v", *stats.PromptSpeed.P50)
	}

	gen := stats.GenerationSpeed
	if gen == nil || gen.Count != 2 || gen.Min != 30 || gen.Max != 40 {
		t.Fatalf("generation speed = %+v, want two usable samples spanning 30 to 40", gen)
	}
	// Two samples cannot support a window, so nothing may be clipped away.
	if gen.Below != 0 || gen.Above != 0 {
		t.Errorf("small sample clipped: below %d above %d", gen.Below, gen.Above)
	}
}

func TestActivity_StatsSpeedWindowAndSupportGate(t *testing.T) {
	ctx := context.Background()
	st, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer st.Close()

	// 1,000 ordinary prefill rates, then three that pass the physical bound but
	// sit far above the bulk. They are under 0.5 percent of the sample, so the
	// window must park them in Above instead of widening the axis for them.
	insert := func(rate float64) {
		t.Helper()
		entry := store.ActivityLogEntry{Model: "m1", Tokens: store.TokenMetrics{
			InputTokens: 500, OutputTokens: 10, PromptPerSecond: rate, TokensPerSecond: 40,
		}}
		if _, err := st.Activity().Insert(ctx, entry); err != nil {
			t.Fatalf("Insert: %v", err)
		}
	}
	for i := range 1000 {
		insert(float64(100 + i))
	}
	for range 3 {
		insert(90_000)
	}

	stats, err := st.Activity().Stats(ctx, store.ActivityStatsQuery{})
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	speed := stats.PromptSpeed
	if speed == nil || speed.Count != 1003 {
		t.Fatalf("prompt speed = %+v, want 1003 usable samples", speed)
	}
	if speed.Above != 3 {
		t.Errorf("the three far-right rates must land in Above, got %d (window %v..%v)", speed.Above, speed.Window.Lo, speed.Window.Hi)
	}
	if speed.Below != 0 {
		t.Errorf("nothing sits under the first edge, got Below %d", speed.Below)
	}
	if speed.Window.Hi > 2000 {
		t.Errorf("window top %v let three rates stretch the axis", speed.Window.Hi)
	}
	if speed.Max != 90_000 {
		t.Errorf("Max must stay honest, got %v", speed.Max)
	}
	if speed.P50 == nil || *speed.P50 < 550 || *speed.P50 > 650 {
		t.Errorf("p50 = %v, want it inside the bulk of the sample", deref(speed.P50))
	}
	// 1,003 samples leave 10 beyond p99, which clears the five-sample floor.
	if speed.P99 == nil {
		t.Errorf("1003 samples support p99, got nil")
	}
	if sum := sumCounts(speed.Counts); sum != speed.Count {
		t.Errorf("counts sum to %d, want %d", sum, speed.Count)
	}
	for i := 1; i < len(speed.Edges); i++ {
		ratio := speed.Edges[i] / speed.Edges[i-1]
		if want := math.Pow(10, 1/float64(speed.StepsPerDecade)); math.Abs(ratio-want) > 1e-9 {
			t.Fatalf("edge %d ratio %v != grid ratio %v", i, ratio, want)
		}
	}
}

func sumBins(h *store.HistogramData) int {
	if h == nil {
		return 0
	}
	total := 0
	for _, n := range h.Bins {
		total += n
	}
	return total
}

func sumCounts(counts []int) int {
	total := 0
	for _, n := range counts {
		total += n
	}
	return total
}

func deref(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

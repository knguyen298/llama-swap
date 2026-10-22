package store

import (
	"math"
	"math/rand"
	"slices"
	"testing"
)

func near(got, want float64) bool {
	return math.Abs(got-want) < 1e-9
}

func TestActivityQuery_WithDefaults(t *testing.T) {
	tests := []struct {
		name      string
		in        ActivityQuery
		wantPage  int
		wantLimit int
	}{
		{"zero value", ActivityQuery{}, 1, DefaultActivityLimit},
		{"negative", ActivityQuery{Page: -3, Limit: -1}, 1, DefaultActivityLimit},
		{"set values kept", ActivityQuery{Page: 4, Limit: 7}, 4, 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.WithDefaults()
			if got.Page != tt.wantPage || got.Limit != tt.wantLimit {
				t.Fatalf("WithDefaults() = page %d limit %d, want %d and %d", got.Page, got.Limit, tt.wantPage, tt.wantLimit)
			}
		})
	}

	// Filter and sort fields must pass through untouched.
	in := ActivityQuery{ActivityFilter: ActivityFilter{Models: []string{"m"}, MinID: 2}, Sort: "time", Order: "asc"}
	got := in.WithDefaults()
	if got.Sort != "time" || got.Order != "asc" || got.MinID != 2 || len(got.Models) != 1 {
		t.Fatalf("WithDefaults() dropped fields: %+v", got)
	}
}

func TestActivity_SortKeys(t *testing.T) {
	keys := ActivitySortKeys()
	if len(keys) == 0 {
		t.Fatal("no sort keys")
	}
	for _, key := range keys {
		if !ValidActivitySortKey(key) {
			t.Fatalf("ValidActivitySortKey(%q) = false", key)
		}
	}
	for _, key := range []string{"", "bogus", "ID", "ts_created; DROP TABLE activity"} {
		if ValidActivitySortKey(key) {
			t.Fatalf("ValidActivitySortKey(%q) = true", key)
		}
	}

	// The returned slice is a copy: mutating it must not change the whitelist.
	keys[0] = "mutated"
	if ValidActivitySortKey("mutated") {
		t.Fatal("ActivitySortKeys() exposes the internal whitelist")
	}
}

func TestActivity_TotalPages(t *testing.T) {
	tests := []struct {
		total, limit, want int
	}{
		{0, 10, 0},
		{5, 0, 0},
		{5, -1, 0},
		{1, 10, 1},
		{10, 10, 1},
		{11, 10, 2},
		{25, 10, 3},
	}
	for _, tt := range tests {
		if got := TotalPages(tt.total, tt.limit); got != tt.want {
			t.Errorf("TotalPages(%d, %d) = %d, want %d", tt.total, tt.limit, got, tt.want)
		}
	}
}

func TestActivity_NewHistogram(t *testing.T) {
	if got := NewHistogram(nil); got != nil {
		t.Fatalf("NewHistogram(nil) = %+v, want nil", got)
	}

	same := NewHistogram([]float64{7, 7, 7})
	if same == nil || len(same.Bins) != 1 || same.Bins[0] != 3 || same.BinSize != 0 || same.Min != 7 || same.Max != 7 {
		t.Fatalf("all-equal histogram = %+v", same)
	}
	if same.P50 != 7 || same.P95 != 7 || same.P99 != 7 {
		t.Fatalf("all-equal percentiles = %+v", same)
	}

	values := []float64{30, 10, 20, 40}
	h := NewHistogram(values)
	if h == nil {
		t.Fatal("nil histogram")
	}
	// Sturges' rule gives 3 bins for 4 values; the minimum is 5.
	if len(h.Bins) != 5 {
		t.Fatalf("bins = %v, want 5 buckets", h.Bins)
	}
	if h.Min != 10 || h.Max != 40 || h.BinSize != 6 {
		t.Fatalf("range = min %v max %v size %v", h.Min, h.Max, h.BinSize)
	}
	count := 0
	for _, n := range h.Bins {
		count += n
	}
	if count != len(values) {
		t.Fatalf("bins %v hold %d values, want %d", h.Bins, count, len(values))
	}
	// The max value lands in the last bucket instead of overflowing.
	if h.Bins[len(h.Bins)-1] != 1 {
		t.Fatalf("max value not in last bucket: %v", h.Bins)
	}
	// Linear interpolation: p50 of 10,20,30,40 is 25.
	if !near(h.P50, 25) || !near(h.P95, 38.5) || !near(h.P99, 39.7) {
		t.Fatalf("percentiles = p50 %v p95 %v p99 %v", h.P50, h.P95, h.P99)
	}
	// Input must not be sorted in place.
	if values[0] != 30 {
		t.Fatalf("input was modified: %v", values)
	}
}

func TestActivity_SpeedBucketIndex(t *testing.T) {
	tests := []struct {
		value float64
		want  int
	}{
		{1, 0},
		{SpeedEdge(1), 1},
		{2, 3},
		{10, 12},
		{1000, 36},
		{0.5, -4},
		{0.1, -12},
	}
	for _, tt := range tests {
		if got := SpeedBucketIndex(tt.value); got != tt.want {
			t.Errorf("SpeedBucketIndex(%v) = %d, want %d", tt.value, got, tt.want)
		}
	}
}

func TestActivity_SpeedBarsForShrinksWithTheSample(t *testing.T) {
	for _, tt := range []struct {
		n, want int
	}{
		{0, SpeedBarsFloor},
		{9, SpeedBarsFloor},
		{24, 5},
		{100, 10},
		{256, SpeedBarsTarget},
		{100000, SpeedBarsTarget},
	} {
		if got := SpeedBarsFor(tt.n); got != tt.want {
			t.Errorf("SpeedBarsFor(%d) = %d, want %d", tt.n, got, tt.want)
		}
	}
}

func TestActivity_SpeedStepsForRefinesNarrowWindows(t *testing.T) {
	for _, tt := range []struct {
		name   string
		lo, hi float64
		n      int
		want   int
	}{
		{"a window over two decades keeps the base grid", 56, 14678, 23819, SpeedStepsPerDecade},
		{"a plus or minus 10 percent window refines", 54, 66, 8000, 192},
		{"a small sample in a narrow window refines less", 54, 65, 24, 96},
		{"a degenerate window stops at the finest grid", 1000, 1001, 8000, 192},
		{"a tiny sample over three decades coarsens", 1, 1000, 24, SpeedStepsFloor},
		{"61 requests over three decades, no cliff from 60", 1, 1000, 61, SpeedStepsFloor},
		{"100 requests over three decades refine one notch", 1, 1000, 100, 6},
		{"1000 requests over three decades keep the base grid", 1, 1000, 1000, SpeedStepsPerDecade},
		{"a wide window with a large sample keeps the base grid", 1, 1e6, 45929, SpeedStepsPerDecade},
		{"equal bounds keep the base grid", 3000, 3000, 100, SpeedStepsPerDecade},
		{"a zero lower bound keeps the base grid", 0, 100, 100, SpeedStepsPerDecade},
	} {
		if got := SpeedStepsFor(tt.lo, tt.hi, tt.n); got != tt.want {
			t.Errorf("%s: SpeedStepsFor(%v, %v, %d) = %d, want %d", tt.name, tt.lo, tt.hi, tt.n, got, tt.want)
		}
	}
}

func TestActivity_SpeedResolutionsAreNested(t *testing.T) {
	// sort.Search over the edges requires ascending edges, and the nesting claim
	// requires each coarser edge to reappear in every finer grid.
	for _, coarse := range SpeedResolutions {
		for _, fine := range SpeedResolutions {
			if fine < coarse {
				continue
			}
			ratio := fine / coarse
			if fine%coarse != 0 {
				t.Fatalf("%d does not divide %d, so their grids are not nested", coarse, fine)
			}
			for k := -60; k <= 600; k++ {
				// Exact equality, not a tolerance: clients match edges by value.
				if got, want := SpeedEdgeAt(k*ratio, fine), SpeedEdgeAt(k, coarse); got != want {
					t.Fatalf("SpeedEdgeAt(%d, %d) = %v, want exactly %v from the %d-step grid", k*ratio, fine, got, want, coarse)
				}
			}
		}
	}
}

func TestActivity_SpeedLogIndexMissesItsOwnEdges(t *testing.T) {
	// Guards the reason NewSpeedDistribution counts by comparing values against
	// the edges. math.Log10 rounds twice, so re-deriving an index from an edge
	// built with math.Pow lands one bucket low on a fraction of edges.
	for _, steps := range SpeedResolutions {
		misses := 0
		for k := -300; k <= 3000; k++ {
			if SpeedBucketIndexAt(SpeedEdgeAt(k, steps), steps) != k {
				misses++
			}
		}
		// No assertion here. A platform that rounds the log round trip exactly
		// reports zero misses, and TestActivity_SpeedCountsPlaceValuesByContract
		// guards the behaviour that matters.
		t.Logf("steps %d: %d of 3301 edges mis-index", steps, misses)
	}
}

func TestActivity_SpeedCountsPlaceValuesByContract(t *testing.T) {
	// A hand made case, so the expected counts come from the documented contract
	// and not from the same predicate the code uses. Values 1 and 10 are the
	// window edges, and 2.154434690031884 is the middle edge of the 3-step grid
	// this sample selects: 1, 2.154, 4.642, 10.
	dist := NewSpeedDistribution([]float64{1, 2, 2.154434690031884, 10}, MinGenTokens)
	if dist == nil {
		t.Fatal("NewSpeedDistribution returned nil")
	}
	if dist.StepsPerDecade != 3 {
		t.Fatalf("StepsPerDecade = %d, want 3 for 4 samples over one decade", dist.StepsPerDecade)
	}
	wantEdges := []float64{1, 2.154434690031884, 4.641588833612778, 10}
	if !slices.Equal(dist.Edges, wantEdges) {
		t.Fatalf("Edges = %v, want %v", dist.Edges, wantEdges)
	}
	// Index 0 is below Edges[0]; 1 covers [1, 2.154); 2 covers [2.154, 4.642);
	// 3 covers [4.642, 10); the last index is at or above Edges[3].
	wantCounts := []int{0, 2, 1, 0, 1}
	if !slices.Equal(dist.Counts, wantCounts) {
		t.Errorf("Counts = %v, want %v", dist.Counts, wantCounts)
	}
	if dist.Below != 0 || dist.Above != 1 {
		t.Errorf("Below %d / Above %d, want 0 / 1", dist.Below, dist.Above)
	}
	if got := sumCounts(dist.Counts); got != dist.Count {
		t.Errorf("counts sum to %d, want %d", got, dist.Count)
	}
}

func TestActivity_SpeedCountsMatchThePublishedEdges(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	for trial := range 12 {
		values := make([]float64, 0, 400)
		switch trial % 4 {
		case 0: // three decades
			for range 400 {
				values = append(values, math.Pow(10, rng.Float64()*3))
			}
		case 1: // one tight model
			for range 400 {
				values = append(values, 60*(0.9+0.2*rng.Float64()))
			}
		case 2: // six decades
			for range 400 {
				values = append(values, math.Pow(10, rng.Float64()*6))
			}
		case 3: // clustered, with values sitting on edges
			for range 400 {
				values = append(values, 100*math.Pow(2, rng.Float64()*4))
			}
		}
		dist := NewSpeedDistribution(values, MinGenTokens)
		if dist == nil {
			t.Fatal("NewSpeedDistribution returned nil")
		}
		if !slices.Contains(SpeedResolutions, dist.StepsPerDecade) {
			t.Errorf("trial %d: StepsPerDecade %d is off the ladder %v", trial, dist.StepsPerDecade, SpeedResolutions)
		}
		want := make([]int, len(dist.Edges)+1)
		for _, v := range values {
			j := 0
			for j < len(dist.Edges) && dist.Edges[j] <= v {
				j++
			}
			want[j]++
		}
		if !slices.Equal(want, dist.Counts) {
			t.Errorf("trial %d (steps %d): counts %v != brute force %v", trial, dist.StepsPerDecade, dist.Counts, want)
		}
		if dist.Below != want[0] || dist.Above != want[len(want)-1] {
			t.Errorf("trial %d: Below %d / Above %d, want %d / %d", trial, dist.Below, dist.Above, want[0], want[len(want)-1])
		}
		if got := sumCounts(dist.Counts); got != dist.Count {
			t.Errorf("trial %d: counts sum to %d, want %d", trial, got, dist.Count)
		}
	}
}

func TestActivity_SpeedDistributionDrawsEnoughBarsForOneModel(t *testing.T) {
	// One model on one GPU: every request lands within 10 percent of 60 t/s.
	values := make([]float64, 0, 8000)
	for i := range 8000 {
		values = append(values, 60*(0.9+0.2*float64(i)/8000))
	}

	dist := NewSpeedDistribution(values, MinGenTokens)
	if dist == nil {
		t.Fatal("NewSpeedDistribution returned nil")
	}
	bars := len(dist.Edges) - 1
	if bars < 12 {
		t.Errorf("a tight window drew %d bars, want enough to see a shape", bars)
	}
	if dist.StepsPerDecade <= SpeedStepsPerDecade {
		t.Errorf("StepsPerDecade = %d, want refinement above the base grid", dist.StepsPerDecade)
	}
	if dist.StepsPerDecade%SpeedStepsPerDecade != 0 {
		t.Errorf("StepsPerDecade = %d, want a multiple of the base grid so edges stay nested", dist.StepsPerDecade)
	}
	if got := sumCounts(dist.Counts); got != dist.Count {
		t.Errorf("counts sum to %d, want %d", got, dist.Count)
	}
	ratio := math.Pow(10, 1/float64(dist.StepsPerDecade))
	for i := 1; i < len(dist.Edges); i++ {
		if got := dist.Edges[i] / dist.Edges[i-1]; math.Abs(got-ratio) > 1e-9 {
			t.Fatalf("edge %d ratio %v != grid ratio %v", i, got, ratio)
		}
	}
	if !(dist.Window.Lo <= dist.Min && dist.Max <= dist.Window.Hi) {
		t.Errorf("window %v..%v does not hold the sample %v..%v", dist.Window.Lo, dist.Window.Hi, dist.Min, dist.Max)
	}
}

func TestActivity_SpeedEdgesHaveEqualLogWidth(t *testing.T) {
	edges := SpeedEdges(2, 8000)
	if len(edges) < 3 {
		t.Fatalf("expected several edges, got %v", edges)
	}
	if edges[0] > 2 || edges[len(edges)-1] < 8000 {
		t.Fatalf("edges %v..%v do not cover the requested 2..8000", edges[0], edges[len(edges)-1])
	}
	wantRatio := math.Pow(10, 1/float64(SpeedStepsPerDecade))
	for i := 1; i < len(edges); i++ {
		if ratio := edges[i] / edges[i-1]; math.Abs(ratio-wantRatio) > 1e-9 {
			t.Fatalf("edge %d ratio %v != grid ratio %v", i, ratio, wantRatio)
		}
	}
}

func TestActivity_SpeedEdgesWidenForASingleValue(t *testing.T) {
	edges := SpeedEdges(3000, 3000)
	if len(edges) != 2 {
		t.Fatalf("single value edges = %v, want two distinct edges", edges)
	}
	if edges[0] > 3000 || edges[1] <= 3000 {
		t.Fatalf("single value edges %v do not hold 3000", edges)
	}
}

func TestActivity_SpeedPercentileSupported(t *testing.T) {
	tests := []struct {
		n    int
		p    float64
		want bool
	}{
		{10, 50, true}, {9, 50, false},
		{50, 90, true}, {49, 90, false},
		{100, 95, true}, {99, 95, false},
		{500, 99, true}, {499, 99, false},
		{1000, 100, false}, {1000, 0, false},
	}
	for _, tt := range tests {
		if got := SpeedPercentileSupported(tt.n, tt.p); got != tt.want {
			t.Errorf("SpeedPercentileSupported(%d, %v) = %v, want %v", tt.n, tt.p, got, tt.want)
		}
	}
}

func TestActivity_NewSpeedDistribution(t *testing.T) {
	if got := NewSpeedDistribution(nil, MinPromptTokens); got != nil {
		t.Fatalf("NewSpeedDistribution(nil, MinPromptTokens) = %+v, want nil", got)
	}

	values := []float64{120, 40, 900, 40, 60}
	dist := NewSpeedDistribution(values, MinPromptTokens)
	if dist == nil {
		t.Fatal("nil distribution")
	}
	if dist.Count != len(values) || dist.Min != 40 || dist.Max != 900 {
		t.Fatalf("summary = %+v", dist)
	}
	if dist.P50 != nil || dist.P90 != nil || dist.P95 != nil || dist.P99 != nil {
		t.Fatalf("five samples must not publish percentiles: %+v", dist)
	}
	// Under 200 samples the window is the data range, so nothing is clipped.
	if dist.Below != 0 || dist.Above != 0 {
		t.Fatalf("small sample clipped: below %d above %d", dist.Below, dist.Above)
	}
	if sum := sumCounts(dist.Counts); sum != dist.Count {
		t.Fatalf("counts %v sum to %d, want %d", dist.Counts, sum, dist.Count)
	}
	if values[0] != 120 {
		t.Fatalf("input was modified: %v", values)
	}
}

func TestActivity_NewSpeedDistributionParksOutliersAboveTheWindow(t *testing.T) {
	values := make([]float64, 0, 601)
	for i := range 600 {
		values = append(values, float64(50+i))
	}
	values = append(values, 1_000_000) // one degenerate upstream report

	dist := NewSpeedDistribution(values, MinPromptTokens)
	if dist == nil {
		t.Fatal("nil distribution")
	}
	if dist.Above != 1 {
		t.Fatalf("the outlier must sit in Above, got above %d counts %v", dist.Above, dist.Counts)
	}
	if dist.Window.Hi >= 1000 {
		t.Fatalf("window top %v let one outlier stretch the axis", dist.Window.Hi)
	}
	if dist.Max != 1_000_000 {
		t.Fatalf("Max must stay honest, got %v", dist.Max)
	}
	if !SpeedPercentileSupported(dist.Count, 99) || dist.P99 == nil {
		t.Fatalf("601 samples support p99, got %v", dist.P99)
	}
	if sum := sumCounts(dist.Counts); sum != dist.Count {
		t.Fatalf("counts sum to %d, want %d", sum, dist.Count)
	}
}

func TestActivity_UsableSpeed(t *testing.T) {
	tests := []struct {
		name      string
		rate      float64
		tokens    int
		minTokens int
		want      bool
	}{
		{"prefill with a real prompt", 3000, 500, MinPromptTokens, true},
		{"prefill of one token", 5, 1, MinPromptTokens, false},
		{"prefill just under the floor", 3000, MinPromptTokens - 1, MinPromptTokens, false},
		{"decode with a real completion", 40, 100, MinGenTokens, true},
		{"one token decode at a million t/s", 1_000_000, 1, MinGenTokens, false},
		{"rate over the physical bound", MaxSpeedPerSecond + 1, 500, MinPromptTokens, false},
		{"not reported", -1, 500, MinPromptTokens, false},
		{"reported as zero", 0, 500, MinPromptTokens, false},
	}
	for _, tt := range tests {
		if got := UsableSpeed(tt.rate, tt.tokens, tt.minTokens); got != tt.want {
			t.Errorf("%s: UsableSpeed(%v, %d, %d) = %v, want %v", tt.name, tt.rate, tt.tokens, tt.minTokens, got, tt.want)
		}
	}
}

func sumCounts(counts []int) int {
	total := 0
	for _, n := range counts {
		total += n
	}
	return total
}

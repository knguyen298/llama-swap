package store

import (
	"math"
	"sort"
	"time"
)

// TokenMetrics holds token usage and performance metrics.
type TokenMetrics struct {
	CachedTokens    int     `json:"cache_tokens"`
	DraftTokens     int     `json:"draft_tokens"`
	DraftAccTokens  int     `json:"draft_acc_tokens"`
	InputTokens     int     `json:"input_tokens"`
	OutputTokens    int     `json:"output_tokens"`
	PromptPerSecond float64 `json:"prompt_per_second"`
	TokensPerSecond float64 `json:"tokens_per_second"`
}

// ActivityLogEntry represents parsed token statistics from llama-server logs.
type ActivityLogEntry struct {
	ID              int               `json:"id"`
	Timestamp       time.Time         `json:"timestamp"`
	Src             string            `json:"src"`
	Model           string            `json:"model"`
	ReqPath         string            `json:"req_path"`
	RespContentType string            `json:"resp_content_type"`
	RespStatusCode  int               `json:"resp_status_code"`
	Tokens          TokenMetrics      `json:"tokens"`
	DurationMs      int               `json:"duration_ms"`
	HasCapture      bool              `json:"has_capture"`
	ErrorMsg        string            `json:"error_msg,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// ActivityFilter narrows which activity rows a query matches. The zero value
// matches every row, and each field is independent: set fields are ANDed.
type ActivityFilter struct {
	Models    []string  // model IN (...); blank entries are ignored, empty matches all models
	Start     time.Time // inclusive lower bound on Timestamp; zero is unbounded
	End       time.Time // inclusive upper bound on Timestamp; zero is unbounded
	MinID     int       // inclusive lower bound on ID; 0 is unbounded
	MaxID     int       // inclusive upper bound on ID; 0 is unbounded
	SrcPrefix string    // literal source prefix; empty matches every source
}

// ActivityQuery selects one page of activity rows.
type ActivityQuery struct {
	ActivityFilter
	Limit int
	Page  int
	Sort  string // sort key from ActivitySortKeys, empty defaults to "id"
	Order string // "asc" or "desc", empty defaults to "desc"
}

// DefaultActivityLimit is the page size used when a query does not set one.
const DefaultActivityLimit = 25

// WithDefaults returns a copy of the query with Page and Limit clamped to
// usable values: Page is at least 1 and Limit defaults to
// DefaultActivityLimit. Repositories apply it before running a query.
func (q ActivityQuery) WithDefaults() ActivityQuery {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.Limit < 1 {
		q.Limit = DefaultActivityLimit
	}
	return q
}

// activitySortKeys whitelists the sortable API keys. Keys mirror the UI
// column ids. Each repository maps them onto its own storage columns, so
// user-supplied sort input never reaches a backend as raw column names.
var activitySortKeys = []string{
	"id",
	"time",
	"model",
	"src",
	"req_path",
	"resp_status_code",
	"resp_content_type",
	"cached",
	"prompt",
	"generated",
	"drafted",
	"prompt_speed",
	"gen_speed",
	"duration",
}

// ActivitySortKeys returns the sort keys accepted by ActivityQuery.Sort.
func ActivitySortKeys() []string {
	return append([]string(nil), activitySortKeys...)
}

// ValidActivitySortKey reports whether key is accepted by ActivityQuery.Sort.
func ValidActivitySortKey(key string) bool {
	for _, k := range activitySortKeys {
		if k == key {
			return true
		}
	}
	return false
}

// ActivityPage is one page of a List result.
type ActivityPage struct {
	Data       []ActivityLogEntry `json:"data"`
	Page       int                `json:"page"`
	Limit      int                `json:"limit"`
	Total      int                `json:"total"`
	TotalPages int                `json:"total_pages"`
}

// TotalPages returns how many pages of size limit are needed for total rows.
// It is zero when there are no rows or the limit is not positive.
func TotalPages(total, limit int) int {
	if total == 0 || limit <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(limit)))
}

// ActivityStatsQuery narrows a Stats call. An empty Model matches all models.
type ActivityStatsQuery struct {
	Model string
}

// ActivityStats are aggregate totals and speed histograms over activity rows.
type ActivityStats struct {
	TotalRequests       int            `json:"total_requests"`
	TotalInputTokens    int            `json:"total_input_tokens"`
	TotalOutputTokens   int            `json:"total_output_tokens"`
	TotalCacheTokens    int            `json:"total_cache_tokens"`
	PromptHistogram     *HistogramData `json:"prompt_histogram"`
	GenerationHistogram *HistogramData `json:"gen_histogram"`
	// Speed distributions are the geometric-bucket view the Activity charts use.
	// The HistogramData fields above stay for clients that still read them.
	PromptSpeed     *SpeedDistribution `json:"prompt_speed"`
	GenerationSpeed *SpeedDistribution `json:"gen_speed"`
	// Sample counts state how many requests each distribution describes.
	PromptSamples     SpeedSampleCounts `json:"prompt_samples"`
	GenerationSamples SpeedSampleCounts `json:"gen_samples"`
}

// HistogramData is a binned distribution with percentile summaries.
type HistogramData struct {
	Bins    []int   `json:"bins"`
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	BinSize float64 `json:"binSize"`
	P50     float64 `json:"p50"`
	P95     float64 `json:"p95"`
	P99     float64 `json:"p99"`
}

// NewHistogram bins values into between 5 and 20 equal-width buckets (Sturges'
// rule) and computes the p50/p95/p99 percentiles. It returns nil for no
// values, and a single bucket with a zero BinSize when every value is equal.
// The input is not modified.
func NewHistogram(values []float64) *HistogramData {
	if len(values) == 0 {
		return nil
	}

	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	minVal := sorted[0]
	maxVal := sorted[len(sorted)-1]

	p50 := percentile(sorted, 50)
	p95 := percentile(sorted, 95)
	p99 := percentile(sorted, 99)

	if minVal == maxVal {
		return &HistogramData{
			Bins:    []int{len(values)},
			Min:     minVal,
			Max:     maxVal,
			BinSize: 0,
			P50:     p50,
			P95:     p95,
			P99:     p99,
		}
	}

	const minBins = 5
	const maxBins = 20
	sturges := int(math.Ceil(math.Log2(float64(len(values))))) + 1
	binCount := min(maxBins, max(minBins, sturges))
	binSize := (maxVal - minVal) / float64(binCount)

	bins := make([]int, binCount)
	for _, value := range values {
		idx := min(int(math.Floor((value-minVal)/binSize)), binCount-1)
		bins[idx]++
	}

	return &HistogramData{
		Bins:    bins,
		Min:     minVal,
		Max:     maxVal,
		BinSize: binSize,
		P50:     p50,
		P95:     p95,
		P99:     p99,
	}
}

// percentile linearly interpolates the p-th percentile of sorted values.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	rank := (p / 100) * float64(len(sorted)-1)
	lower := int(math.Floor(rank))
	upper := int(math.Ceil(rank))
	fraction := rank - float64(lower)
	return sorted[lower] + fraction*(sorted[upper]-sorted[lower])
}

// Speed charts bucket request rates on a geometric grid, so every bucket covers
// the same multiplicative range. Reported rates span several decades, and a
// single extreme value stretches an equal-width axis until every real sample
// shares one bucket. A geometric grid keeps one bucket worth one multiplicative
// step everywhere on the chart, and its bucket index is cheap arithmetic that a
// future stored counter can reuse.
//
// The grid resolution adapts to the plotted window. Twelve steps per decade
// suits data that spans decades, and gives a window of 2.4 decades 29 buckets.
// The same resolution gives one model running at plus or minus 10 percent two
// buckets, which shows no shape at all, so SpeedStepsFor refines the grid up to
// 16 times as fine for a narrow window, and coarsens it to a quarter for a
// sample too small to fill the bars.
// Resolutions come from SpeedResolutions, where each value divides the next, so
// a finer grid contains every coarser edge and charts at different resolutions
// stay comparable.
const (
	// SpeedStepsPerDecade is the base grid resolution and the floor for the
	// adaptive one. One bucket spans 1/SpeedStepsPerDecade decades.
	SpeedStepsPerDecade = 12

	// SpeedStepsFloor is the coarsest grid the ladder offers. A bucket spans a
	// factor of 2.15 there, which suits a sample too small to fill a finer grid.
	SpeedStepsFloor = 3

	// SpeedSamplesPerBar is the fewest samples one bar should carry. It caps the
	// bar count for a small sample, so the cap moves smoothly with n instead of
	// switching on at a fixed sample size.
	SpeedSamplesPerBar = 4

	// SpeedBarsTarget is the number of bars a chart aims for. Sixteen is enough
	// to see a mode and a tail, and the old equal-width chart used 16 bins for
	// a sample this size, so the two charts read at a similar coarseness.
	SpeedBarsTarget = 16

	// SpeedBarsFloor is the target for a very small sample, where more bars than
	// samples turns the chart into a comb of single-request bars.
	SpeedBarsFloor = 4

	// MinPromptTokens is the smallest prompt a reported prefill rate can
	// describe. A shorter prompt measures request overhead, not speed. In an
	// 86 day sample of 45,929 requests the median prefill rate was 5 tokens/sec
	// at one input token, 55 tokens/sec at 8 to 31 tokens, and 186 tokens/sec at
	// 32 to 127 tokens.
	MinPromptTokens = 32

	// MinGenTokens is the smallest completion a reported generation rate can
	// describe. An inter-token latency needs a gap between two tokens, so a
	// one-token completion cannot yield a rate.
	MinGenTokens = 2

	// MaxSpeedPerSecond rejects rates no upstream sustains for one request. A
	// one-token completion that reports a 0.001 ms inter-token latency yields
	// exactly 1,000,000 tokens/sec, which is not a measurement.
	MaxSpeedPerSecond = 100000
)

// SpeedResolutions lists the allowed grids, coarsest first. Each value divides
// the next, so the edges of a coarse grid are a subset of a finer one.
var SpeedResolutions = []int{3, 6, 12, 24, 48, 96, 192}

// SpeedBucketIndex returns the base-grid index that holds value. Values below 1
// give negative indexes, which is intended: the grid is a ladder over decades.
//
// Do not use this to count samples into a published edge list. The round trip
// math.Floor(math.Log10(math.Pow(10, float64(k)/steps))*steps) misses k on 10 to
// 19 percent of edges, because math.Log10 rounds twice. NewSpeedDistribution
// compares values against the edges instead.
func SpeedBucketIndex(value float64) int {
	return SpeedBucketIndexAt(value, SpeedStepsPerDecade)
}

// SpeedBucketIndexAt returns the index that holds value on a grid with the given
// steps per decade.
func SpeedBucketIndexAt(value float64, steps int) int {
	return int(math.Floor(math.Log10(value) * float64(steps)))
}

// SpeedEdge returns the lower bound of base-grid index k.
func SpeedEdge(k int) float64 {
	return SpeedEdgeAt(k, SpeedStepsPerDecade)
}

// SpeedEdgeAt returns the lower bound of index k on a grid with the given steps
// per decade.
func SpeedEdgeAt(k, steps int) float64 {
	return math.Pow(10, float64(k)/float64(steps))
}

// SpeedEdges returns the base-grid edges that cover [lo, hi].
func SpeedEdges(lo, hi float64) []float64 {
	return SpeedEdgesAt(lo, hi, SpeedStepsPerDecade)
}

// SpeedEdgesAt returns the grid edges that cover [lo, hi], rounded outward to
// the grid so the plotted range always contains the window. It returns at least
// two edges, even when every value is equal.
func SpeedEdgesAt(lo, hi float64, steps int) []float64 {
	if steps <= 0 {
		steps = SpeedStepsPerDecade
	}
	if !(hi >= lo) {
		lo, hi = hi, lo
	}
	first := SpeedBucketIndexAt(lo, steps)
	last := int(math.Ceil(math.Log10(hi) * float64(steps)))
	if last <= first {
		last = first + 1
	}
	edges := make([]float64, 0, last-first+1)
	for k := first; k <= last; k++ {
		edges = append(edges, SpeedEdgeAt(k, steps))
	}
	return edges
}

// SpeedBarsFor returns how many bars a chart of n samples aims for. A sample
// too small to fill the target gets the floor instead, so 24 requests draw
// about 6 bars rather than 16 bars of one or two requests each.
func SpeedBarsFor(n int) int {
	target := int(math.Ceil(math.Sqrt(float64(max(n, 0)))))
	if target < SpeedBarsFloor {
		target = SpeedBarsFloor
	}
	if target > SpeedBarsTarget {
		target = SpeedBarsTarget
	}
	return target
}

// SpeedStepsFor returns the grid resolution for the window [lo, hi] and n
// samples. It refines the base grid until the window holds SpeedBarsFor(n) bars,
// and it coarsens the grid when the window would hold more bars than
// max(n/SpeedSamplesPerBar, SpeedBarsFor(n)), because a bar holding one or two
// requests shows binning noise rather than a distribution. Resolution always
// comes from SpeedResolutions, so a window that drifts slightly keeps its grid
// instead of redrawing at a new scale.
func SpeedStepsFor(lo, hi float64, n int) int {
	if !(hi > lo) || lo <= 0 {
		return SpeedStepsPerDecade
	}
	span := math.Log10(hi / lo)
	if span <= 0 {
		return SpeedStepsPerDecade
	}
	target := SpeedBarsFor(n)
	if float64(SpeedStepsPerDecade)*span < float64(target) {
		for _, steps := range SpeedResolutions {
			if float64(steps)*span >= float64(target) {
				return steps
			}
		}
		return SpeedResolutions[len(SpeedResolutions)-1]
	}
	maxBars := max(target, n/SpeedSamplesPerBar)
	if float64(SpeedStepsPerDecade)*span <= float64(maxBars) {
		return SpeedStepsPerDecade
	}
	// Coarsen, finest fitting grid first, so the bars stay as numerous as the
	// sample allows.
	for i := len(SpeedResolutions) - 1; i >= 0; i-- {
		steps := SpeedResolutions[i]
		if steps > SpeedStepsPerDecade {
			continue
		}
		if float64(steps)*span <= float64(maxBars) {
			return steps
		}
	}
	return SpeedStepsFloor
}

// SpeedPercentileSupported reports whether n samples support percentile p. It
// requires at least five samples beyond the requested percentile, so p50 needs
// 10 samples, p90 needs 50, p95 needs 100 and p99 needs 500. A p99.5 of 24
// samples is an interpolation between the top two values, not a tail estimate.
func SpeedPercentileSupported(n int, p float64) bool {
	if p <= 0 || p >= 100 {
		return false
	}
	return float64(n)*float64(100-p)/100 >= 5
}

// SpeedWindow is the range a chart plots. Values outside it still count, and
// SpeedDistribution reports them in Below and Above.
type SpeedWindow struct {
	Lo float64 `json:"lo"`
	Hi float64 `json:"hi"`
}

// SpeedDistribution is a geometric-bucket view of one rate column.
type SpeedDistribution struct {
	// StepsPerDecade is the grid resolution, so a client can rebuild the index
	// arithmetic instead of reading Edges.
	StepsPerDecade int       `json:"stepsPerDecade"`
	Edges          []float64 `json:"edges"`
	// Counts has len(Edges)+1 entries. Index 0 counts samples below Edges[0],
	// index i counts samples in [Edges[i-1], Edges[i]), and the last index
	// counts samples at or above the final edge. The entries sum to Count.
	Counts []int       `json:"counts"`
	Below  int         `json:"below"`
	Above  int         `json:"above"`
	Window SpeedWindow `json:"window"`
	// Count is the number of usable samples, which is Min and Max over the same
	// values. Reported, Excluded and Missing live in SpeedSampleCounts.
	Count int     `json:"count"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	// MinTokens and MaxRatePerSecond restate the usability rules the query
	// applied, so the UI can explain exclusions without duplicating constants.
	MinTokens        int     `json:"minTokens"`
	MaxRatePerSecond float64 `json:"maxRatePerSecond"`
	// Percentiles are nil when Count cannot support them.
	P50 *float64 `json:"p50"`
	P90 *float64 `json:"p90"`
	P95 *float64 `json:"p95"`
	P99 *float64 `json:"p99"`
}

// SpeedSampleCounts states where the samples came from, because the answer is
// never all requests: an upstream either omits timings or reports them as zero
// or negative.
type SpeedSampleCounts struct {
	Rows     int `json:"rows"`     // requests in scope
	Reported int `json:"reported"` // requests that reported a rate above zero
	Usable   int `json:"usable"`   // reported rates the chart can trust
	Excluded int `json:"excluded"` // reported rates rejected by the rules above
	Missing  int `json:"missing"`  // requests that reported no rate at all
}

// NewSpeedDistribution bins values on the shared grid and returns nil when no
// value is usable. minTokens is the rule the caller applied, which the result
// echoes back for the UI. The window is p0.5 to p99.5 once 200 samples support
// those percentiles, and the full range otherwise. The input is not modified.
func NewSpeedDistribution(values []float64, minTokens int) *SpeedDistribution {
	if len(values) == 0 {
		return nil
	}

	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	lo, hi := SpeedWindowFor(sorted)
	steps := SpeedStepsFor(lo, hi, len(sorted))
	edges := SpeedEdgesAt(lo, hi, steps)
	counts := make([]int, len(edges)+1)
	for _, value := range sorted {
		// Compare against the edges rather than re-deriving a grid index from
		// math.Log10, so the counts match the Edges the API publishes.
		// j is the number of edges at or below value: 0 below the first edge,
		// len(edges) at or above the last one.
		j := sort.Search(len(edges), func(j int) bool { return edges[j] > value })
		counts[j]++
	}

	dist := &SpeedDistribution{
		StepsPerDecade:   steps,
		Edges:            edges,
		Counts:           counts,
		Below:            counts[0],
		Above:            counts[len(edges)],
		Window:           SpeedWindow{Lo: edges[0], Hi: edges[len(edges)-1]},
		Count:            len(sorted),
		Min:              sorted[0],
		Max:              sorted[len(sorted)-1],
		MinTokens:        minTokens,
		MaxRatePerSecond: MaxSpeedPerSecond,
	}
	for _, p := range []float64{50, 90, 95, 99} {
		if !SpeedPercentileSupported(len(sorted), p) {
			continue
		}
		value := percentile(sorted, p)
		switch p {
		case 50:
			dist.P50 = &value
		case 90:
			dist.P90 = &value
		case 95:
			dist.P95 = &value
		case 99:
			dist.P99 = &value
		}
	}
	return dist
}

// SpeedWindowFor returns the plotted range of sorted values.
func SpeedWindowFor(sorted []float64) (float64, float64) {
	if len(sorted) == 0 {
		return 0, 0
	}
	if len(sorted) >= 200 {
		return percentile(sorted, 0.5), percentile(sorted, 99.5)
	}
	return sorted[0], sorted[len(sorted)-1]
}

// UsableSpeed reports whether a reported rate describes throughput. tokCount is
// the token count the rate applies to: prompt tokens for a prefill rate and
// generated tokens for a generation rate.
func UsableSpeed(rate float64, tokCount int, minTokens int) bool {
	return rate > 0 && rate <= MaxSpeedPerSecond && tokCount >= minTokens
}

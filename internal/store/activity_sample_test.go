package store

import (
	"compress/gzip"
	"encoding/csv"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestActivity_SpeedDistributionMatchesSampleExport checks the distribution
// against a real activity export, because synthetic values do not reproduce the
// shape that motivated this design: two decode modes from a mixed model fleet, a
// prefill tail that reaches 17,000 tokens/sec, and a handful of degenerate
// one-token completions that report 1,000,000 tokens/sec.
//
// The fixture is a personal dataset, so it stays outside the repository. Point
// LLAMA_SWAP_SAMPLE_CSV at a gzipped export to run the check:
//
//	LLAMA_SWAP_SAMPLE_CSV=~/llama-swap-sample/activity-sample.csv.gz go test ./internal/store/ -run SpeedDistributionMatches
//
// The expected values come from the reviewed prototype, which binned the same
// file. Rates are rounded to two decimals there, so the tolerance is loose.
func TestActivity_SpeedDistributionMatchesSampleExport(t *testing.T) {
	path := os.Getenv("LLAMA_SWAP_SAMPLE_CSV")
	if path == "" {
		t.Skip("LLAMA_SWAP_SAMPLE_CSV is not set; skipping the real-export check")
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open sample: %v", err)
	}
	defer file.Close()

	var source io.Reader = file
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(file)
		if err != nil {
			t.Fatalf("gzip sample: %v", err)
		}
		defer gz.Close()
		source = gz
	}

	reader := csv.NewReader(source)
	header, err := reader.Read()
	if err != nil {
		t.Fatalf("read header: %v", err)
	}
	column := map[string]int{}
	for i, name := range header {
		column[name] = i
	}
	for _, name := range []string{"input_tokens", "output_tokens", "prompt_per_second", "tokens_per_second"} {
		if _, ok := column[name]; !ok {
			t.Fatalf("sample is missing column %q", name)
		}
	}

	var prompt, gen []float64
	var rows, reportedPrompt, reportedGen int
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read row %d: %v", rows+1, err)
		}
		rows++
		promptRate := sampleFloat(t, record, column["prompt_per_second"])
		genRate := sampleFloat(t, record, column["tokens_per_second"])
		inputTokens := int(sampleFloat(t, record, column["input_tokens"]))
		outputTokens := int(sampleFloat(t, record, column["output_tokens"]))
		if promptRate > 0 {
			reportedPrompt++
			if UsableSpeed(promptRate, inputTokens, MinPromptTokens) {
				prompt = append(prompt, promptRate)
			}
		}
		if genRate > 0 {
			reportedGen++
			if UsableSpeed(genRate, outputTokens, MinGenTokens) {
				gen = append(gen, genRate)
			}
		}
	}

	checks := []struct {
		name       string
		dist       *SpeedDistribution
		wantUsable int
		wantEdges  int
		wantBelow  int
		wantAbove  int
		wantMax    float64
		wantP50    float64
		wantP99    float64
	}{
		{"prompt", NewSpeedDistribution(prompt, MinPromptTokens), 23819, 30, 91, 82, 17175.56, 2563.33, 13072.3},
		{"generation", NewSpeedDistribution(gen, MinGenTokens), 24431, 22, 105, 0, 347.49, 93.37, 305.88},
	}
	for _, want := range checks {
		got := want.dist
		if got == nil {
			t.Fatalf("%s: nil distribution", want.name)
		}
		if got.Count != want.wantUsable {
			t.Errorf("%s: usable count %d, want %d", want.name, got.Count, want.wantUsable)
		}
		if len(got.Edges) != want.wantEdges {
			t.Errorf("%s: %d edges, want %d (window %v..%v)", want.name, len(got.Edges), want.wantEdges, got.Window.Lo, got.Window.Hi)
		}
		if got.Below != want.wantBelow || got.Above != want.wantAbove {
			t.Errorf("%s: below %d above %d, want below %d above %d", want.name, got.Below, got.Above, want.wantBelow, want.wantAbove)
		}
		if sum := sumCounts(got.Counts); sum != got.Count {
			t.Errorf("%s: counts sum to %d, want %d", want.name, sum, got.Count)
		}
		sameFloat(t, want.name+" max", got.Max, want.wantMax)
		if got.P50 == nil {
			t.Fatalf("%s: p50 is nil", want.name)
		}
		sameFloat(t, want.name+" p50", *got.P50, want.wantP50)
		if got.P99 == nil {
			t.Fatalf("%s: p99 is nil", want.name)
		}
		sameFloat(t, want.name+" p99", *got.P99, want.wantP99)
	}

	if reportedPrompt != 24436 || reportedGen != 24434 {
		t.Errorf("reported rates: prompt %d generation %d, want 24436 and 24434", reportedPrompt, reportedGen)
	}
	if rows != 45929 {
		t.Errorf("sample rows %d, want 45929", rows)
	}
}

func sampleFloat(t *testing.T, record []string, index int) float64 {
	t.Helper()
	value, err := strconv.ParseFloat(record[index], 64)
	if err != nil {
		t.Fatalf("column %d is %q: %v", index, record[index], err)
	}
	return value
}

// sameFloat allows the two-decimal rounding the prototype applied to its copy of
// the data, plus float drift over 24,000 values.
func sameFloat(t *testing.T, name string, got, want float64) {
	t.Helper()
	tolerance := math.Max(math.Abs(want)*1e-4, 0.01)
	if math.Abs(got-want) > tolerance {
		t.Errorf("%s = %v, want %v (tolerance %v)", name, got, want, tolerance)
	}
}

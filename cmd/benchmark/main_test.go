package main

import (
	"testing"
	"time"
)

func TestPercentileUsesNearestRankForSmallSamples(t *testing.T) {
	values := []int64{40, 10, 30, 20}
	if got := percentile(values, .50); got == nil || *got != 30 {
		t.Fatalf("expected p50=30, got %v", got)
	}
	if got := percentile(values, .95); got == nil || *got != 40 {
		t.Fatalf("expected p95=40, got %v", got)
	}
}

func TestSummarizeKeepsFailuresInThroughputDenominator(t *testing.T) {
	first := int64(100)
	second := int64(200)
	results := []jobResult{
		{Status: "completed", Timings: &timing{TotalMS: &first}},
		{Status: "dead", Error: "invalid input"},
		{Status: "completed", Timings: &timing{TotalMS: &second}},
	}
	started := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := summarize(results, started, started.Add(2*time.Minute))
	if got.Completed != 2 || got.Failed != 1 {
		t.Fatalf("unexpected counts: %+v", got)
	}
	if got.ThroughputPerMin != 1 {
		t.Fatalf("expected one completed job per minute, got %f", got.ThroughputPerMin)
	}
}

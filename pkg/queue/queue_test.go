package queue

import (
	"testing"
	"time"
)

func TestPriorityScoreOrdersHigherPriorityFirst(t *testing.T) {
	enqueuedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	high := PriorityScore(10, enqueuedAt)
	normal := PriorityScore(0, enqueuedAt)

	if high <= normal {
		t.Fatalf("expected higher priority score to win: high=%f normal=%f", high, normal)
	}
}

func TestPriorityScoreOrdersOlderJobsFirstWithinPriority(t *testing.T) {
	older := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	newer := older.Add(time.Minute)

	olderScore := PriorityScore(5, older)
	newerScore := PriorityScore(5, newer)

	if olderScore <= newerScore {
		t.Fatalf("expected older enqueue time to score higher: older=%f newer=%f", olderScore, newerScore)
	}
}

func TestPriorityScoreIsDeterministic(t *testing.T) {
	enqueuedAt := time.Date(2026, 1, 2, 3, 4, 5, 123000000, time.UTC)

	first := PriorityScore(3, enqueuedAt)
	second := PriorityScore(3, enqueuedAt)

	if first != second {
		t.Fatalf("expected deterministic score, got %f and %f", first, second)
	}
}

package queue

import (
	"testing"
)

func TestValidatePriorityAcceptsConfiguredBounds(t *testing.T) {
	for _, priority := range []int{MinPriority, 0, MaxPriority} {
		if err := ValidatePriority(priority); err != nil {
			t.Fatalf("priority %d should be accepted: %v", priority, err)
		}
	}
}

func TestValidatePriorityRejectsOutsideConfiguredBounds(t *testing.T) {
	for _, priority := range []int{MinPriority - 1, MaxPriority + 1} {
		if err := ValidatePriority(priority); err == nil {
			t.Fatalf("priority %d should be rejected", priority)
		}
	}
}

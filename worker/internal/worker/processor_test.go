package worker

import "testing"

func TestFailureActionForRetriesWhenAttemptsRemain(t *testing.T) {
	job := Job{RetryCount: 2, MaxRetries: 3}

	if got := failureActionFor(job); got != failureActionRequeue {
		t.Fatalf("expected %q, got %q", failureActionRequeue, got)
	}
}

func TestFailureActionForMarksDeadWhenRetriesExhausted(t *testing.T) {
	tests := []struct {
		name string
		job  Job
	}{
		{name: "equal to max", job: Job{RetryCount: 3, MaxRetries: 3}},
		{name: "over max", job: Job{RetryCount: 4, MaxRetries: 3}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := failureActionFor(test.job); got != failureActionMarkDead {
				t.Fatalf("expected %q, got %q", failureActionMarkDead, got)
			}
		})
	}
}

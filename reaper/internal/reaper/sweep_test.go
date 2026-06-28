package reaper

import "testing"

func TestRequeueActionForRetriesWhenAttemptsRemain(t *testing.T) {
	job := Job{RetryCount: 0, MaxRetries: 1}

	if got := requeueActionFor(job); got != requeueActionRequeue {
		t.Fatalf("expected %q, got %q", requeueActionRequeue, got)
	}
}

func TestRequeueActionForMarksDeadWhenRetriesExhausted(t *testing.T) {
	tests := []struct {
		name string
		job  Job
	}{
		{name: "equal to max", job: Job{RetryCount: 1, MaxRetries: 1}},
		{name: "over max", job: Job{RetryCount: 2, MaxRetries: 1}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := requeueActionFor(test.job); got != requeueActionMarkDead {
				t.Fatalf("expected %q, got %q", requeueActionMarkDead, got)
			}
		})
	}
}

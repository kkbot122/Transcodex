package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

func (s *Server) metrics(c *gin.Context) {
	stats, err := s.collectStats(c.Request.Context())
	if err != nil {
		c.String(http.StatusServiceUnavailable, "# metrics unavailable\n")
		return
	}

	c.Header("Content-Type", "text/plain; version=0.0.4")
	for _, line := range []string{
		"# HELP transcodex_queue_depth Number of queued jobs known to Redis.",
		"# TYPE transcodex_queue_depth gauge",
		fmt.Sprintf("transcodex_queue_depth %d", stats.QueueDepth),
		"# HELP transcodex_throughput_per_minute Completed jobs in the last minute.",
		"# TYPE transcodex_throughput_per_minute gauge",
		fmt.Sprintf("transcodex_throughput_per_minute %d", stats.ThroughputPerMin),
		"# HELP transcodex_oldest_queued_age_seconds Age of the oldest queued job.",
		"# TYPE transcodex_oldest_queued_age_seconds gauge",
		fmt.Sprintf("transcodex_oldest_queued_age_seconds %s", optionalFloat(stats.OldestQueuedAge)),
		"# HELP transcodex_active_lease_age_seconds Age of the oldest running attempt.",
		"# TYPE transcodex_active_lease_age_seconds gauge",
		fmt.Sprintf("transcodex_active_lease_age_seconds %s", optionalFloat(stats.ActiveLeaseAge)),
	} {
		_, _ = c.Writer.WriteString(line + "\n")
	}
	for status, count := range stats.Jobs {
		_, _ = c.Writer.WriteString(fmt.Sprintf("transcodex_jobs{status=%q} %d\n", status, count))
	}
	for status, count := range stats.Workers {
		_, _ = c.Writer.WriteString(fmt.Sprintf("transcodex_workers{status=%q} %d\n", status, count))
	}
	for status, count := range stats.Attempts {
		_, _ = c.Writer.WriteString(fmt.Sprintf("transcodex_attempts{status=%q} %d\n", status, count))
	}
	for name, value := range map[string]*int64{
		"queue_wait": stats.Latency.QueueWaitP95MS,
		"processing": stats.Latency.ProcessingP95MS,
		"total":      stats.Latency.TotalP95MS,
	} {
		if value != nil {
			_, _ = c.Writer.WriteString(fmt.Sprintf("transcodex_%s_p95_milliseconds %d\n", name, *value))
		}
	}
}

func optionalFloat(value *float64) string {
	if value == nil {
		return "0"
	}
	return fmt.Sprintf("%.3f", *value)
}

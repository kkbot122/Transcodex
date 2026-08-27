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
	} {
		_, _ = c.Writer.WriteString(line + "\n")
	}
	for status, count := range stats.Jobs {
		_, _ = c.Writer.WriteString(fmt.Sprintf("transcodex_jobs{status=%q} %d\n", status, count))
	}
	for status, count := range stats.Workers {
		_, _ = c.Writer.WriteString(fmt.Sprintf("transcodex_workers{status=%q} %d\n", status, count))
	}
}

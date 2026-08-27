package metrics

import (
	"fmt"
	"net/http"
	"time"
)

// Handler exposes the low-cardinality process metrics shared by service containers.
// Domain-specific queue and lifecycle metrics are emitted by the API handler.
func Handler(service string) http.Handler {
	started := time.Now()
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = fmt.Fprintf(w, "# HELP transcodex_service_info Service process identity.\n# TYPE transcodex_service_info gauge\ntranscodex_service_info{service=%q} 1\n# HELP transcodex_process_uptime_seconds Service process uptime.\n# TYPE transcodex_process_uptime_seconds gauge\ntranscodex_process_uptime_seconds{service=%q} %.3f\n", service, service, time.Since(started).Seconds())
	})
}

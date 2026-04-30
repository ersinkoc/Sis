package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ersinkoc/sis/internal/stats"
)

func (s *Server) metrics(w http.ResponseWriter, _ *http.Request) {
	if s.stats == nil {
		http.Error(w, "stats unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(prometheusMetrics(s.stats.Snapshot())))
}

func prometheusMetrics(snapshot stats.Snapshot) string {
	var b strings.Builder
	writeMetricHelp(&b, "sis_dns_queries_total", "counter", "Total DNS queries processed.")
	writeMetricCounter(&b, "sis_dns_queries_total", snapshot.QueryTotal)
	writeMetricHelp(&b, "sis_dns_cache_hits_total", "counter", "Total DNS cache hits.")
	writeMetricCounter(&b, "sis_dns_cache_hits_total", snapshot.CacheHit)
	writeMetricHelp(&b, "sis_dns_cache_misses_total", "counter", "Total DNS cache misses.")
	writeMetricCounter(&b, "sis_dns_cache_misses_total", snapshot.CacheMiss)
	writeMetricHelp(&b, "sis_dns_blocked_queries_total", "counter", "Total DNS queries blocked by policy.")
	writeMetricCounter(&b, "sis_dns_blocked_queries_total", snapshot.BlockedTotal)
	writeMetricHelp(&b, "sis_dns_rate_limited_total", "counter", "Total DNS or API requests rejected by rate limiting.")
	writeMetricCounter(&b, "sis_dns_rate_limited_total", snapshot.RateLimitedTotal)
	writeMetricHelp(&b, "sis_dns_malformed_packets_total", "counter", "Total malformed DNS packets.")
	writeMetricCounter(&b, "sis_dns_malformed_packets_total", snapshot.MalformedTotal)
	writeLatencyMetrics(&b, "sis_dns_latency", "", snapshot.Latency)

	writeMetricHelp(&b, "sis_upstream_requests_total", "counter", "Total upstream resolver requests.")
	writeMetricHelp(&b, "sis_upstream_errors_total", "counter", "Total upstream resolver errors.")
	writeMetricHelp(&b, "sis_upstream_consecutive_errors", "gauge", "Current consecutive upstream resolver errors.")
	writeMetricHelp(&b, "sis_upstream_healthy", "gauge", "Current upstream resolver health state, 1 for healthy and 0 for unhealthy.")
	writeLatencyMetricHelp(&b, "sis_upstream_latency")
	upstreamIDs := make([]string, 0, len(snapshot.Upstreams))
	for id := range snapshot.Upstreams {
		upstreamIDs = append(upstreamIDs, id)
	}
	sort.Strings(upstreamIDs)
	for _, id := range upstreamIDs {
		upstream := snapshot.Upstreams[id]
		label := fmt.Sprintf(`{upstream="%s"}`, escapePrometheusLabel(id))
		writeMetricCounterWithLabel(&b, "sis_upstream_requests_total", label, upstream.Requests)
		writeMetricCounterWithLabel(&b, "sis_upstream_errors_total", label, upstream.Errors)
		writeMetricGaugeWithLabel(&b, "sis_upstream_consecutive_errors", label, upstream.ConsecutiveErrors)
		healthy := uint64(0)
		if upstream.Healthy {
			healthy = 1
		}
		writeMetricGaugeWithLabel(&b, "sis_upstream_healthy", label, healthy)
		writeLatencyMetrics(&b, "sis_upstream_latency", label, upstream.Latency)
	}
	return b.String()
}

func writeMetricHelp(b *strings.Builder, name, metricType, help string) {
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, metricType)
}

func writeMetricCounter(b *strings.Builder, name string, value uint64) {
	fmt.Fprintf(b, "%s %d\n", name, value)
}

func writeMetricCounterWithLabel(b *strings.Builder, name, label string, value uint64) {
	fmt.Fprintf(b, "%s%s %d\n", name, label, value)
}

func writeMetricGaugeWithLabel(b *strings.Builder, name, label string, value uint64) {
	fmt.Fprintf(b, "%s%s %d\n", name, label, value)
}

func writeLatencyMetrics(b *strings.Builder, prefix, label string, latency stats.HistogramSnapshot) {
	if label == "" {
		writeLatencyMetricHelp(b, prefix)
	}
	fmt.Fprintf(b, "%s_observations_total%s %d\n", prefix, label, latency.Count)
	fmt.Fprintf(b, "%s_p50_seconds%s %.9g\n", prefix, label, seconds(latency.P50))
	fmt.Fprintf(b, "%s_p95_seconds%s %.9g\n", prefix, label, seconds(latency.P95))
	fmt.Fprintf(b, "%s_p99_seconds%s %.9g\n", prefix, label, seconds(latency.P99))
}

func writeLatencyMetricHelp(b *strings.Builder, prefix string) {
	fmt.Fprintf(b, "# HELP %s_observations_total Total latency observations.\n# TYPE %s_observations_total counter\n", prefix, prefix)
	fmt.Fprintf(b, "# HELP %s_p50_seconds Approximate p50 latency in seconds.\n# TYPE %s_p50_seconds gauge\n", prefix, prefix)
	fmt.Fprintf(b, "# HELP %s_p95_seconds Approximate p95 latency in seconds.\n# TYPE %s_p95_seconds gauge\n", prefix, prefix)
	fmt.Fprintf(b, "# HELP %s_p99_seconds Approximate p99 latency in seconds.\n# TYPE %s_p99_seconds gauge\n", prefix, prefix)
}

func seconds(d time.Duration) float64 {
	return float64(d) / float64(time.Second)
}

func escapePrometheusLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

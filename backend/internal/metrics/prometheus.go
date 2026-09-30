package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// IngestionJobsTotal tracks the total number of data ingestion pipeline runs.
var IngestionJobsTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "halal_equity_ingestion_jobs_total",
		Help: "Total number of completed data ingestion jobs by symbol",
	},
	[]string{"symbol", "status"},
)

// DataQualityScore measures the average data quality of ingested symbols.
var DataQualityScore = promauto.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "halal_equity_data_quality_score",
		Help: "Data quality percentage score for a specific symbol",
	},
	[]string{"symbol"},
)

// QuantEngineLatency tracks the time taken to process requests to the Python engine.
var QuantEngineLatency = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "halal_equity_quant_engine_latency_ms",
		Help:    "Latency of requests sent to the Python Quant engine in milliseconds",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"endpoint"},
)

// InitMetricsHandler returns an HTTP handler for exposing Prometheus metrics.
func InitMetricsHandler() http.Handler {
	return promhttp.Handler()
}

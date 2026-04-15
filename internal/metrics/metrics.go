package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// IndexedBlocksTotal counts blocks successfully processed per chain and protocol.
	IndexedBlocksTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "indexed_blocks_total",
			Help: "Total number of blocks indexed.",
		},
		[]string{"chain_id", "protocol"},
	)

	// IndexedMessagesTotal counts cross-chain messages indexed per chain and protocol.
	IndexedMessagesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "indexed_messages_total",
			Help: "Total number of cross-chain messages indexed.",
		},
		[]string{"chain_id", "protocol", "event_type"},
	)

	// RPCRequestDurationSeconds observes latency of RPC calls per chain and method.
	RPCRequestDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rpc_request_duration_seconds",
			Help:    "Duration of RPC calls in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"chain_id", "method"},
	)

	// RPCFailoversTotal counts how many times the client switched to a fallback endpoint.
	RPCFailoversTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rpc_failovers_total",
			Help: "Total number of RPC endpoint failovers.",
		},
		[]string{"chain_id"},
	)

	// ReorgsDetectedTotal counts chain reorganizations detected per chain.
	ReorgsDetectedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reorgs_detected_total",
			Help: "Total number of chain reorganizations detected.",
		},
		[]string{"chain_id", "protocol"},
	)

	// ProcessorLagBlocks tracks how far behind each processor is from chain head.
	ProcessorLagBlocks = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "processor_lag_blocks",
			Help: "Number of blocks the processor is behind the chain head.",
		},
		[]string{"chain_id", "protocol"},
	)

	// ArchivedFilesTotal counts Parquet files successfully uploaded to O3.
	ArchivedFilesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "archived_files_total",
			Help: "Total number of Parquet files archived to O3.",
		},
		[]string{"protocol", "chain_id"},
	)
)

// Register registers all metrics with the default Prometheus registry.
func Register() {
	prometheus.MustRegister(
		IndexedBlocksTotal,
		IndexedMessagesTotal,
		RPCRequestDurationSeconds,
		RPCFailoversTotal,
		ReorgsDetectedTotal,
		ProcessorLagBlocks,
		ArchivedFilesTotal,
	)
}

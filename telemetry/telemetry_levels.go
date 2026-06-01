package telemetry

const EnvOtelInsecure = "STEAMPIPE_OTEL_INSECURE"
const EnvOtelLevel = "STEAMPIPE_OTEL_LEVEL"
const EnvOtelEndpoint = "OTEL_EXPORTER_OTLP_ENDPOINT"

// EnvOtelTraceSampleRatio sets the head-based trace sampling ratio in [0,1].
// Unset or >=1 keeps the historical AlwaysSample behaviour; 0 disables tracing
// (NeverSample); a fractional value uses ParentBased(TraceIdRatioBased), i.e. it
// keeps that fraction of whole scan-traces (every span of a sampled trace, both
// per-table and per-row) and drops the rest entirely. Because the FDW and the
// plugins share this telemetry package, the same ratio is applied at the trace
// root (the FDW) and honoured by child spans in the plugin process.
const EnvOtelTraceSampleRatio = "STEAMPIPE_OTEL_TRACE_SAMPLE_RATIO"

// EnvOtelRowSpanSampleRatio sets the sampling ratio in [0,1] for the high-volume
// per-row hydrate spans (rowData.callHydrateWithRetries / hydrateWithIgnoreError),
// which are emitted once per row per hydrate function and dominate OTLP/Cloud
// Trace span volume on wide tables. Unset or >=1 emits them all (historical
// behaviour); 0 drops them entirely; a fractional value keeps that fraction.
// Unlike EnvOtelTraceSampleRatio this is sampled per span at the call site, so
// per-table timing spans (Plugin.Execute, Table.executeListCall/executeGetCall)
// are always retained in full regardless of this setting.
const EnvOtelRowSpanSampleRatio = "STEAMPIPE_OTEL_ROW_SPAN_SAMPLE_RATIO"

// constants for telemetry config flag
const (
	OtelNone    = "none"
	OtelAll     = "all"
	OtelTrace   = "trace"
	OtelMetrics = "metrics"
)

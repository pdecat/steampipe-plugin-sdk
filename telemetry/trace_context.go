package telemetry

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// TraceCtx is a struct which contains a span and the associated context
// This is used by the FDW for persisting [`span`,`context`] tuples across PostreSQL callbacks
type TraceCtx struct {
	Ctx  context.Context
	Span trace.Span
}

func GetTracer(service string) trace.Tracer {
	return otel.GetTracerProvider().Tracer(service)
}

func StartSpan(baseCtx context.Context, service string, format string, args ...interface{}) (context.Context, trace.Span) {
	tr := GetTracer(service)
	return tr.Start(baseCtx, fmt.Sprintf(format, args...), trace.WithTimestamp(time.Now()))
}

// rowSpanSampleRatioVal is the resolved sampling ratio for per-row hydrate
// spans, read once on first use from STEAMPIPE_OTEL_ROW_SPAN_SAMPLE_RATIO.
// Defaults to 1.0 (emit every per-row span — the historical behaviour).
//
// Resolution is deliberately lazy rather than a package-level var initialiser:
// when the SDK is linked into a shared library that is dlopen'd into a host
// process (e.g. the standalone Postgres FDW), the Go runtime's environment is
// not fully populated from the host's C environ until the embedding package's
// init copies it across — and dependency packages such as this one initialise
// first, so a package-init read would always miss the variable and fall back
// to the default. First use (the first hydrate row, or telemetry Init logging)
// is always late enough.
var (
	rowSpanSampleRatioOnce sync.Once
	rowSpanSampleRatioVal  float64
)

// rowSpanSampleRatio resolves the per-row span sampling ratio on first call
// and returns the cached value thereafter.
func rowSpanSampleRatio() float64 {
	rowSpanSampleRatioOnce.Do(func() {
		rowSpanSampleRatioVal = resolveRatioEnv(EnvOtelRowSpanSampleRatio, 1.0)
	})
	return rowSpanSampleRatioVal
}

// noopRowSpanTracer yields non-recording spans so StartRowSpan can return a span
// whose SetAttributes/End calls are safe no-ops when a per-row span is sampled out.
var noopRowSpanTracer = noop.NewTracerProvider().Tracer("steampipe-plugin-sdk")

// resolveRatioEnv parses a [0,1] float from the named env var, clamping out of
// range values and falling back to def when unset or unparseable.
func resolveRatioEnv(envVar string, def float64) float64 {
	raw, ok := os.LookupEnv(envVar)
	if !ok {
		return def
	}
	r, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return def
	}
	if r < 0 {
		return 0
	}
	if r > 1 {
		return 1
	}
	return r
}

// RowSpanSampleRatio reports the resolved per-row hydrate span sampling ratio.
func RowSpanSampleRatio() float64 {
	return rowSpanSampleRatio()
}

// sampleRowSpan decides whether to record an individual per-row hydrate span.
func sampleRowSpan() bool {
	ratio := rowSpanSampleRatio()
	if ratio >= 1 {
		return true
	}
	if ratio <= 0 {
		return false
	}
	return rand.Float64() < ratio
}

// StartRowSpan starts a high-volume per-row hydrate span, subject to the sampling
// ratio in STEAMPIPE_OTEL_ROW_SPAN_SAMPLE_RATIO. When the span is sampled out it
// returns the original context and a no-op span, so the caller's SetAttributes/End
// calls remain safe no-ops and any child spans (e.g. outbound API calls) nest under
// the enclosing per-table span instead. Per-table timing spans are never affected.
func StartRowSpan(baseCtx context.Context, service string, format string, args ...interface{}) (context.Context, trace.Span) {
	if !sampleRowSpan() {
		_, span := noopRowSpanTracer.Start(baseCtx, "")
		return baseCtx, span
	}
	return StartSpan(baseCtx, service, format, args...)
}
